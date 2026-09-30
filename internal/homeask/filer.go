package homeask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Typed outcomes of a filing. The autarch CLI maps them to exit codes 2, 3, 4 and 5.
var (
	// ErrInvalid: the ask is malformed, or its request id belongs to a different ask.
	ErrInvalid = errors.New("invalid ask")
	// ErrHomeDown: bb is missing or its server is not reachable; nothing was filed.
	ErrHomeDown = errors.New("home is down, nothing filed")
	// ErrOutcomeUnknown: the request may have been written; re-run the same command.
	ErrOutcomeUnknown = errors.New("outcome unknown, re-run the same command")
	// ErrAlreadyRuled: the decision has already been ruled on.
	ErrAlreadyRuled = errors.New("already ruled")
)

// Filer files an ask with Home and returns the decision id.
type Filer interface {
	File(ctx context.Context, a Ask) (string, error)
}

// ListRow is one waiting ask as `bb home list --json` prints it.
type ListRow struct {
	ID       string `json:"id"`
	Subject  string `json:"subject"`
	Project  string `json:"project"`
	Thread   string `json:"thread"`
	Asker    string `json:"asker"`
	FiledAt  string `json:"filed_at"`
	Priority int    `json:"priority,omitempty"`
}

// Lister reads the asks waiting for mk, for one asker.
type Lister interface {
	List(ctx context.Context, asker string) ([]ListRow, error)
}

// ExecFiler files through the bb CLI: `bb home ask --request-stdin`.
type ExecFiler struct {
	Bin             string        // default "bb"
	Timeout         time.Duration // per bb call, default 10 s
	RecoveryTimeout time.Duration // for the recovery read, default 10 s
}

var _ Filer = (*ExecFiler)(nil)
var _ Lister = (*ExecFiler)(nil)

var refusedRe = regexp.MustCompile(`(?i)ECONNREFUSED|connection refused|connect ETIMEDOUT|not running|could not connect|no such host`)

func (f *ExecFiler) bin() string {
	if f.Bin != "" {
		return f.Bin
	}
	return "bb"
}

func dur(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

// childEnv fixes the thread the bb CLI proxy will see: BB_THREAD_ID is the only
// variable it derives the caller's thread from. Mycroft filings run without one.
func childEnv(thread, asker string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "BB_THREAD_ID=") {
			env = append(env, kv)
		}
	}
	if thread != "" && asker != "mycroft" {
		env = append(env, "BB_THREAD_ID="+thread)
	}
	return env
}

type outcome struct {
	stdout, stderr []byte
	code           int
	timedOut       bool
	startErr       error
}

func (f *ExecFiler) run(ctx context.Context, timeout time.Duration, env []string, stdin []byte, args ...string) outcome {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.bin(), args...)
	cmd.Env = env
	cmd.WaitDelay = time.Second // a killed script's orphaned child must not hold the pipes open
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	o := outcome{stdout: so.Bytes(), stderr: se.Bytes()}
	if err == nil {
		return o
	}
	if ctx.Err() == context.DeadlineExceeded {
		o.timedOut = true
		return o
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		o.code = ee.ExitCode()
		return o
	}
	o.startErr = err
	return o
}

func msg(o outcome) string {
	s := strings.TrimSpace(string(o.stderr))
	if s == "" {
		s = strings.TrimSpace(string(o.stdout))
	}
	return s
}

// File implements Filer.
func (f *ExecFiler) File(ctx context.Context, a Ask) (string, error) {
	n, err := Normalize(a)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if n.RequestID == "" {
		n.RequestID = identityOf(n)
	}
	body, err := json.Marshal(n)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	env := childEnv(n.Thread, n.Asker)
	o := f.run(ctx, dur(f.Timeout, 10*time.Second), env, body, "home", "ask", "--request-stdin")
	if o.startErr != nil {
		return "", fmt.Errorf("%w: %v", ErrHomeDown, o.startErr)
	}
	if !o.timedOut {
		switch o.code {
		case 0:
			var r struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(o.stdout, &r) == nil && r.ID != "" {
				return r.ID, nil
			}
		case 2:
			return "", fmt.Errorf("%w: %s", ErrInvalid, msg(o))
		case 3:
			return "", fmt.Errorf("%w: %s", ErrHomeDown, msg(o))
		case 5:
			return "", fmt.Errorf("%w: %s", ErrAlreadyRuled, msg(o))
		default:
			if refusedRe.Match(o.stderr) {
				return "", fmt.Errorf("%w: %s", ErrHomeDown, msg(o))
			}
		}
	}
	// A timeout, a lost connection or an unreadable answer after the request was
	// written: ask Home whether it is filed before saying anything.
	return f.recover(ctx, n, env)
}

func (f *ExecFiler) recover(ctx context.Context, n Ask, env []string) (string, error) {
	o := f.run(ctx, dur(f.RecoveryTimeout, 10*time.Second), env, nil, "home", "get", "--request-id", n.RequestID, "--json")
	if o.timedOut || o.startErr != nil || o.code != 0 {
		return "", fmt.Errorf("%w: the recovery read did not answer", ErrOutcomeUnknown)
	}
	var g struct {
		DecisionID string  `json:"decision_id"`
		Identity   string  `json:"identity"`
		Thread     *string `json:"thread"`
		Project    *string `json:"project"`
	}
	if json.Unmarshal(o.stdout, &g) != nil || g.DecisionID == "" {
		return "", fmt.Errorf("%w: the recovery read was unreadable", ErrOutcomeUnknown)
	}
	thread, project := "", ""
	if g.Thread != nil {
		thread = *g.Thread
	}
	if g.Project != nil {
		project = *g.Project
	}
	if g.Identity != Identity(n) || thread != n.Thread || project != n.Project {
		return "", fmt.Errorf("%w: request id %s is already used for a different ask", ErrInvalid, n.RequestID)
	}
	return g.DecisionID, nil
}

// List implements Lister: `bb home list --asker X --json`.
func (f *ExecFiler) List(ctx context.Context, asker string) ([]ListRow, error) {
	o := f.run(ctx, dur(f.Timeout, 10*time.Second), childEnv("", "mycroft"), nil, "home", "list", "--asker", asker, "--json")
	if o.startErr != nil || o.timedOut || o.code != 0 {
		return nil, fmt.Errorf("%w: %v", ErrHomeDown, firstErr(o))
	}
	var rows []ListRow
	if err := json.Unmarshal(o.stdout, &rows); err != nil {
		return nil, fmt.Errorf("%w: unreadable list: %v", ErrHomeDown, err)
	}
	return rows, nil
}

func firstErr(o outcome) any {
	switch {
	case o.startErr != nil:
		return o.startErr
	case o.timedOut:
		return "timed out"
	default:
		return msg(o)
	}
}
