package spawn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

type fakeResolver struct {
	seat Seat
	err  error
	n    int
}

func (f *fakeResolver) Resolve(_ context.Context, role string) (Seat, error) {
	f.n++
	s := f.seat
	s.Role = role
	return s, f.err
}

type call struct {
	name string
	args []string
}

func stub(calls *[]call, replies map[string]string, fail map[string]error) Runner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, call{name, args})
		key := name + " " + args[0] + " " + args[1]
		if err := fail[key]; err != nil {
			return nil, err
		}
		return []byte(replies[key]), nil
	}
}

var seat = Seat{ProfileRef: "routine-exec-sonnet", Backend: "claude", Model: "claude-sonnet-5-5", Effort: "high", PolicyHash: "abc"}
var bead = mycroft.BeadView{ID: "mk-1", Title: "fix thing", Type: "task", Priority: 2, Card: "AUTA-9"}

func newTest(r SeatResolver, run Runner, receipts string) *BBSpawner {
	s := NewBBSpawner("proj", "env", receipts)
	s.Resolver, s.run = r, run
	return s
}

func argOf(c call, flag string) string {
	for i, a := range c.args {
		if a == flag && i+1 < len(c.args) {
			return c.args[i+1]
		}
	}
	return ""
}

func TestSpawnCreatesThreadWithRoutedSeatAndAttachesCard(t *testing.T) {
	var calls []call
	rf := filepath.Join(t.TempDir(), "r.jsonl")
	s := newTest(&fakeResolver{seat: seat}, stub(&calls, map[string]string{"bb thread create": `{"id":"thr_x"}`}, nil), rf)
	id, err := s.Spawn("a", bead, "")
	if err != nil || id != "thr_x" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls: %v", calls)
	}
	c := calls[0]
	for flag, want := range map[string]string{"--project": "proj", "--environment": "env", "--provider": "claude-code", "--model": "claude-sonnet-5-5", "--reasoning-level": "high", "--title": "AUTA-9 | mk-1 | fix thing"} {
		if got := argOf(c, flag); got != want {
			t.Errorf("%s = %q, want %q", flag, got, want)
		}
	}
	if got := strings.Join(calls[1].args, " "); got != "tasks attach AUTA-9 --thread thr_x" {
		t.Errorf("attach: %s", got)
	}
	b, _ := os.ReadFile(rf)
	for _, w := range []string{`"thread":"thr_x"`, `"policy_hash":"abc"`, `"model":"claude-sonnet-5-5"`, `"effort":"high"`, `"role":"routine-execution"`} {
		if !strings.Contains(string(b), w) {
			t.Errorf("receipt lacks %s: %s", w, b)
		}
	}
	d := s.Detail("thr_x")
	if !strings.Contains(d, "policy=abc") || !strings.Contains(d, "effort=high") {
		t.Errorf("detail: %q", d)
	}
}

func TestSpawnFailsClosedWhenRoutingCannotResolve(t *testing.T) {
	var calls []call
	s := newTest(&fakeResolver{err: errors.New("no profile")}, stub(&calls, nil, nil), "")
	if _, err := s.Spawn("a", bead, ""); err == nil || !strings.Contains(err.Error(), "seat unresolved") {
		t.Fatalf("err=%v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("bb was called: %v", calls)
	}
}

func TestSpawnRefusesUnknownBackend(t *testing.T) {
	var calls []call
	bad := seat
	bad.Backend = "weird"
	s := newTest(&fakeResolver{seat: bad}, stub(&calls, nil, nil), "")
	if _, err := s.Spawn("a", bead, ""); err == nil || len(calls) != 0 {
		t.Fatalf("err=%v calls=%v", err, calls)
	}
}

func TestSpawnStopsThreadWhenCardAttachFails(t *testing.T) {
	var calls []call
	s := newTest(&fakeResolver{seat: seat}, stub(&calls, map[string]string{"bb thread create": `{"thread":{"id":"thr_y"}}`}, map[string]error{"bb tasks attach": errors.New("nope")}), "")
	if _, err := s.Spawn("a", bead, ""); err == nil {
		t.Fatal("want error")
	}
	last := calls[len(calls)-1]
	if strings.Join(last.args, " ") != "thread stop thr_y" {
		t.Fatalf("last call: %v", last)
	}
}

func TestSpawnNeedsProjectAndEnvironment(t *testing.T) {
	s := newTest(&fakeResolver{seat: seat}, nil, "")
	s.Project = ""
	if _, err := s.Spawn("a", bead, ""); err == nil {
		t.Fatal("want error")
	}
}

func TestPlanStartsNothing(t *testing.T) {
	var calls []call
	s := newTest(&fakeResolver{seat: seat}, stub(&calls, nil, nil), "")
	title, st, brief, err := s.Plan(bead)
	if err != nil || len(calls) != 0 || st.Model == "" || !strings.Contains(title, "AUTA-9") || !strings.Contains(brief, "mk-1") {
		t.Fatalf("%v %v %v %v", title, st, err, calls)
	}
}

func TestICResolverParsesRoutingAnswer(t *testing.T) {
	reply := `{"requested_role":"routine-execution","profile_ref":"r","profile":{"backend":"claude","model":"m","reasoning_effort":"high"},"policy_hash":"h","frontier_required":false}`
	r := &ICResolver{Policy: "/p/routing.yaml", run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "ic" || !strings.Contains(strings.Join(args, " "), "--role=routine-execution") || !strings.Contains(strings.Join(args, " "), "--policy=/p/routing.yaml") {
			t.Errorf("args: %v", args)
		}
		return []byte(reply), nil
	}}
	s, err := r.Resolve(context.Background(), "routine-execution")
	if err != nil || s.Model != "m" || s.Effort != "high" || s.PolicyHash != "h" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestICResolverFailsClosed(t *testing.T) {
	ok := func(out string, err error) *ICResolver {
		return &ICResolver{Policy: "/p", run: func(context.Context, string, ...string) ([]byte, error) { return []byte(out), err }}
	}
	cases := map[string]*ICResolver{
		"ic error":   ok("", errors.New("no dispatch profile found")),
		"garbage":    ok("not json", nil),
		"no hash":    ok(`{"requested_role":"routine-execution","profile":{"backend":"claude","model":"m","reasoning_effort":"high"}}`, nil),
		"wrong role": ok(`{"requested_role":"other","profile":{"backend":"claude","model":"m","reasoning_effort":"high"},"policy_hash":"h"}`, nil),
		"frontier":   ok(`{"requested_role":"routine-execution","profile":{"backend":"claude","model":"m","reasoning_effort":"high"},"policy_hash":"h","frontier_required":true}`, nil),
		"no policy":  {},
	}
	for name, r := range cases {
		if _, err := r.Resolve(context.Background(), "routine-execution"); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := ok("{}", nil).Resolve(context.Background(), ""); err == nil {
		t.Error("empty role: want error")
	}
}
