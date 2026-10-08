package homeask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// memBB is an in-memory tasks service plus Home registry behind CardFiler.Run. It records every
// call so the tests can assert what was and was not run.
type memBB struct {
	mu       sync.Mutex
	t        *testing.T
	projects []string
	labels   map[string]bool
	tasks    []*fakeTask
	registry map[string]registryAnswer
	legacy   map[string]registryAnswer
	pageSize int
	calls    [][]string
	fail     func(args []string) *BBResult // injected failure, nil to proceed
	// registerOnCreate registers created cards the way the plugin does once it has seen them.
	registerOnCreate bool
	afterCreate      func(f *memBB, id string) // runs after a create, to model lost answers
	seq              int
	lockDir          string
	lockHeld         func() bool
}

type fakeTask struct {
	ID, Project, Created, Desc string
	Comments                   []Comment
	Labels                     []string
	Deleted                    bool
}

func newFake(t *testing.T) *memBB {
	return &memBB{t: t, projects: []string{"P1"}, labels: map[string]bool{}, registry: map[string]registryAnswer{},
		legacy: map[string]registryAnswer{}, pageSize: 2, registerOnCreate: true}
}

func (f *memBB) filer() *CardFiler {
	return &CardFiler{Run: f.run, LockDir: f.t.TempDir(), TasksProject: "P1"}
}

func okRes(v any) BBResult {
	raw, _ := json.Marshal(v)
	return BBResult{Stdout: raw}
}

func badRes(code int, msg string) *BBResult { return &BBResult{Code: code, Stderr: []byte(msg)} }

func argFlag(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func (f *memBB) count(prefix ...string) int {
	n := 0
	for _, c := range f.calls {
		if len(c) >= len(prefix) && strings.Join(c[:len(prefix)], " ") == strings.Join(prefix, " ") {
			n++
		}
	}
	return n
}

func (f *memBB) run(ctx context.Context, env []string, args ...string) BBResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string(nil), args...))
	if f.fail != nil {
		if r := f.fail(args); r != nil {
			return *r
		}
	}
	thread := ""
	for _, kv := range env {
		if strings.HasPrefix(kv, "BB_THREAD_ID=") {
			thread = strings.TrimPrefix(kv, "BB_THREAD_ID=")
		}
	}
	cmd := strings.Join(args[:min(len(args), 3)], " ")
	switch {
	case cmd == "home get --request":
		key := args[3]
		if a, hit := f.registry[key]; hit {
			return okRes(a)
		}
		if a, hit := f.legacy[key]; hit {
			return okRes(a)
		}
		return okRes(registryAnswer{Status: "absent"})
	case cmd == "tasks label list":
		var ls []map[string]string
		for l := range f.labels {
			ls = append(ls, map[string]string{"name": l})
		}
		return okRes(map[string]any{"labels": ls})
	case cmd == "tasks label create":
		n := argFlag(args, "--name")
		if f.labels[n] {
			return *badRes(1, "label already in use")
		}
		f.labels[n] = true
		return okRes(map[string]any{"label": map[string]string{"name": n}})
	case cmd == "tasks project list":
		var ps []map[string]string
		for _, p := range f.projects {
			ps = append(ps, map[string]string{"id": p, "name": p})
		}
		return okRes(map[string]any{"projects": ps})
	case strings.HasPrefix(cmd, "tasks list"):
		p, search := argFlag(args, "--project"), argFlag(args, "--search")
		var hits []*fakeTask
		for _, tk := range f.tasks {
			if !tk.Deleted && (p == "" || tk.Project == p) && strings.Contains(tk.Desc, search) {
				hits = append(hits, tk)
			}
		}
		start := 0
		if c := argFlag(args, "--cursor"); c != "" {
			start, _ = strconv.Atoi(c)
		}
		end := min(start+f.pageSize, len(hits))
		var rows []map[string]string
		for _, tk := range hits[start:end] {
			rows = append(rows, map[string]string{"id": tk.ID, "projectId": tk.Project})
		}
		var next *string
		if end < len(hits) {
			s := strconv.Itoa(end)
			next = &s
		}
		return okRes(map[string]any{"tasks": rows, "nextCursor": next, "limit": 500})
	case cmd == "tasks show "+args[2]:
		for _, tk := range f.tasks {
			if tk.ID == args[2] && !tk.Deleted {
				return okRes(map[string]any{"task": map[string]string{"id": tk.ID, "projectId": tk.Project, "createdAt": tk.Created, "description": tk.Desc}, "comments": tk.Comments})
			}
		}
		return *badRes(1, "task not found")
	case cmd == "tasks create --project":
		file := argFlag(args, "--description-file")
		raw, err := os.ReadFile(file)
		if err != nil {
			return *badRes(1, err.Error())
		}
		if argFlag(args, "--label") != NeedsMkLabel {
			f.t.Errorf("create without the needs-mk label: %v", args)
		}
		f.seq++
		tk := &fakeTask{Labels: labelFlags(args), ID: fmt.Sprintf("T%d", f.seq), Project: argFlag(args, "--project"), Created: fmt.Sprintf("2026-10-01T00:00:%02d", f.seq), Desc: string(raw)}
		f.tasks = append(f.tasks, tk)
		if f.registerOnCreate {
			if rl, hit := requestOf(tk.Desc); hit {
				f.registry[rl.Key] = registryAnswer{Status: "registered", TaskID: tk.ID, Identity: rl.Identity}
			}
		}
		if f.afterCreate != nil {
			f.afterCreate(f, tk.ID)
		}
		return okRes(map[string]any{"task": map[string]string{"id": tk.ID, "projectId": tk.Project}})
	case cmd == "tasks update "+args[2]:
		for _, tk := range f.tasks {
			if tk.ID == args[2] && !tk.Deleted {
				raw, err := os.ReadFile(argFlag(args, "--description-file"))
				if err != nil {
					return *badRes(1, err.Error())
				}
				tk.Desc = string(raw)
				tk.Labels = append(tk.Labels, argFlag(args, "--add-label"))
				return okRes(map[string]any{"task": map[string]string{"id": tk.ID}})
			}
		}
		return *badRes(1, "task not found")
	case cmd == "tasks comment "+args[2]:
		for _, tk := range f.tasks {
			if tk.ID == args[2] && !tk.Deleted {
				kind := "user"
				if thread != "" {
					kind = "agent"
				}
				c := Comment{ID: fmt.Sprintf("c%d", len(tk.Comments)+1), Kind: kind, ThreadID: thread, CreatedAt: fmt.Sprintf("2026-10-01T01:00:%02d", len(tk.Comments))}
				tk.Comments = append(tk.Comments, c)
				return okRes(map[string]any{"comment": c})
			}
		}
		return *badRes(1, "task not found")
	}
	f.t.Errorf("memBB: unexpected call %v", args)
	return *badRes(1, "unexpected")
}

func (f *memBB) cards() int {
	n := 0
	for _, tk := range f.tasks {
		if !tk.Deleted {
			n++
		}
	}
	return n
}

func testReq() CardRequest {
	return CardRequest{
		Project: "P1", Title: "Ship the thing?", Key: "0b9d1c6e-0000-4000-8000-000000000001", Thread: "thr_a",
		Blocks: []string{"thread:thr_a", "bead:Autarch-1"},
		Ask:    map[string]any{"project": "autarch", "project_root": "/srv/autarch", "question": "Ship it?", "options": []any{map[string]any{"id": "yes", "label": "Yes", "kind": "ruling-only"}, map[string]any{"id": "no", "label": "No", "kind": "ruling-only"}}},
	}
}

func wantErrIs(t *testing.T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("err = %v, want %v", err, target)
	}
}

func TestCardFilerFilesLabelsCreatesAndComments(t *testing.T) {
	f := newFake(t)
	res, err := f.filer().FileCard(context.Background(), testReq())
	if err != nil || res.Replay || res.ID != "T1" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if !f.labels[NeedsMkLabel] {
		t.Fatal("label not ensured")
	}
	tk := f.tasks[0]
	if len(tk.Comments) != 1 || tk.Comments[0].Kind != "agent" || tk.Comments[0].ThreadID != "thr_a" {
		t.Fatalf("comments %+v", tk.Comments)
	}
	c, err := ParseCard(tk.Desc)
	if err != nil {
		t.Fatalf("description does not round-trip: %v\n%s", err, tk.Desc)
	}
	if c.Request.Key != testReq().Key || c.Pull != "" || len(c.Blocks) != 2 {
		t.Fatalf("card %+v", c)
	}
	// the create call carries every flag the plan names
	for _, call := range f.calls {
		if len(call) > 2 && call[1] == "create" && (argFlag(call, "--title") != "Ship the thing?" || argFlag(call, "--project") != "P1") {
			t.Fatalf("create args %v", call)
		}
	}
}

func TestCardFilerLabelAlreadyInUseIsSuccess(t *testing.T) {
	f := newFake(t)
	f.fail = func(a []string) *BBResult {
		if strings.HasPrefix(strings.Join(a, " "), "tasks label list") {
			return &BBResult{Stdout: []byte(`{"labels":[]}`)}
		}
		return nil
	}
	f.labels[NeedsMkLabel] = true // exists, but the list answer above hides it
	if _, err := f.filer().FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
}

func TestCardFilerReplaySameThreadIsExit0AndCreatesNothing(t *testing.T) {
	f := newFake(t)
	fl := f.filer()
	if _, err := fl.FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
	res, err := fl.FileCard(context.Background(), testReq())
	if err != nil || !res.Replay || res.ID != "T1" || f.cards() != 1 || len(f.tasks[0].Comments) != 1 {
		t.Fatalf("res=%+v err=%v cards=%d comments=%d", res, err, f.cards(), len(f.tasks[0].Comments))
	}
}

func TestCardFilerReplayFromAnotherThreadIsRefused(t *testing.T) {
	f := newFake(t)
	if _, err := f.filer().FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
	r := testReq()
	r.Thread = "thr_b"
	_, err := f.filer().FileCard(context.Background(), r)
	wantErrIs(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "filed from thread thr_a") || len(f.tasks[0].Comments) != 1 {
		t.Fatalf("err=%v comments=%d", err, len(f.tasks[0].Comments))
	}
}

func TestCardFilerCommentFailureIsExit4AndRerunRepairsBeforeSuccess(t *testing.T) {
	f := newFake(t)
	failComment := true
	f.fail = func(a []string) *BBResult {
		if failComment && len(a) > 1 && a[1] == "comment" {
			return badRes(1, "boom")
		}
		return nil
	}
	_, err := f.filer().FileCard(context.Background(), testReq())
	wantErrIs(t, err, ErrOutcomeUnknown)
	if f.cards() != 1 || len(f.tasks[0].Comments) != 0 {
		t.Fatalf("cards=%d comments=%d", f.cards(), len(f.tasks[0].Comments))
	}
	failComment = false
	res, err := f.filer().FileCard(context.Background(), testReq())
	if err != nil || !res.Replay || f.cards() != 1 || len(f.tasks[0].Comments) != 1 || f.tasks[0].Comments[0].ThreadID != "thr_a" {
		t.Fatalf("res=%+v err=%v comments=%+v", res, err, f.tasks[0].Comments)
	}
}

func TestCardFilerNonAgentCommentIsExit4(t *testing.T) {
	f := newFake(t)
	f.fail = func(a []string) *BBResult {
		if len(a) > 1 && a[1] == "comment" {
			return &BBResult{Stdout: []byte(`{"comment":{"id":"c1","kind":"user","threadId":""}}`)}
		}
		return nil
	}
	_, err := f.filer().FileCard(context.Background(), testReq())
	wantErrIs(t, err, ErrOutcomeUnknown)
}

func TestCardFilerLostCreateAnswerLooksAgainUnderTheLock(t *testing.T) {
	f := newFake(t)
	lost := true
	f.fail = nil
	orig := f.afterCreate
	_ = orig
	fl := f.filer()
	inner := fl.Run
	fl.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		r := inner(ctx, env, args...)
		if lost && len(args) > 1 && args[1] == "create" {
			return BBResult{TimedOut: true} // the card was written, the answer was lost
		}
		return r
	}
	res, err := fl.FileCard(context.Background(), testReq())
	if err != nil || !res.Replay || f.cards() != 1 || len(f.tasks[0].Comments) != 1 {
		t.Fatalf("res=%+v err=%v cards=%d", res, err, f.cards())
	}
}

func TestCardFilerCreateFailureWithNoCardIsExit3(t *testing.T) {
	f := newFake(t)
	f.fail = func(a []string) *BBResult {
		if len(a) > 1 && a[1] == "create" {
			return badRes(1, "connection refused")
		}
		return nil
	}
	_, err := f.filer().FileCard(context.Background(), testReq())
	wantErrIs(t, err, ErrHomeDown)
	if f.cards() != 0 {
		t.Fatal("a card was created")
	}
}

func TestCardFilerRegistryUnavailableCreatesNothing(t *testing.T) {
	for name, fail := range map[string]func([]string) *BBResult{
		"exit": func(a []string) *BBResult { return pickCall(a, "home get", badRes(1, "not ready")) },
		"garbage": func(a []string) *BBResult {
			return pickCall(a, "home get", &BBResult{Stdout: []byte(`{"status":"weird"}`)})
		},
		"timeout": func(a []string) *BBResult { return pickCall(a, "home get", &BBResult{TimedOut: true}) },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			f.fail = fail
			_, err := f.filer().FileCard(context.Background(), testReq())
			wantErrIs(t, err, ErrHomeDown)
			if f.cards() != 0 || f.count("tasks", "create") != 0 || f.count("tasks", "list") != 0 {
				t.Fatalf("cards=%d calls=%v", f.cards(), f.calls)
			}
		})
	}
}

func pickCall(a []string, prefix string, r *BBResult) *BBResult {
	if strings.HasPrefix(strings.Join(a, " "), prefix) {
		return r
	}
	return nil
}

func TestCardFilerRegisteredCardDeletedOrEditedIsExit2OrExit3(t *testing.T) {
	f := newFake(t)
	fl := f.filer()
	if _, err := fl.FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
	// edited: the Request line no longer matches the registry
	f.tasks[0].Desc = strings.Replace(f.tasks[0].Desc, "sha256:", "sha256:0000", 1)
	_, err := fl.FileCard(context.Background(), testReq())
	wantErrIs(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "is registered to card T1, which was deleted or changed; use a new Request") {
		t.Fatalf("err=%v", err)
	}
	// deleted
	f.tasks[0].Deleted = true
	_, err = fl.FileCard(context.Background(), testReq())
	wantErrIs(t, err, ErrInvalid)
	if f.cards() != 0 {
		t.Fatal("a replacement card was created")
	}
	// Home down: nothing is decided from tasks alone
	f.fail = func(a []string) *BBResult { return pickCall(a, "home get", badRes(1, "down")) }
	_, err = fl.FileCard(context.Background(), testReq())
	wantErrIs(t, err, ErrHomeDown)
}

func TestCardFilerLegacyRequestIsExit2(t *testing.T) {
	f := newFake(t)
	r := testReq()
	f.legacy[r.Key] = registryAnswer{Status: "legacy", DecisionID: "dec_9", State: "open"}
	_, err := f.filer().FileCard(context.Background(), r)
	wantErrIs(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "pre-cards ask dec_9") || f.cards() != 0 || f.count("tasks", "list") != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestCardFilerAbsentButDeletedBeforeMaterializationCreates(t *testing.T) {
	f := newFake(t)
	f.registerOnCreate = false
	fl := f.filer()
	if _, err := fl.FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
	f.tasks[0].Deleted = true
	res, err := fl.FileCard(context.Background(), testReq())
	if err != nil || res.Replay || f.cards() != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

// seedDesc renders the description of a card with the given key and identity.
func seedDesc(t *testing.T, key, ident string) string {
	r := testReq()
	r.Key = key
	d, i, err := r.Description()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Replace(d, "sha256:"+i, "sha256:"+ident, 1)
}

func TestCardFilerPaginationFindsTheMatchBehindSubstringDecoys(t *testing.T) {
	f := newFake(t)
	f.registerOnCreate = false
	r := testReq()
	_, ident, _ := r.Description()
	for i := 0; i < 6; i++ {
		f.tasks = append(f.tasks, &fakeTask{ID: fmt.Sprintf("D%d", i), Project: "P1", Created: "2026-09-01T00:00:0" + strconv.Itoa(i), Desc: seedDesc(t, r.Key+"-"+strconv.Itoa(i), ident)})
	}
	f.tasks = append(f.tasks, &fakeTask{ID: "REAL", Project: "P1", Created: "2026-09-02T00:00:00", Desc: seedDesc(t, r.Key, ident),
		Comments: []Comment{{ID: "c", Kind: "agent", ThreadID: "thr_a", CreatedAt: "2026-09-02T01:00:00"}}})
	res, err := f.filer().FileCard(context.Background(), r)
	if err != nil || !res.Replay || res.ID != "REAL" || f.count("tasks", "create") != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if f.count("tasks", "list") < 4 {
		t.Fatalf("pages walked: %d", f.count("tasks", "list"))
	}
}

func TestCardFilerSubstringOnlyHitIsNotAMatch(t *testing.T) {
	f := newFake(t)
	f.registerOnCreate = false
	r := testReq()
	_, ident, _ := r.Description()
	f.tasks = append(f.tasks, &fakeTask{ID: "LONG", Project: "P1", Created: "2026-09-01T00:00:00", Desc: seedDesc(t, r.Key+"-extra", ident)})
	res, err := f.filer().FileCard(context.Background(), r)
	if err != nil || res.Replay || f.cards() != 2 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestCardFilerPageFailureIsExit3AndCreatesNothing(t *testing.T) {
	f := newFake(t)
	f.registerOnCreate = false
	r := testReq()
	_, ident, _ := r.Description()
	for i := 0; i < 5; i++ {
		f.tasks = append(f.tasks, &fakeTask{ID: fmt.Sprintf("D%d", i), Project: "P1", Created: "2026-09-01T00:00:0" + strconv.Itoa(i), Desc: seedDesc(t, r.Key+"-"+strconv.Itoa(i), ident)})
	}
	f.fail = func(a []string) *BBResult {
		if argFlag(a, "--cursor") == "2" {
			return badRes(1, "page 2 failed")
		}
		return nil
	}
	_, err := f.filer().FileCard(context.Background(), r)
	wantErrIs(t, err, ErrHomeDown)
	if f.count("tasks", "create") != 0 {
		t.Fatal("created after an incomplete search")
	}
}

func TestCardFilerIdentityMismatchIsExit2BeforeAnyCreate(t *testing.T) {
	f := newFake(t)
	fl := f.filer()
	if _, err := fl.FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
	r := testReq()
	r.Title = "Other title"
	_, err := fl.FileCard(context.Background(), r)
	wantErrIs(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "Request reused for a different card") || f.cards() != 1 {
		t.Fatalf("err=%v cards=%d", err, f.cards())
	}
	// and through the search path (no registry row)
	f.registry = map[string]registryAnswer{}
	_, err = fl.FileCard(context.Background(), r)
	wantErrIs(t, err, ErrInvalid)
}

func TestCardFilerKeepsKeyMatchWhateverItsIdentityAndPicksEarliest(t *testing.T) {
	f := newFake(t)
	f.registerOnCreate = false
	f.projects = []string{"P1", "P2"}
	r := testReq()
	_, ident, _ := r.Description()
	f.tasks = append(f.tasks,
		&fakeTask{ID: "LATE", Project: "P1", Created: "2026-09-05T00:00:00", Desc: seedDesc(t, r.Key, ident)},
		&fakeTask{ID: "EARLY", Project: "P2", Created: "2026-09-03T00:00:00", Desc: seedDesc(t, r.Key, "ffffffffffffffff")},
	)
	_, err := f.filer().FileCard(context.Background(), r)
	wantErrIs(t, err, ErrInvalid) // earliest carries another identity: reused
	if f.count("tasks", "create") != 0 {
		t.Fatal("created")
	}
}

func TestCardFilerSearchesEveryProjectInOneCall(t *testing.T) {
	f := newFake(t)
	f.registerOnCreate = false
	f.projects = []string{"P1", "P2", "P3"}
	r := testReq()
	_, ident, _ := r.Description()
	f.tasks = append(f.tasks, &fakeTask{ID: "INB", Project: "P3", Created: "2026-09-03T00:00:00", Desc: seedDesc(t, r.Key, ident),
		Comments: []Comment{{ID: "c", Kind: "agent", ThreadID: "thr_a", CreatedAt: "2026-09-03T01:00:00"}}})
	res, err := f.filer().FileCard(context.Background(), r)
	if err != nil || !res.Replay || res.ID != "INB" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	for _, c := range f.calls {
		if len(c) > 2 && c[1] == "list" {
			if argFlag(c, "--project") != "" || argFlag(c, "--status") != searchStatuses || argFlag(c, "--search") != "Request: "+r.Key {
				t.Fatalf("list args %v", c)
			}
		}
	}
	// One list call (one page), no per-project loop, no project listing for the search.
	if n := f.count("tasks", "list"); n != 1 {
		t.Fatalf("tasks list calls: %d", n)
	}
	if n := f.count("tasks", "project"); n != 0 {
		t.Fatalf("tasks project calls: %d", n)
	}
}

func TestCardFilerBBInvocationsDoNotGrowWithProjectCount(t *testing.T) {
	calls := func(projects int) int {
		f := newFake(t)
		f.projects = nil
		for i := 0; i < projects; i++ {
			f.projects = append(f.projects, fmt.Sprintf("P%d", i))
		}
		if _, err := f.filer().FileCard(context.Background(), testReq()); err != nil {
			t.Fatal(err)
		}
		return len(f.calls)
	}
	if small, big := calls(2), calls(150); small != big {
		t.Fatalf("bb invocations depend on the project count: %d with 2 projects, %d with 150", small, big)
	}
}

func TestCardFilerAFailingListIsExit3(t *testing.T) {
	f := newFake(t)
	f.fail = func(a []string) *BBResult {
		if len(a) > 2 && a[1] == "list" {
			return badRes(1, "no")
		}
		return nil
	}
	_, err := f.filer().FileCard(context.Background(), testReq())
	wantErrIs(t, err, ErrHomeDown)
	if f.count("tasks", "create") != 0 {
		t.Fatal("created")
	}
}

func TestCardFilerUnparseableHitWithTheExactKeyIsExit2(t *testing.T) {
	f := newFake(t)
	f.registerOnCreate = false
	r := testReq()
	f.tasks = append(f.tasks, &fakeTask{ID: "BROKEN", Project: "P1", Created: "2026-09-01T00:00:00", Desc: "hi\n\nRequest: " + r.Key + " sha256:zzz\n```home-ask\n{\n```\n"})
	_, err := f.filer().FileCard(context.Background(), r)
	wantErrIs(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "appears on unparseable card BROKEN") || f.count("tasks", "create") != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestCardFilerReplayWithNoAgentCommentPostsOneBeforeSuccess(t *testing.T) {
	f := newFake(t)
	f.registerOnCreate = false
	r := testReq()
	d, _, _ := r.Description()
	f.tasks = append(f.tasks, &fakeTask{ID: "NOC", Project: "P1", Created: "2026-09-01T00:00:00", Desc: d,
		Comments: []Comment{{ID: "u", Kind: "user", CreatedAt: "2026-09-01T00:00:01"}}})
	res, err := f.filer().FileCard(context.Background(), r)
	if err != nil || !res.Replay || len(f.tasks[0].Comments) != 2 || f.tasks[0].Comments[1].Kind != "agent" {
		t.Fatalf("res=%+v err=%v %+v", res, err, f.tasks[0].Comments)
	}
}

func TestCardFilerRoutesByEarliestAgentCommentNotLatest(t *testing.T) {
	f := newFake(t)
	r := testReq()
	d, _, _ := r.Description()
	rl, _ := requestOf(d)
	f.registry[r.Key] = registryAnswer{Status: "registered", TaskID: "X", Identity: rl.Identity}
	f.tasks = append(f.tasks, &fakeTask{ID: "X", Project: "P1", Created: "2026-09-01T00:00:00", Desc: d, Comments: []Comment{
		{ID: "a2", Kind: "agent", ThreadID: "thr_late", CreatedAt: "2026-09-01T02:00:00"},
		{ID: "a1", Kind: "agent", ThreadID: "thr_a", CreatedAt: "2026-09-01T01:00:00"},
	}})
	if _, err := f.filer().FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
}

func TestCardFilerHoldsTheLockForTheWholeFiling(t *testing.T) {
	f := newFake(t)
	fl := f.filer()
	held := 0
	inner := fl.Run
	fl.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		p, _ := fl.lockPath(testReq().Key)
		if tryLock(p) {
			t.Errorf("lock not held during bb %v", args)
		} else {
			held++
		}
		return inner(ctx, env, args...)
	}
	if _, err := fl.FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
	if held < 5 {
		t.Fatalf("lock checked %d times", held)
	}
}

func TestCardFilerConcurrentFilersProduceOneCard(t *testing.T) {
	f := newFake(t)
	fl := f.filer()
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = fl.FileCard(context.Background(), testReq())
		}(i)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if f.cards() != 1 || len(f.tasks[0].Comments) != 1 {
		t.Fatalf("cards=%d comments=%d", f.cards(), len(f.tasks[0].Comments))
	}
}

func TestCardFilerRefusesWhatTheCardCannotCarry(t *testing.T) {
	f := newFake(t)
	for name, mut := range map[string]func(*CardRequest){
		"no thread":        func(r *CardRequest) { r.Thread = "" },
		"no title":         func(r *CardRequest) { r.Title = " " },
		"bad key":          func(r *CardRequest) { r.Key = "bad key" },
		"v1 field":         func(r *CardRequest) { r.Ask["steps"] = []any{"x"} },
		"pull from thread": func(r *CardRequest) { r.Ask["pull"] = "mycroft" },
		"blocks lookalike": func(r *CardRequest) { r.Ask["question"] = "Blocks: nothing\nreally?" },
	} {
		r := testReq()
		mut(&r)
		_, err := f.filer().FileCard(context.Background(), r)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("bb was run: %v", f.calls)
	}
}

func TestCardFilerIdentityIgnoresBlockOrderAndTasksProject(t *testing.T) {
	a, b := testReq(), testReq()
	b.Blocks = []string{"bead:Autarch-1", "thread:thr_a"}
	b.Project = "P2"
	ia, _ := RequestIdentity(a)
	ib, _ := RequestIdentity(b)
	if ia != ib || len(ia) != 16 {
		t.Fatalf("%s %s", ia, ib)
	}
	b.Ask["question"] = "different"
	if ic, _ := RequestIdentity(b); ic == ia {
		t.Fatal("identity ignores the question")
	}
}

func TestCardFilerRootRunRoundTrips(t *testing.T) {
	f := newFake(t)
	r := testReq()
	r.RootRun = &RootRun{Script: "/opt/x.sh", SHA256: strings.Repeat("a", 64), Timeout: 60, Set: "deploy"}
	if _, err := f.filer().FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	c, err := ParseCard(f.tasks[0].Desc)
	if err != nil || c.RootRun == nil || c.RootRun.Set != "deploy" {
		t.Fatalf("%+v %v", c, err)
	}
}

// Key extraction agrees with the strict parser over the shared vectors.
func TestCardFilerRequestKeyAgreesWithParserVectors(t *testing.T) {
	n := 0
	for _, cases := range loadCardFixtures(t, "card") {
		for _, raw := range cases {
			var c cardCase
			if err := json.Unmarshal(raw, &c); err != nil {
				t.Fatal(err)
			}
			card, err := ParseCard(c.Description)
			if err != nil {
				continue
			}
			rl, hit := requestOf(c.Description)
			if !hit || rl != card.Request {
				t.Errorf("%s: requestOf=%+v ok=%v parser=%+v", c.Name, rl, hit, card.Request)
			}
			n++
		}
	}
	if n == 0 {
		t.Fatal("no vectors ran")
	}
}

func TestCardFilerFileFromAsk(t *testing.T) {
	f := newFake(t)
	a := Ask{V: 1, Kind: "decide", Asker: "thread", Thread: "thr_a", Project: "autarch", ProjectRoot: "/srv/autarch", Question: "Ship it?", Options: []Option{{ID: "yes", Label: "Yes", Kind: "ruling-only"}, {ID: "no", Label: "No", Kind: "ruling-only"}}}
	id, err := f.filer().File(context.Background(), a)
	if err != nil || id != "T1" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	id2, err := f.filer().File(context.Background(), a)
	if err != nil || id2 != "T1" || f.cards() != 1 {
		t.Fatalf("replay id=%q err=%v cards=%d", id2, err, f.cards())
	}
}

func TestFileForPullIsThreadlessAndPostsNoComment(t *testing.T) {
	f := newFake(t)
	a := Ask{V: 1, Kind: "decide", Asker: "mycroft", Project: "autarch", ProjectRoot: "/srv/autarch", Question: "Merge it?", Options: []Option{{ID: "yes", Label: "Yes", Kind: "ruling-only"}, {ID: "no", Label: "No", Kind: "ruling-only"}}, RequestID: "mycroft:autarch:bead1:agent1"}
	id, err := f.filer().FileForPull(context.Background(), a)
	if err != nil || id != "T1" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if len(f.tasks[0].Comments) != 0 || f.count("tasks", "comment") != 0 {
		t.Fatal("a pull filing posted a comment")
	}
	c, err := ParseCard(f.tasks[0].Desc)
	if err != nil || c.Pull != "mycroft" || c.Request.Key != PullKey(a.RequestID) {
		t.Fatalf("%+v %v", c, err)
	}
	if id2, err := f.filer().FileForPull(context.Background(), a); err != nil || id2 != "T1" || f.cards() != 1 {
		t.Fatalf("replay %q %v", id2, err)
	}
}

func TestFileForPullSkipsFilingWhileTheLegacyAskIsOpen(t *testing.T) {
	f := newFake(t)
	a := Ask{V: 1, Kind: "decide", Asker: "mycroft", Project: "autarch", ProjectRoot: "/srv/autarch", Question: "Merge it?", Options: []Option{{ID: "yes", Label: "Yes", Kind: "ruling-only"}, {ID: "no", Label: "No", Kind: "ruling-only"}}, RequestID: "mycroft:autarch:bead1:agent1"}
	f.legacy[a.RequestID] = registryAnswer{Status: "legacy", DecisionID: "dec_7", State: "open"}
	id, err := f.filer().FileForPull(context.Background(), a)
	if err != nil || id != "dec_7" || f.cards() != 0 || f.count("tasks") != 0 {
		t.Fatalf("id=%q err=%v calls=%v", id, err, f.calls)
	}
	f.legacy[a.RequestID] = registryAnswer{Status: "legacy", DecisionID: "dec_7", State: "ruled"}
	_, err = f.filer().FileForPull(context.Background(), a)
	wantErrIs(t, err, ErrAlreadyRuled)
	// not ready: nothing filed
	f.fail = func(x []string) *BBResult { return pickCall(x, "home get", badRes(1, "down")) }
	_, err = f.filer().FileForPull(context.Background(), a)
	wantErrIs(t, err, ErrHomeDown)
	// a thread ask cannot use the pull path, nor a pull ask the thread path
	if _, err := f.filer().FileForPull(context.Background(), Ask{Asker: "thread", RequestID: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := f.filer().File(context.Background(), a); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestCardFilerTasksProjectIsConfiguredNotNameMatched(t *testing.T) {
	f := newFake(t)
	f.projects = []string{"P1", "autarch"} // a tasks project named like the Home project must not be guessed
	a := Ask{V: 1, Kind: "decide", Asker: "thread", Thread: "thr_a", Project: "autarch", ProjectRoot: "/srv/autarch", Question: "Ship it?", Options: []Option{{ID: "yes", Label: "Yes", Kind: "ruling-only"}, {ID: "no", Label: "No", Kind: "ruling-only"}}}
	unbound := &CardFiler{Run: f.run, LockDir: t.TempDir()}
	if _, err := unbound.File(context.Background(), a); !errors.Is(err, ErrInvalid) || len(f.calls) != 0 {
		t.Fatalf("unbound: err=%v calls=%v", err, f.calls)
	}
	bound := &CardFiler{Run: f.run, LockDir: t.TempDir(), TasksProject: "other", TasksProjects: map[string]string{"autarch": "P1"}}
	if _, err := bound.File(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if got := f.tasks[0].Project; got != "P1" {
		t.Fatalf("filed into %q, want the bound P1 (mapping beats default)", got)
	}
}

func TestCardFilerFromEnvReadsTheBindingAndRefusesAMalformedOne(t *testing.T) {
	t.Setenv(EnvTasksProject, "DEF")
	t.Setenv(EnvTasksProjects, "Alpha=ABC, beta = XYZ")
	f := CardFilerFromEnv(0)
	if p, err := f.tasksProject("ALPHA"); err != nil || p != "ABC" {
		t.Fatalf("%q %v", p, err)
	}
	if p, err := f.tasksProject("other"); err != nil || p != "DEF" {
		t.Fatalf("%q %v", p, err)
	}
	for _, bad := range []string{"alpha", "alpha=", "=x", "a=b,A=c"} {
		t.Setenv(EnvTasksProjects, bad)
		if _, err := CardFilerFromEnv(0).tasksProject("alpha"); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

// The real `bb tasks show` text for a deleted card (plugins/tasks/cli/index.ts, code task_not_found) is
// "task not found: <address>". notFoundRe must keep matching it: a registered-but-deleted card is exit 2, never exit 3.
func TestNotFoundReMatchesRealTasksShowText(t *testing.T) {
	for _, text := range []string{"task not found: 01JABCDEFGHJKMNPQRSTVWXYZ0\n", "task not found: ABC-12"} {
		if !notFoundRe.MatchString(text) {
			t.Fatalf("notFoundRe does not match the real bb tasks show error %q", text)
		}
	}
	for _, text := range []string{"connection refused", "plugin tasks is not running"} {
		if notFoundRe.MatchString(text) {
			t.Fatalf("notFoundRe matches %q, which is not a deleted card", text)
		}
	}
}

// Real-bb finding (Task 2.12 part B): in a read-only sandbox /tmp cannot be written, so the filer's default lock
// directory failed with "read-only file system" and the filer reported "home is down". It now falls back to the
// directory os.TempDir() names (TMPDIR) when the default is unwritable.
func TestCardFilerLockDirFallsBackWhenDefaultUnwritable(t *testing.T) {
	base := t.TempDir()
	// The sandbox case: the default directory already exists (made before the sandbox) but cannot be written.
	existing := filepath.Join(base, fmt.Sprintf("autarch-needsmk-%d", os.Getuid()))
	if err := os.MkdirAll(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existing, 0o500); err != nil {
		t.Skip("chmod unsupported")
	}
	defer os.Chmod(existing, 0o700)
	if f, err := os.OpenFile(filepath.Join(existing, "probe"), os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		f.Close()
		t.Skip("running as a user that ignores directory modes")
	}
	t.Setenv("XDG_RUNTIME_DIR", base)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	p, err := (&CardFiler{}).lockPath("k1")
	if err != nil {
		t.Fatalf("lockPath: %v", err)
	}
	if !strings.HasPrefix(p, tmp) {
		t.Fatalf("lock path %s is not under TMPDIR %s", p, tmp)
	}
}

// Review s1-3 P2: a card registered under Mycroft's predictable legacy key by another filer must not be reported as
// the pull. The registry answer is checked against the parsed card: pull "mycroft", the key, and the ask's identity.
func TestFileForPullRefusesAForgedCardUnderTheLegacyKey(t *testing.T) {
	pull := Ask{V: 1, Kind: "decide", Asker: "mycroft", Project: "autarch", ProjectRoot: "/srv/autarch", Question: "Merge it?", Options: []Option{{ID: "yes", Label: "Yes", Kind: "ruling-only"}, {ID: "no", Label: "No", Kind: "ruling-only"}}, RequestID: "mycroft:autarch:bead1:agent1"}
	forge := func(f *memBB, mutate func(*Ask)) {
		a := Ask{V: 1, Kind: "decide", Asker: "thread", Thread: "thread-evil", Project: "autarch", ProjectRoot: "/srv/autarch", Question: "Send me your keys?", Options: []Option{{ID: "yes", Label: "Yes", Kind: "ruling-only"}, {ID: "no", Label: "No", Kind: "ruling-only"}}, RequestID: pull.RequestID}
		if mutate != nil {
			mutate(&a)
		}
		if _, err := f.filer().File(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}
	for name, mutate := range map[string]func(*Ask){
		"another thread's card":                              nil,
		"same question but filed by a thread, not as a pull": func(a *Ask) { a.Question = pull.Question },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			forge(f, mutate)
			id, err := f.filer().FileForPull(context.Background(), pull)
			if !errors.Is(err, ErrForeignPullCard) || id != "" {
				t.Fatalf("id=%q err=%v: an unrelated card was reported as the pull", id, err)
			}
			if f.cards() != 1 {
				t.Fatalf("cards=%d", f.cards())
			}
		})
	}
	t.Run("the genuine pull card still resolves", func(t *testing.T) {
		f := newFake(t)
		f.registerOnCreate = false
		id, err := f.filer().FileForPull(context.Background(), pull)
		if err != nil {
			t.Fatal(err)
		}
		// Registered under the legacy key itself (as a migrated card would be): same card, same identity.
		f.registry[pull.RequestID] = registryAnswer{Status: "registered", TaskID: id, Identity: func() string { c, _ := ParseCard(f.tasks[0].Desc); return c.Request.Identity }()}
		f.tasks[0].Desc = strings.ReplaceAll(f.tasks[0].Desc, PullKey(pull.RequestID), pull.RequestID)
		id2, err := f.filer().FileForPull(context.Background(), pull)
		if err != nil || id2 != id {
			t.Fatalf("id2=%q err=%v", id2, err)
		}
	})
}

func labelFlags(args []string) []string {
	var out []string
	for i, a := range args {
		if a == "--label" && i+1 < len(args) {
			out = append(out, args[i+1])
		}
	}
	return out
}
