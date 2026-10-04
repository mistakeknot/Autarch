package homeask

// CardFiler files an ask as a tasks card (label needs-mk) through the bb CLI. The protocol is plan section 1.3.1:
//
//   - one exclusive flock per Request key for the whole filing;
//   - Home's registry (`bb home get --request`) is the authority: nothing is created, and
//     tasks is not searched, without an answer from it;
//   - only when the registry says "absent" is every tasks project searched, every page of
//     every project, for a card that carries the exact Request key;
//   - the canonical card (registered, else earliest by (createdAt, id)) is checked against
//     this payload's identity token, then routed by AskingThread;
//   - a replay never succeeds before the agent comment that routes the card exists.
//
// Nothing here talks to a bb server except through the bb CLI, and the CLI is replaceable
// (CardFiler.Run) so tests drive the whole protocol against an in-memory tasks.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// NeedsMkLabel is the label that makes a tasks card an ask for mk.
	NeedsMkLabel = "needs-mk"
	// searchStatuses is every tasks status: a replay after done or canceled must still find the card.
	searchStatuses = "backlog,todo,in_progress,in_review,done,canceled"
	pageLimit      = "500"
	maxPages       = 10000
)

// BBResult is the outcome of one bb call.
type BBResult struct {
	Stdout, Stderr []byte
	Code           int
	TimedOut       bool
	Err            error // the process could not be started
}

// BBRunner runs `bb <args...>` with env as its whole environment.
type BBRunner func(ctx context.Context, env []string, args ...string) BBResult

// CardFiler files cards. The zero value works: it runs `bb` from PATH.
type CardFiler struct {
	Bin     string        // default "bb"
	Timeout time.Duration // per bb call, default 10 s
	Run     BBRunner      // default: exec Bin
	LockDir string        // default ${XDG_RUNTIME_DIR:-/tmp}/autarch-needsmk-<uid>
	// TasksProjects maps a Home project name (case-insensitive) to the tasks project (id, key
	// prefix or name) File and FileForPull file its cards into. It is the operator's binding:
	// the filer never guesses a tasks project from names (a name match is only the poller's
	// "suggested" binding, which mk confirms in Home Settings).
	TasksProjects map[string]string
	// TasksProject is the tasks project used for a Home project with no TasksProjects entry.
	TasksProject string
	// ConfigErr, when set, makes File and FileForPull refuse (ErrInvalid): the binding config was
	// unreadable, so nothing may be filed against a guess.
	ConfigErr error
}

// Environment variables CardFilerFromEnv reads.
const (
	// EnvTasksProject names the default tasks project for every Home project.
	EnvTasksProject = "AUTARCH_TASKS_PROJECT"
	// EnvTasksProjects is a comma list of <home project>=<tasks project> bindings.
	EnvTasksProjects = "AUTARCH_TASKS_PROJECTS"
)

// ParseTasksProjects reads "alpha=ABC,beta=XYZ". Home project names are lowercased.
func ParseTasksProjects(v string) (map[string]string, error) {
	out := map[string]string{}
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, val, ok := strings.Cut(part, "=")
		k, val = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(val)
		if !ok || k == "" || val == "" {
			return nil, fmt.Errorf("%s: %q is not <home project>=<tasks project>", EnvTasksProjects, part)
		}
		if _, dup := out[k]; dup {
			return nil, fmt.Errorf("%s: %q is bound twice", EnvTasksProjects, k)
		}
		out[k] = val
	}
	return out, nil
}

// CardFilerFromEnv is the production constructor: a CardFiler whose tasks project binding comes
// from AUTARCH_TASKS_PROJECTS and AUTARCH_TASKS_PROJECT. A malformed value is kept in ConfigErr.
func CardFilerFromEnv(timeout time.Duration) *CardFiler {
	f := &CardFiler{Timeout: timeout, TasksProject: strings.TrimSpace(os.Getenv(EnvTasksProject))}
	f.TasksProjects, f.ConfigErr = ParseTasksProjects(os.Getenv(EnvTasksProjects))
	return f
}

var (
	_ Filer  = (*CardFiler)(nil)
	_ Lister = (*CardFiler)(nil)
)

// RootRunSpec is the root-run tuple a card carries (the same fields as RootRun).
type RootRunSpec = RootRun

// CardRequest is one filing.
type CardRequest struct {
	Project string         // tasks project: id, key prefix or name; always passed as --project
	Title   string         // the card title
	Blocks  []string       // Blocks refs (bead:, thread:, project:)
	Ask     map[string]any // the home-ask/v2 object; schema is filled in
	RootRun *RootRunSpec
	Key     string // the Request key (idempotency key)
	Thread  string // the asking thread; the filer posts the routing comment from it
	Pull    bool   // threadless Mycroft filing (mk question 4): writes "pull":"mycroft", posts no comment
}

// CardResult is what a successful filing returns.
type CardResult struct {
	ID      string `json:"card"`
	Key     string `json:"request"`
	Project string `json:"project,omitempty"`
	// Replay is true when the card already existed (a routing comment may still have been repaired).
	Replay bool `json:"replay"`
}

// CardView is `bb home get --card`.
type CardView struct {
	TaskID        string `json:"task_id"`
	ProjectID     string `json:"project_id"`
	CardKey       string `json:"card_key"`
	Title         string `json:"title"`
	State         string `json:"state"`
	AskingThread  string `json:"asking_thread"`
	RoutingMode   string `json:"routing_mode"`
	DecisionID    string `json:"decision_id"`
	Generation    int    `json:"generation"`
	DecisionState string `json:"decision_state"`
}

// CardLister reads the outcomes of cards Mycroft pulled.
type CardLister interface {
	// ListPull is `bb home list --pull mycroft`: pull cards and pre-card mycroft asks alike.
	ListPull(ctx context.Context) ([]ListRow, error)
	// Card is `bb home get --card`.
	Card(ctx context.Context, taskID string) (CardView, error)
}

var _ CardLister = (*CardFiler)(nil)

// ---- identity and description -------------------------------------------------------

// RequestIdentity is hash H1 (plan 1.2): the first 16 hex characters of the sha256 of the
// canonical JSON of {project, title, blocks sorted, ask, root_run}. The tasks project is not
// part of it, so the same ask filed into two tasks projects is one Request. It is compared
// only by the filer.
func RequestIdentity(r CardRequest) (string, error) {
	ask, err := r.wireAsk()
	if err != nil {
		return "", err
	}
	blocks := dedupe(r.Blocks)
	sort.Strings(blocks)
	node := map[string]any{"project": ask["project"], "title": r.Title, "blocks": toAny(blocks), "ask": ask}
	if r.RootRun != nil {
		node["root_run"] = map[string]any{"script": r.RootRun.Script, "sha256": r.RootRun.SHA256, "timeout": r.RootRun.Timeout, "set": r.RootRun.Set}
	}
	raw, err := json.Marshal(node)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	canon, err := CanonicalJSON(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	sum := sha256.Sum256([]byte(canon))
	return hex.EncodeToString(sum[:])[:16], nil
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func dedupe(ss []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// wireAsk is the home-ask/v2 object this request writes.
func (r CardRequest) wireAsk() (map[string]any, error) {
	if r.Ask == nil {
		return nil, fmt.Errorf("%w: no ask", ErrInvalid)
	}
	m := make(map[string]any, len(r.Ask)+2)
	for k, v := range r.Ask {
		m[k] = v
	}
	if s, ok := m["schema"]; ok && s != "home-ask/v2" {
		return nil, fmt.Errorf("%w: ask schema must be home-ask/v2", ErrInvalid)
	}
	m["schema"] = "home-ask/v2"
	if r.Pull {
		m["pull"] = "mycroft"
	} else if _, ok := m["pull"]; ok {
		return nil, fmt.Errorf("%w: only Mycroft's threadless filing may set pull", ErrInvalid)
	}
	return m, nil
}

// Description renders the card description and returns it with the identity token. The
// result is parsed back with ParseCard and must read as the same card, so an ask the card
// convention cannot carry (a question line that starts with "Blocks:", an approval token,
// a v1-only field) is refused here, before anything is created.
func (r CardRequest) Description() (desc, ident string, err error) {
	if !cardKeyRe.MatchString(r.Key) {
		return "", "", fmt.Errorf("%w: Request key must match %s", ErrInvalid, cardKeyRe)
	}
	if strings.TrimSpace(r.Title) == "" {
		return "", "", fmt.Errorf("%w: a title is required", ErrInvalid)
	}
	ask, err := r.wireAsk()
	if err != nil {
		return "", "", err
	}
	question, _ := ask["question"].(string)
	if ident, err = RequestIdentity(r); err != nil {
		return "", "", err
	}
	body, err := json.Marshal(ask)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("\n\n")
	if blocks := dedupe(r.Blocks); len(blocks) > 0 {
		b.WriteString("Blocks: " + strings.Join(blocks, " ") + "\n")
	}
	fmt.Fprintf(&b, "Request: %s sha256:%s\n\n```home-ask\n%s\n```\n", r.Key, ident, body)
	if rr := r.RootRun; rr != nil {
		fmt.Fprintf(&b, "\n```root-run\nscript: %s\nsha256: %s\ntimeout: %d\nset: %s\n```\n", rr.Script, rr.SHA256, rr.Timeout, rr.Set)
	}
	desc = b.String()

	c, perr := ParseCard(desc)
	if perr != nil {
		return "", "", fmt.Errorf("%w: the ask does not read back as a card: %v", ErrInvalid, perr)
	}
	want := dedupe(r.Blocks)
	got := make([]string, len(c.Blocks))
	for i, br := range c.Blocks {
		got[i] = br.Ref
	}
	wantAsk, _ := json.Marshal(ask)
	gotAsk, _ := json.Marshal(c.Ask)
	wa, _ := CanonicalJSON(wantAsk)
	ga, _ := CanonicalJSON(gotAsk)
	pull := ""
	if r.Pull {
		pull = "mycroft"
	}
	ok := c.Request == RequestLine{Key: r.Key, Identity: ident} && strings.Join(got, " ") == strings.Join(want, " ") &&
		c.Pull == pull && wa == ga && c.Question == strings.TrimSpace(question) && rootRunEqual(c.RootRun, r.RootRun)
	if !ok {
		return "", "", fmt.Errorf("%w: the ask does not read back as the same card (a question line may start with Blocks: or Request:)", ErrInvalid)
	}
	return desc, ident, nil
}

func rootRunEqual(a, b *RootRun) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ---- running bb ---------------------------------------------------------------------

func (f *CardFiler) timeout() time.Duration {
	if f.Timeout > 0 {
		return f.Timeout
	}
	return 10 * time.Second
}

// cardEnv is the child environment: BB_THREAD_ID is the only variable the bb CLI derives the
// caller's thread from, so it is always set to the asking thread, or removed for a pull.
func cardEnv(thread string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "BB_THREAD_ID=") {
			env = append(env, kv)
		}
	}
	if thread != "" {
		env = append(env, "BB_THREAD_ID="+thread)
	}
	return env
}

func (f *CardFiler) exec(ctx context.Context, env []string, args ...string) BBResult {
	if f.Run != nil {
		return f.Run(ctx, env, args...)
	}
	bin := f.Bin
	if bin == "" {
		bin = "bb"
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.WaitDelay = time.Second
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	r := BBResult{Stdout: so.Bytes(), Stderr: se.Bytes()}
	if err == nil {
		return r
	}
	if ctx.Err() == context.DeadlineExceeded {
		r.TimedOut = true
		return r
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		r.Code = ee.ExitCode()
		return r
	}
	r.Err = err
	return r
}

func (r BBResult) failed() bool { return r.Err != nil || r.TimedOut || r.Code != 0 }

func (r BBResult) text() string {
	switch {
	case r.Err != nil:
		return r.Err.Error()
	case r.TimedOut:
		return "timed out"
	}
	s := strings.TrimSpace(string(r.Stderr))
	if s == "" {
		s = strings.TrimSpace(string(r.Stdout))
	}
	return s
}

// call runs bb and decodes its JSON stdout into out. Any failure is "unavailable".
func (f *CardFiler) call(ctx context.Context, thread string, out any, args ...string) error {
	r := f.exec(ctx, cardEnv(thread), args...)
	if r.failed() {
		return fmt.Errorf("%w: bb %s: %s", ErrHomeDown, strings.Join(args[:min(len(args), 3)], " "), r.text())
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(r.Stdout, out); err != nil {
		return fmt.Errorf("%w: bb %s: unreadable answer: %v", ErrHomeDown, strings.Join(args[:min(len(args), 3)], " "), err)
	}
	return nil
}

var notFoundRe = regexp.MustCompile(`(?i)not found|no such task|unknown task|does not exist`)

// ---- lock ---------------------------------------------------------------------------

func (f *CardFiler) lockPath(key string) (string, error) {
	if f.LockDir != "" {
		if err := os.MkdirAll(f.LockDir, 0o700); err != nil {
			return "", err
		}
		return filepath.Join(f.LockDir, key+".lock"), nil
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = "/tmp"
	}
	name := fmt.Sprintf("autarch-needsmk-%d", os.Getuid())
	dir := filepath.Join(base, name)
	err := os.MkdirAll(dir, 0o700)
	if err == nil {
		// An existing directory on a read-only file system passes MkdirAll; only a write shows it.
		var probe *os.File
		if probe, err = os.CreateTemp(dir, ".probe-*"); err == nil {
			probe.Close()
			os.Remove(probe.Name())
		}
	}
	if err != nil {
		// A read-only /tmp (a sandbox) cannot hold the lock; os.TempDir() honors TMPDIR.
		dir = filepath.Join(os.TempDir(), name)
		if err2 := os.MkdirAll(dir, 0o700); err2 != nil {
			return "", err
		}
	}
	return filepath.Join(dir, key+".lock"), nil
}

// ---- registry -----------------------------------------------------------------------

type registryAnswer struct {
	Status     string `json:"status"`
	TaskID     string `json:"task_id"`
	Identity   string `json:"identity"`
	DecisionID string `json:"decision_id"`
	State      string `json:"state"`
}

func (f *CardFiler) registry(ctx context.Context, key string) (registryAnswer, error) {
	var a registryAnswer
	if err := f.call(ctx, "", &a, "home", "get", "--request", key, "--json"); err != nil {
		return a, err
	}
	switch a.Status {
	case "absent":
	case "registered":
		if a.TaskID == "" || a.Identity == "" {
			return a, fmt.Errorf("%w: bb home get: a registered answer without a card", ErrHomeDown)
		}
	case "legacy":
		if a.DecisionID == "" {
			return a, fmt.Errorf("%w: bb home get: a legacy answer without a decision", ErrHomeDown)
		}
	default:
		return a, fmt.Errorf("%w: bb home get: unexpected answer %q", ErrHomeDown, a.Status)
	}
	return a, nil
}

// ---- tasks reads ---------------------------------------------------------------------

type taskRow struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	CreatedAt   string `json:"createdAt"`
	Description string `json:"description"`
}

type shown struct {
	Task     taskRow   `json:"task"`
	Comments []Comment `json:"comments"`
}

var errTaskGone = errors.New("task not found")

func (f *CardFiler) show(ctx context.Context, id string) (shown, error) {
	var s shown
	r := f.exec(ctx, cardEnv(""), "tasks", "show", id, "--json")
	if r.failed() {
		if r.Err == nil && !r.TimedOut && notFoundRe.MatchString(r.text()) {
			return s, errTaskGone
		}
		return s, fmt.Errorf("%w: bb tasks show %s: %s", ErrHomeDown, id, r.text())
	}
	if err := json.Unmarshal(r.Stdout, &s); err != nil || s.Task.ID == "" {
		return s, fmt.Errorf("%w: bb tasks show %s: unreadable answer", ErrHomeDown, id)
	}
	return s, nil
}

// requestOf reads the card's Request line through the strict parser. A card the parser refuses
// has no usable identity: it is never canonical.
func requestOf(desc string) (RequestLine, bool) {
	c, err := ParseCard(desc)
	if err != nil {
		return RequestLine{}, false
	}
	return c.Request, true
}

type found struct {
	ID        string
	ProjectID string
	CreatedAt string
	Req       RequestLine
	Comments  []Comment
}

// search is step 2.4: every tasks project, every page, exact key. It returns nil when no card
// carries the key. Any failure is ErrHomeDown, so nothing is created on an incomplete read.
func (f *CardFiler) search(ctx context.Context, key string) (*found, error) {
	var projects struct {
		Projects []struct {
			ID string `json:"id"`
		} `json:"projects"`
	}
	if err := f.call(ctx, "", &projects, "tasks", "project", "list", "--json"); err != nil {
		return nil, err
	}
	exact := regexp.MustCompile(`(?m)^Request: ` + regexp.QuoteMeta(key) + `(?:[^A-Za-z0-9:_.-]|$)`)
	var keep []*found
	seen := map[string]bool{}
	for _, p := range projects.Projects {
		cursor := ""
		seenCursor := map[string]bool{}
		for page := 0; ; page++ {
			if page >= maxPages {
				return nil, fmt.Errorf("%w: tasks search did not finish", ErrHomeDown)
			}
			args := []string{"tasks", "list", "--project", p.ID, "--status", searchStatuses, "--search", "Request: " + key, "--limit", pageLimit}
			if cursor != "" {
				args = append(args, "--cursor", cursor)
			}
			args = append(args, "--json")
			var res struct {
				Tasks      []taskRow `json:"tasks"`
				NextCursor *string   `json:"nextCursor"`
			}
			if err := f.call(ctx, "", &res, args...); err != nil {
				return nil, err
			}
			for _, t := range res.Tasks {
				if seen[t.ID] {
					continue
				}
				seen[t.ID] = true
				s, err := f.show(ctx, t.ID)
				if err != nil {
					if errors.Is(err, errTaskGone) { // deleted between the list and the read
						continue
					}
					return nil, err
				}
				rl, ok := requestOf(s.Task.Description)
				if !ok || rl.Key != key {
					if !ok && exact.MatchString(s.Task.Description) {
						return nil, fmt.Errorf("%w: Request %s appears on unparseable card %s", ErrInvalid, key, t.ID)
					}
					continue
				}
				keep = append(keep, &found{ID: t.ID, ProjectID: s.Task.ProjectID, CreatedAt: s.Task.CreatedAt, Req: rl, Comments: s.Comments})
			}
			if res.NextCursor == nil || *res.NextCursor == "" {
				break
			}
			if seenCursor[*res.NextCursor] {
				return nil, fmt.Errorf("%w: tasks search cursor did not advance", ErrHomeDown)
			}
			seenCursor[*res.NextCursor] = true
			cursor = *res.NextCursor
		}
	}
	if len(keep) == 0 {
		return nil, nil
	}
	sort.Slice(keep, func(i, j int) bool {
		if keep[i].CreatedAt != keep[j].CreatedAt {
			return keep[i].CreatedAt < keep[j].CreatedAt
		}
		return keep[i].ID < keep[j].ID
	})
	return keep[0], nil
}

// locate returns the canonical card for the key, or nil when the Request was never filed.
func (f *CardFiler) locate(ctx context.Context, key, ident string) (*found, error) {
	reg, err := f.registry(ctx, key)
	if err != nil {
		return nil, err
	}
	switch reg.Status {
	case "legacy":
		return nil, fmt.Errorf("%w: Request %s belongs to a pre-cards ask %s", ErrInvalid, key, reg.DecisionID)
	case "registered":
		s, err := f.show(ctx, reg.TaskID)
		moved := fmt.Errorf("%w: Request %s is registered to card %s, which was deleted or changed; use a new Request", ErrInvalid, key, reg.TaskID)
		if errors.Is(err, errTaskGone) {
			return nil, moved
		}
		if err != nil {
			return nil, err
		}
		rl, ok := requestOf(s.Task.Description)
		if !ok || rl.Key != key || rl.Identity != reg.Identity {
			return nil, moved
		}
		if reg.Identity != ident {
			return nil, errReused
		}
		return &found{ID: reg.TaskID, ProjectID: s.Task.ProjectID, CreatedAt: s.Task.CreatedAt, Req: rl, Comments: s.Comments}, nil
	}
	c, err := f.search(ctx, key)
	if err != nil || c == nil {
		return nil, err
	}
	if c.Req.Identity != ident {
		return nil, errReused
	}
	return c, nil
}

// ErrForeignPullCard: the card registered under a pull's legacy key is not that Mycroft pull.
var ErrForeignPullCard = fmt.Errorf("%w: foreign card under the pull key", ErrInvalid)

var errReused = fmt.Errorf("%w: Request reused for a different card", ErrInvalid)

// ---- filing -------------------------------------------------------------------------

func (f *CardFiler) ensureLabel(ctx context.Context, project string) error {
	var l struct {
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := f.call(ctx, "", &l, "tasks", "label", "list", "--project", project, "--json"); err != nil {
		return err
	}
	for _, x := range l.Labels {
		if strings.EqualFold(x.Name, NeedsMkLabel) {
			return nil
		}
	}
	r := f.exec(ctx, cardEnv(""), "tasks", "label", "create", "--project", project, "--name", NeedsMkLabel)
	if r.failed() && !(r.Err == nil && !r.TimedOut && regexp.MustCompile(`(?i)already (in use|exists)`).MatchString(r.text())) {
		return fmt.Errorf("%w: bb tasks label create: %s", ErrHomeDown, r.text())
	}
	return nil
}

func (f *CardFiler) comment(ctx context.Context, thread, id, key string) error {
	var res struct {
		Comment Comment `json:"comment"`
	}
	err := f.call(ctx, thread, &res, "tasks", "comment", id, "--body", fmt.Sprintf("Asked from this thread (Request %s).", key), "--json")
	if err != nil {
		return fmt.Errorf("%w: card %s exists but its routing comment was not recorded; re-run the same command: %v", ErrOutcomeUnknown, id, err)
	}
	if res.Comment.Kind != "agent" || res.Comment.ThreadID != thread {
		return fmt.Errorf("%w: card %s: the comment was not recorded as an agent comment from thread %s; re-run the same command", ErrOutcomeUnknown, id, thread)
	}
	return nil
}

// replay finishes a filing whose card already exists: it never succeeds before the routing
// comment exists, and it refuses a card routed to another thread.
func (f *CardFiler) replay(ctx context.Context, req CardRequest, c *found) (CardResult, error) {
	res := CardResult{ID: c.ID, Key: req.Key, Replay: true}
	if req.Pull {
		return res, nil
	}
	agent := false
	for _, cm := range c.Comments {
		if cm.Kind == "agent" {
			agent = true
		}
	}
	asking := AskingThread(c.Comments)
	switch {
	case !agent:
		return res, f.comment(ctx, req.Thread, c.ID, req.Key)
	case asking == "":
		return CardResult{}, fmt.Errorf("%w: Request %s is on card %s, whose first agent comment carries no thread", ErrInvalid, req.Key, c.ID)
	case asking != req.Thread:
		return CardResult{}, fmt.Errorf("%w: Request %s filed from thread %s", ErrInvalid, req.Key, asking)
	}
	return res, nil
}

// FileCard files one card, or finds the card this Request already filed.
func (f *CardFiler) FileCard(ctx context.Context, req CardRequest) (CardResult, error) {
	if !req.Pull && req.Thread == "" {
		return CardResult{}, fmt.Errorf("%w: BB_THREAD_ID is required: the asking thread routes the card", ErrInvalid)
	}
	if req.Pull && req.Thread != "" {
		return CardResult{}, fmt.Errorf("%w: a pull filing has no thread", ErrInvalid)
	}
	if strings.TrimSpace(req.Project) == "" {
		return CardResult{}, fmt.Errorf("%w: a tasks project is required", ErrInvalid)
	}
	desc, ident, err := req.Description()
	if err != nil {
		return CardResult{}, err
	}
	path, err := f.lockPath(req.Key)
	if err != nil {
		return CardResult{}, fmt.Errorf("%w: lock: %v", ErrHomeDown, err)
	}
	unlock, err := lockFile(path)
	if err != nil {
		return CardResult{}, fmt.Errorf("%w: lock: %v", ErrHomeDown, err)
	}
	defer unlock()

	if err := f.ensureLabel(ctx, req.Project); err != nil {
		return CardResult{}, err
	}
	c, err := f.locate(ctx, req.Key, ident)
	if err != nil {
		return CardResult{}, err
	}
	if c != nil {
		res, err := f.replay(ctx, req, c)
		res.Project = c.ProjectID
		return res, err
	}

	dir, err := os.MkdirTemp("", "autarch-needsmk-")
	if err != nil {
		return CardResult{}, fmt.Errorf("%w: %v", ErrHomeDown, err)
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "description.md")
	if err := os.WriteFile(file, []byte(desc), 0o600); err != nil {
		return CardResult{}, fmt.Errorf("%w: %v", ErrHomeDown, err)
	}
	var created struct {
		Task taskRow `json:"task"`
	}
	r := f.exec(ctx, cardEnv(req.Thread), "tasks", "create", "--project", req.Project, "--title", req.Title, "--description-file", file, "--label", NeedsMkLabel, "--json")
	if r.failed() || json.Unmarshal(r.Stdout, &created) != nil || created.Task.ID == "" {
		// The card may have been written before the answer was lost: look again, under the same lock.
		c, lerr := f.locate(ctx, req.Key, ident)
		switch {
		case lerr != nil && errors.Is(lerr, ErrInvalid):
			return CardResult{}, lerr
		case lerr != nil:
			return CardResult{}, fmt.Errorf("%w: the create call failed (%s) and the card could not be looked up; re-run the same command", ErrOutcomeUnknown, r.text())
		case c == nil:
			return CardResult{}, fmt.Errorf("%w: bb tasks create: %s", ErrHomeDown, r.text())
		}
		res, err := f.replay(ctx, req, c)
		res.Project = c.ProjectID
		return res, err
	}
	res := CardResult{ID: created.Task.ID, Key: req.Key, Project: created.Task.ProjectID}
	if req.Pull {
		return res, nil
	}
	return res, f.comment(ctx, req.Thread, created.Task.ID, req.Key)
}

// ---- Filer, Lister and the threadless pull ---------------------------------------------

// wireFromAsk turns a rev-4 ask into the v2 object a card carries.
func wireFromAsk(a Ask) (map[string]any, error) {
	n, err := Normalize(a)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if len(n.Steps) > 0 || n.Machine != nil || n.MentionOf != "" || n.Supersedes != "" {
		return nil, fmt.Errorf("%w: steps, machine, mention_of and supersedes cannot be carried by a card", ErrInvalid)
	}
	raw, err := json.Marshal(n)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	for k := range v1Only {
		delete(m, k)
	}
	return m, nil
}

func titleOf(a Ask) string {
	if a.Subject != "" {
		return a.Subject
	}
	q := strings.Join(strings.Fields(a.Question), " ")
	if len([]rune(q)) > 80 {
		q = string([]rune(q)[:80])
	}
	return q
}

// tasksProject is the tasks project File and FileForPull file into: the configured binding for
// the Home project, else the configured default. There is no name-matching fallback.
func (f *CardFiler) tasksProject(homeProject string) (string, error) {
	if f.ConfigErr != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, f.ConfigErr)
	}
	if p := f.TasksProjects[strings.ToLower(homeProject)]; p != "" {
		return p, nil
	}
	if f.TasksProject != "" {
		return f.TasksProject, nil
	}
	return "", fmt.Errorf("%w: no tasks project is bound to Home project %q: set %s=%s=<tasks project> or %s", ErrInvalid, homeProject, EnvTasksProjects, strings.ToLower(homeProject), EnvTasksProject)
}

// File implements Filer for an ask from a thread: the card is filed and routed to a.Thread. The
// Request key is a.RequestID, else a UUIDv5 of the ask's identity, so a blind retry is idempotent.
// It returns the card's task id.
func (f *CardFiler) File(ctx context.Context, a Ask) (string, error) {
	if a.Asker == "mycroft" {
		return "", fmt.Errorf("%w: a threadless ask is filed with FileForPull", ErrInvalid)
	}
	wire, err := wireFromAsk(a)
	if err != nil {
		return "", err
	}
	key := a.RequestID
	if key == "" {
		key = uuid.NewSHA1(needsMkNS, []byte(a.Thread+"\x00"+Identity(a))).String()
	}
	project, err := f.tasksProject(a.Project)
	if err != nil {
		return "", err
	}
	res, err := f.FileCard(ctx, CardRequest{Project: project, Title: titleOf(a), Ask: wire, Key: key, Thread: a.Thread})
	return res.ID, err
}

// needsMkNS is the UUIDv5 namespace of derived Request keys.
var needsMkNS = uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://autarch.invalid/needs-mk"))

// PullKey is the Request key of a pull filing: the UUIDv5 of the rev-4 key
// mycroft:<project>:<bead>:<agent>.
func PullKey(legacyKey string) string {
	return uuid.NewSHA1(needsMkNS, []byte(legacyKey)).String()
}

// FileForPull is mk question 4's default, and the only threadless way to file: Mycroft's pull
// asks. It writes "pull":"mycroft" and posts no comment; the card is forgeable and carries no
// routing authority. It is called only from cmd/mycroft, and the autarch CLI has no flag for it.
//
// The rev-4 key mycroft:<project>:<bead>:<agent> (a.RequestID) is looked up first, in Home's
// registry: while the legacy ask is open nothing is filed. It returns the card's task id, or the
// open legacy ask's decision id.
func (f *CardFiler) FileForPull(ctx context.Context, a Ask) (string, error) {
	if a.Asker != "mycroft" || !strings.HasPrefix(a.RequestID, "mycroft:") {
		return "", fmt.Errorf("%w: a pull ask needs asker mycroft and a mycroft:<project>:<bead>:<agent> request id", ErrInvalid)
	}
	reg, err := f.registry(ctx, a.RequestID)
	if err != nil {
		return "", err
	}
	switch reg.Status {
	case "legacy":
		if reg.State == "open" {
			return reg.DecisionID, nil
		}
		return "", fmt.Errorf("%w: %s", ErrAlreadyRuled, a.RequestID)
	case "registered":
		// The legacy key is predictable, so any same-uid filer can register a card under it. Report the card as
		// this pull only when its parsed body says so: a Mycroft pull card, under this key, for this ask.
		s, err := f.show(ctx, reg.TaskID)
		if err != nil {
			return "", err
		}
		c, perr := ParseCard(s.Task.Description)
		wire, werr := wireFromAsk(a)
		var want string
		if werr == nil {
			want, werr = RequestIdentity(CardRequest{Title: titleOf(a), Ask: wire, Pull: true})
		}
		if werr != nil {
			return "", werr
		}
		if perr != nil || c.Pull != "mycroft" || c.Request.Key != a.RequestID || reg.Identity != want || c.Request.Identity != want {
			return "", fmt.Errorf("%w: card %s registered under %s is not that Mycroft pull", ErrForeignPullCard, reg.TaskID, a.RequestID)
		}
		return reg.TaskID, nil
	}
	wire, err := wireFromAsk(a)
	if err != nil {
		return "", err
	}
	project, err := f.tasksProject(a.Project)
	if err != nil {
		return "", err
	}
	res, err := f.FileCard(ctx, CardRequest{Project: project, Title: titleOf(a), Ask: wire, Key: PullKey(a.RequestID), Pull: true})
	return res.ID, err
}

// List implements Lister: `bb home list --asker X --json`.
func (f *CardFiler) List(ctx context.Context, asker string) ([]ListRow, error) {
	var rows []ListRow
	if err := f.call(ctx, "", &rows, "home", "list", "--asker", asker, "--json"); err != nil {
		return nil, err
	}
	return rows, nil
}

// ListPull implements CardLister.
func (f *CardFiler) ListPull(ctx context.Context) ([]ListRow, error) {
	var rows []ListRow
	if err := f.call(ctx, "", &rows, "home", "list", "--pull", "mycroft", "--json"); err != nil {
		return nil, err
	}
	return rows, nil
}

// Card implements CardLister.
func (f *CardFiler) Card(ctx context.Context, taskID string) (CardView, error) {
	var v CardView
	err := f.call(ctx, "", &v, "home", "get", "--card", taskID, "--json")
	return v, err
}

// DerivedKey is the Request key `autarch needs-mk file` uses when --request is absent: the
// UUIDv5 of the asking thread and the request's identity, so a blind retry of the same ask from
// the same thread is idempotent and a different ask never collides.
func DerivedKey(thread, ident string) string {
	return uuid.NewSHA1(needsMkNS, []byte("thread\x00"+thread+"\x00"+ident)).String()
}

// ResolveAskProject checks the ask's project against the Home project the card's tasks project is
// bound to. Home hides a card whose ask project is not the bound one (SHWK-25), so a confirmed
// binding is enforced at file time: the tasks key (prefix) is accepted as an alias and replaced by
// the bound name; any other value is refused with the expected one. An unbound, suggested or
// rejected binding leaves the ask alone (Home shows the card flagged) and returns a warning for the
// caller to print; a Home too old to answer leaves it alone silently.
func (f *CardFiler) ResolveAskProject(ctx context.Context, tasksProject, askProject string) (string, string, error) {
	var projects struct {
		Projects []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Prefix string `json:"prefix"`
		} `json:"projects"`
	}
	if err := f.call(ctx, "", &projects, "tasks", "project", "list", "--json"); err != nil {
		return askProject, "", nil
	}
	id, prefix := "", ""
	for _, p := range projects.Projects {
		if p.ID == tasksProject || strings.EqualFold(p.Prefix, tasksProject) || strings.EqualFold(p.Name, tasksProject) {
			id, prefix = p.ID, p.Prefix
			break
		}
	}
	if id == "" {
		return askProject, "", nil
	}
	var b struct {
		Home  *string `json:"home_project"`
		State *string `json:"state"`
	}
	if err := f.call(ctx, "", &b, "home", "binding", id); err != nil {
		return askProject, "", nil
	}
	if b.Home == nil || b.State == nil || *b.State != "confirmed" {
		// Fails open (the card still shows, flagged with a Bind button), but say so at file time.
		return askProject, fmt.Sprintf("tasks project %q has no confirmed Home project binding; the card will show in Home flagged \"project not bound\" until the vizier runs `bb home bind` or mk clicks Bind", tasksProject), nil
	}
	home := *b.Home
	if askProject == home {
		return askProject, "", nil
	}
	if prefix != "" && strings.EqualFold(askProject, prefix) {
		return home, "", nil
	}
	return "", "", fmt.Errorf("%w: ask project %q is not the Home project bound to tasks project %q; expected project %q (the tasks key %q is also accepted)", ErrInvalid, askProject, tasksProject, home, prefix)
}
