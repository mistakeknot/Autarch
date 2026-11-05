//go:build unix

package homerun

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// UnitProps are the systemd properties the launcher and the recovery lookup read.
type UnitProps struct {
	LoadState    string // "not-found" when systemd has no such unit
	ActiveState  string
	SubState     string
	Result       string // success, exit-code, timeout, signal, core-dump, ...
	InvocationID string
	ExitStatus   int       // ExecMainStatus
	StartedAt    time.Time // ExecMainStartTimestamp; zero when unknown
	ExitedAt     time.Time // ExecMainExitTimestamp; zero when unknown
}

// Systemd is everything the launcher needs from the system manager. The real implementation shells out
// (CLISystemd); tests use a fake and start nothing.
type Systemd interface {
	// Start runs systemd-run with args (the program name is not part of args). It returns once the unit's
	// process has been exec'd, or with the error systemd-run reported.
	Start(ctx context.Context, args []string) error
	// Show reads the unit's properties. A unit systemd does not know is not an error: LoadState is "not-found".
	Show(ctx context.Context, unit string) (UnitProps, error)
}

// Runner runs a program and returns its standard output. ExecRunner is the real one; it is never used
// by the tests.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs a program with no environment other than a fixed PATH and no standard input.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"PATH=" + UnitPATH, "LC_ALL=C"}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// showProps is the property list passed to "systemctl show".
var showProps = []string{"LoadState", "ActiveState", "SubState", "Result", "InvocationID", "ExecMainStatus", "ExecMainStartTimestamp", "ExecMainExitTimestamp"}

// CLISystemd drives the system manager (not --user: the unit needs User= and the sandbox properties,
// which only the system manager grants) through systemd-run and systemctl.
type CLISystemd struct {
	Runner     Runner
	SystemdRun string // default "systemd-run"
	Systemctl  string // default "systemctl"
}

func (c CLISystemd) bin(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// Start implements Systemd.
func (c CLISystemd) Start(ctx context.Context, args []string) error {
	_, err := c.Runner.Run(ctx, c.bin(c.SystemdRun, "systemd-run"), args...)
	return err
}

// ShowArgs is the systemctl argument list Show uses.
func ShowArgs(unit string) []string {
	return []string{"show", "--timestamp=unix", "--property=" + strings.Join(showProps, ","), "--", unit}
}

// Show implements Systemd.
func (c CLISystemd) Show(ctx context.Context, unit string) (UnitProps, error) {
	out, err := c.Runner.Run(ctx, c.bin(c.Systemctl, "systemctl"), ShowArgs(unit)...)
	if err != nil {
		return UnitProps{}, err
	}
	return ParseShow(out)
}

// ParseShow parses "systemctl show" output (Key=Value lines). A missing LoadState is an error: the answer
// cannot be told from "systemd has no such unit".
func ParseShow(out []byte) (UnitProps, error) {
	kv := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			kv[k] = v
		}
	}
	if err := sc.Err(); err != nil {
		return UnitProps{}, err
	}
	p := UnitProps{LoadState: kv["LoadState"], ActiveState: kv["ActiveState"], SubState: kv["SubState"], Result: kv["Result"], InvocationID: kv["InvocationID"]}
	if p.LoadState == "" {
		return UnitProps{}, errors.New("systemctl show gave no LoadState")
	}
	if s := kv["ExecMainStatus"]; s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			return UnitProps{}, fmt.Errorf("ExecMainStatus %q: %w", s, err)
		}
		p.ExitStatus = n
	}
	var err error
	if p.StartedAt, err = parseUnixTS(kv["ExecMainStartTimestamp"]); err != nil {
		return UnitProps{}, err
	}
	if p.ExitedAt, err = parseUnixTS(kv["ExecMainExitTimestamp"]); err != nil {
		return UnitProps{}, err
	}
	return p, nil
}

// parseUnixTS reads systemctl's "--timestamp=unix" form "@<seconds>"; empty, "n/a" and "@0" mean unset.
func parseUnixTS(s string) (time.Time, error) {
	if s == "" || s == "n/a" || s == "@0" {
		return time.Time{}, nil
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(s, "@"), 10, 64)
	if err != nil || !strings.HasPrefix(s, "@") || n < 0 {
		return time.Time{}, fmt.Errorf("timestamp %q is not @<unix seconds>", s)
	}
	return time.Unix(n, 0).UTC(), nil
}

// OutputSummary reports the size and sha256 of an attempt's output.
type OutputSummary interface {
	Summarize(attempt string) (sha string, bytes int64, err error)
}

// FileOutput summarises the attempt's log under Config.LogDir.
type FileOutput struct{ Config LaunchConfig }

// Summarize implements OutputSummary.
func (f FileOutput) Summarize(attempt string) (string, int64, error) {
	file, err := os.Open(f.Config.LogPath(attempt))
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// AttemptFromUnit inverts UnitName.
func AttemptFromUnit(unit string) (string, error) {
	id := strings.ToUpper(strings.TrimSuffix(strings.TrimPrefix(unit, "homerun-"), ".service"))
	if !strings.HasPrefix(unit, "homerun-") || !strings.HasSuffix(unit, ".service") || !reAttempt.MatchString(id) || UnitName(id) != unit {
		return "", fmt.Errorf("unit %q is not a run unit", unit)
	}
	return id, nil
}

// ended reports whether the unit's main process is finished. A state the launcher does not know is an
// error, never a guess.
func (p UnitProps) ended() (bool, error) {
	switch p.ActiveState {
	case "activating", "deactivating":
		return false, nil
	case "failed", "inactive":
		return true, nil
	case "active":
		// RemainAfterExit=yes keeps a finished service "active (exited)".
		return p.SubState == "exited", nil
	}
	return false, fmt.Errorf("unit state %q/%q is not one the launcher knows", p.ActiveState, p.SubState)
}

// endInfo maps systemd's Result onto an EndInfo. A result with no faithful mapping (resources, start-limit,
// watchdog, a zero status on "exit-code", an unreadable log) leaves Result or OutputSHA256 empty, which
// RecoverUnits records as UNCERTAIN, not as a guess.
func endInfo(p UnitProps, out OutputSummary, attempt string) EndInfo {
	info := EndInfo{ExitStatus: p.ExitStatus}
	switch p.Result {
	case "success":
		info.Result = ResultSuccess
	case "exit-code":
		info.Result = ResultFailed
	case "timeout":
		info.Result = ResultTimeout
	case "signal", "core-dump":
		info.Result = ResultSignal
	}
	if !p.ExitedAt.IsZero() {
		info.EndedAt = p.ExitedAt
		if !p.StartedAt.IsZero() && !p.ExitedAt.Before(p.StartedAt) {
			info.Duration = p.ExitedAt.Sub(p.StartedAt)
		}
	}
	if out != nil {
		if sha, n, err := out.Summarize(attempt); err == nil {
			info.OutputSHA256, info.OutputBytes = sha, n
		}
	}
	return info
}

// UnitLookupFor returns the UnitLookup RecoverUnits needs: it asks systemd about the unit and maps the
// answer. out may be nil (every ended unit is then UNCERTAIN, for want of an output digest). Pass nil as
// the by-InvocationID lookup of RecoverUnits: systemd cannot be asked by invocation once a unit is gone,
// and the by-unit answer is checked against the recorded invocation anyway.
func UnitLookupFor(sys Systemd, out OutputSummary) UnitLookup {
	return func(ctx context.Context, unit string) (UnitState, error) {
		attempt, err := AttemptFromUnit(unit)
		if err != nil {
			return UnitState{}, err
		}
		p, err := sys.Show(ctx, unit)
		if err != nil {
			return UnitState{}, err
		}
		if p.LoadState == "not-found" {
			return UnitState{}, nil
		}
		st := UnitState{Found: true, InvocationID: p.InvocationID}
		done, err := p.ended()
		if err != nil {
			return UnitState{}, err
		}
		if done {
			info := endInfo(p, out, attempt)
			st.Ended = &info
		}
		return st, nil
	}
}
