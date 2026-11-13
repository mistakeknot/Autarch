package orchcompat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type fbComment struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	ThreadID  string `json:"threadId"`
	CreatedAt string `json:"createdAt"`
	Body      string `json:"body"`
}

type fbTask struct {
	ID       string      `json:"id"`
	Project  string      `json:"projectId"`
	Created  string      `json:"createdAt"`
	Desc     string      `json:"description"`
	Title    string      `json:"title"`
	Comments []fbComment `json:"comments"`
	Labels   []string    `json:"labels"`
	Deleted  bool        `json:"deleted"`
}

type fbProject struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

type fbReg struct {
	Status     string `json:"status"`
	TaskID     string `json:"task_id,omitempty"`
	Identity   string `json:"identity,omitempty"`
	DecisionID string `json:"decision_id,omitempty"`
	State      string `json:"state,omitempty"`
}

type fbBinding struct {
	Home  string `json:"home_project"`
	State string `json:"state"`
}

type fbFail struct {
	Prefix        string `json:"prefix"`
	Code          int    `json:"code"`
	Stderr        string `json:"stderr"`
	Stdout        string `json:"stdout"`
	Skip          int    `json:"skip"`
	Times         int    `json:"times"`
	ApplyThenFail bool   `json:"applyThenFail"`
}

type fbState struct {
	Projects         []fbProject          `json:"projects"`
	Labels           map[string]bool      `json:"labels"`
	Tasks            []*fbTask            `json:"tasks"`
	Registry         map[string]fbReg     `json:"registry"`
	Threads          map[string]string    `json:"threads"`
	Bindings         map[string]fbBinding `json:"bindings"`
	NoBinding        bool                 `json:"noBinding"`
	NoProjectList    bool                 `json:"noProjectList"`
	CardKeyAfter     int                  `json:"cardKeyAfter"`
	CardGets         int                  `json:"cardGets"`
	Seq              int                  `json:"seq"`
	Clock            int                  `json:"clock"`
	PageSize         int                  `json:"pageSize"`
	NoRegisterOnMake bool                 `json:"noRegisterOnMake"`
	Fail             []fbFail             `json:"fail"`
}

type fbCall struct {
	Args    []string `json:"args"`
	Thread  string   `json:"thread"`
	Project string   `json:"project"`
	Body    string   `json:"body,omitempty"`
	Desc    string   `json:"desc,omitempty"`
}

var requestLineRe = regexp.MustCompile(`(?m)^Request: (\S+) sha256:([0-9a-f]{16})$`)

func fbPaths(dir string) (state, calls, lock string) {
	return filepath.Join(dir, "state.json"), filepath.Join(dir, "calls.jsonl"), filepath.Join(dir, "lock")
}

func fbLoad(dir string) (*fbState, error) {
	sp, _, _ := fbPaths(dir)
	s := &fbState{Labels: map[string]bool{}, Registry: map[string]fbReg{}, Threads: map[string]string{}, Bindings: map[string]fbBinding{}, PageSize: 2}
	raw, err := os.ReadFile(sp)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(raw, s); err != nil {
		return nil, err
	}
	if s.Labels == nil {
		s.Labels = map[string]bool{}
	}
	if s.Registry == nil {
		s.Registry = map[string]fbReg{}
	}
	if s.Threads == nil {
		s.Threads = map[string]string{}
	}
	if s.Bindings == nil {
		s.Bindings = map[string]fbBinding{}
	}
	return s, nil
}

func fbSave(dir string, s *fbState) error {
	sp, _, _ := fbPaths(dir)
	raw, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(sp, raw, 0o600)
}

func fbLock(dir string) (func(), error) {
	_, _, lp := fbPaths(dir)
	f, err := os.OpenFile(lp, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}

func fbFlag(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func fbFlags(args []string, name string) []string {
	var out []string
	for i, a := range args {
		if a == name && i+1 < len(args) {
			out = append(out, args[i+1])
		}
	}
	return out
}

func fbStamp(base int, n int) string {
	return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(base+n) * time.Second).Format("2006-01-02T15:04:05.000Z")
}

func fbAsking(t *fbTask) string {
	best := -1
	for i, c := range t.Comments {
		if c.Kind != "agent" {
			continue
		}
		if best < 0 || c.CreatedAt < t.Comments[best].CreatedAt || (c.CreatedAt == t.Comments[best].CreatedAt && c.ID < t.Comments[best].ID) {
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	return t.Comments[best].ThreadID
}

func fakeBBMain(args []string) int {
	dir := os.Getenv("ORCHCOMPAT_FAKEBB_DIR")
	if dir == "" {
		fmt.Fprintln(os.Stderr, "fakebb: ORCHCOMPAT_FAKEBB_DIR is not set")
		return 98
	}
	unlock, err := fbLock(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakebb:", err)
		return 98
	}
	defer unlock()
	s, err := fbLoad(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakebb:", err)
		return 98
	}
	call := fbCall{Args: args, Thread: os.Getenv("BB_THREAD_ID"), Project: os.Getenv("BB_PROJECT_ID")}
	code, out, errOut := fbRun(s, args, &call)
	_, cp, _ := fbPaths(dir)
	if f, err := os.OpenFile(cp, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		raw, _ := json.Marshal(call)
		f.Write(append(raw, '\n'))
		f.Close()
	}
	if err := fbSave(dir, s); err != nil {
		fmt.Fprintln(os.Stderr, "fakebb:", err)
		return 98
	}
	os.Stdout.WriteString(out)
	os.Stderr.WriteString(errOut)
	return code
}

func fbJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw) + "\n"
}

func fbRun(s *fbState, args []string, call *fbCall) (int, string, string) {
	joined := strings.Join(args, " ")
	var lost *fbFail
	for i := range s.Fail {
		f := &s.Fail[i]
		if !strings.HasPrefix(joined, f.Prefix) || f.Times == 0 {
			continue
		}
		if f.Skip > 0 {
			f.Skip--
			continue
		}
		if f.Times > 0 {
			f.Times--
		}
		if f.ApplyThenFail {
			lost = f
			break
		}
		return f.Code, f.Stdout, f.Stderr
	}
	code, out, errOut := fbDispatch(s, args, call)
	if lost != nil {
		return lost.Code, lost.Stdout, lost.Stderr
	}
	return code, out, errOut
}

func fbTaskByID(s *fbState, id string) *fbTask {
	for _, t := range s.Tasks {
		if t.ID == id && !t.Deleted {
			return t
		}
	}
	return nil
}

func fbDispatch(s *fbState, args []string, call *fbCall) (int, string, string) {
	if len(args) < 2 {
		return 99, "", "fakebb: unexpected call " + strings.Join(args, " ") + "\n"
	}
	head := strings.Join(args[:min(len(args), 3)], " ")
	switch {
	case args[0] == "thread" && args[1] == "show" && len(args) > 2:
		id := args[2]
		switch s.Threads[id] {
		case "live":
			return 0, fbJSON(map[string]any{"thread": map[string]any{"id": id, "archivedAt": nil, "deletedAt": nil}}), ""
		case "archived":
			return 0, fbJSON(map[string]any{"thread": map[string]any{"id": id, "archivedAt": 1793829351063, "deletedAt": nil}}), ""
		case "deleted":
			return 0, fbJSON(map[string]any{"thread": map[string]any{"id": id, "archivedAt": nil, "deletedAt": 1793829351063}}), ""
		}
		return 1, "", "Error: HTTP 404: Thread not found\n"
	case head == "home get --request":
		key := args[3]
		if r, ok := s.Registry[key]; ok {
			return 0, fbJSON(r), ""
		}
		return 0, fbJSON(fbReg{Status: "absent"}), ""
	case head == "home get --card":
		t := fbTaskByID(s, args[3])
		if t == nil {
			return 1, "", "card not found\n"
		}
		s.CardGets++
		key := ""
		if s.CardKeyAfter >= 0 && s.CardGets > s.CardKeyAfter {
			key = "CK-" + t.ID
		}
		return 0, fbJSON(map[string]any{"task_id": t.ID, "project_id": t.Project, "card_key": key, "title": t.Title, "state": "open",
			"asking_thread": fbAsking(t), "routing_mode": "thread", "generation": 1}), ""
	case args[0] == "home" && args[1] == "binding":
		if s.NoBinding {
			return 1, "", "unknown command: binding\n"
		}
		if len(args) < 3 {
			return 1, "", "binding needs a project\n"
		}
		b, ok := s.Bindings[args[2]]
		if !ok {
			return 0, `{"home_project":null,"state":null}` + "\n", ""
		}
		return 0, fbJSON(b), ""
	case head == "tasks label list":
		var ls []map[string]string
		names := make([]string, 0, len(s.Labels))
		for l := range s.Labels {
			names = append(names, l)
		}
		sort.Strings(names)
		for _, l := range names {
			ls = append(ls, map[string]string{"name": l})
		}
		return 0, fbJSON(map[string]any{"labels": ls}), ""
	case head == "tasks label create":
		n := fbFlag(args, "--name")
		if s.Labels[n] {
			return 1, "", "label already in use\n"
		}
		s.Labels[n] = true
		return 0, fbJSON(map[string]any{"label": map[string]string{"name": n}}), ""
	case head == "thread list --json":
		return 0, "[]", ""
	case head == "tasks project list":
		if s.NoProjectList {
			return 1, "", "projects unavailable\n"
		}
		return 0, fbJSON(map[string]any{"projects": s.Projects}), ""
	case args[0] == "tasks" && args[1] == "list":
		p, search := fbFlag(args, "--project"), fbFlag(args, "--search")
		if p == "" && call.Project != "" {
			return 0, fbJSON(map[string]any{"tasks": []any{}, "nextCursor": nil, "limit": 500}), ""
		}
		var hits []*fbTask
		for _, t := range s.Tasks {
			if !t.Deleted && (p == "" || t.Project == p) && strings.Contains(t.Desc, search) {
				hits = append(hits, t)
			}
		}
		start := 0
		if c := fbFlag(args, "--cursor"); c != "" {
			start, _ = strconv.Atoi(c)
		}
		page := s.PageSize
		if page <= 0 {
			page = 2
		}
		end := min(start+page, len(hits))
		rows := []map[string]string{}
		for _, t := range hits[min(start, len(hits)):end] {
			rows = append(rows, map[string]string{"id": t.ID, "projectId": t.Project})
		}
		var next *string
		if end < len(hits) {
			n := strconv.Itoa(end)
			next = &n
		}
		return 0, fbJSON(map[string]any{"tasks": rows, "nextCursor": next, "limit": 500}), ""
	case args[0] == "tasks" && args[1] == "show" && len(args) > 2:
		t := fbTaskByID(s, args[2])
		if t == nil {
			return 1, "", "task not found\n"
		}
		return 0, fbJSON(map[string]any{"task": map[string]string{"id": t.ID, "projectId": t.Project, "createdAt": t.Created, "description": t.Desc}, "comments": t.Comments}), ""
	case head == "tasks create --project", args[0] == "tasks" && args[1] == "create":
		raw, err := os.ReadFile(fbFlag(args, "--description-file"))
		if err != nil {
			return 1, "", err.Error() + "\n"
		}
		s.Seq++
		t := &fbTask{ID: fmt.Sprintf("T%d", s.Seq), Project: fbFlag(args, "--project"), Created: fbStamp(0, s.Seq), Desc: string(raw), Title: fbFlag(args, "--title"), Labels: fbFlags(args, "--label")}
		call.Desc = t.Desc
		s.Tasks = append(s.Tasks, t)
		if !s.NoRegisterOnMake {
			if m := requestLineRe.FindStringSubmatch(t.Desc); m != nil {
				s.Registry[m[1]] = fbReg{Status: "registered", TaskID: t.ID, Identity: m[2]}
			}
		}
		return 0, fbJSON(map[string]any{"task": map[string]string{"id": t.ID, "projectId": t.Project}}), ""
	case args[0] == "tasks" && args[1] == "update" && len(args) > 2:
		t := fbTaskByID(s, args[2])
		if t == nil {
			return 1, "", "task not found\n"
		}
		if f := fbFlag(args, "--description-file"); f != "" {
			raw, err := os.ReadFile(f)
			if err != nil {
				return 1, "", err.Error() + "\n"
			}
			t.Desc = string(raw)
			call.Desc = t.Desc
		}
		t.Labels = append(t.Labels, fbFlags(args, "--add-label")...)
		return 0, fbJSON(map[string]any{"task": map[string]string{"id": t.ID}}), ""
	case args[0] == "tasks" && args[1] == "comment" && len(args) > 2:
		t := fbTaskByID(s, args[2])
		if t == nil {
			return 1, "", "task not found\n"
		}
		kind := "user"
		if call.Thread != "" {
			kind = "agent"
		}
		s.Clock++
		c := fbComment{ID: fmt.Sprintf("c%03d", s.Clock), Kind: kind, ThreadID: call.Thread, CreatedAt: fbStamp(1000, s.Clock), Body: fbFlag(args, "--body")}
		call.Body = c.Body
		t.Comments = append(t.Comments, c)
		return 0, fbJSON(map[string]any{"comment": c}), ""
	}
	return 99, "", "fakebb: unexpected call " + strings.Join(args, " ") + "\n"
}
