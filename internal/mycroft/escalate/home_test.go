package escalate

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
)

func TestAddPendingFilesOnceWithRulingOnlyOptions(t *testing.T) {
	f := &MemoryFiler{}
	q := NewDecisionQueue()
	q.SetHome(f, nil, "/srv/estate")
	p := PendingDecision{Agent: "grey-area", BeadID: "Demarch-1", BeadTitle: "Fix test", Priority: 1, Reasoning: "match", Labels: []string{"project:demarch"}}
	if err := q.AddPending(p); err != nil {
		t.Fatal(err)
	}
	if err := q.AddPending(p); err != nil {
		t.Fatal(err)
	}
	if len(f.Asks) != 1 {
		t.Fatalf("filed %d, want 1", len(f.Asks))
	}
	a := f.Asks[0]
	if a.RequestID != "mycroft:demarch:Demarch-1:grey-area" || a.Project != "demarch" || a.Asker != "mycroft" || a.Thread != "" {
		t.Errorf("bad ask: %+v", a)
	}
	if a.ProjectRoot != "/srv/estate" || a.Subject != "mycroft/Demarch-1/grey-area" {
		t.Errorf("bad root/subject: %+v", a)
	}
	if len(a.Options) != 2 {
		t.Fatalf("options: %+v", a.Options)
	}
	for _, o := range a.Options {
		if o.Kind != "ruling-only" {
			t.Errorf("option %+v not ruling-only", o)
		}
	}
	if _, err := homeask.Parse(mustJSON(t, a)); err != nil {
		t.Errorf("ask does not validate: %v", err)
	}
}

func TestAddPendingWithoutARootFilesNothingInsteadOfGuessing(t *testing.T) {
	f := &MemoryFiler{}
	q := NewDecisionQueue()
	q.SetHome(f, nil, "")
	err := q.AddPending(PendingDecision{Agent: "a", BeadID: "b1", BeadTitle: "t"})
	if err == nil || len(f.Asks) != 0 {
		t.Fatalf("filed with a made-up root: err=%v asks=%+v", err, f.Asks)
	}
}

func TestAddPendingUsesTheResolvedProjectNameAndRoot(t *testing.T) {
	f := &MemoryFiler{}
	q := NewDecisionQueue()
	var asked []string
	q.SetHomeRoots(f, nil, func(project string) (string, string, error) {
		asked = append(asked, project)
		if project == "estate" {
			return "estate", "/srv/uqbar", nil
		}
		return "Demarch", "/real/projects/Demarch", nil
	})
	if err := q.AddPending(PendingDecision{Agent: "a", BeadID: "b1", BeadTitle: "t", Labels: []string{"project:demarch"}}); err != nil {
		t.Fatal(err)
	}
	if err := q.AddPending(PendingDecision{Agent: "a", BeadID: "b2", BeadTitle: "t"}); err != nil {
		t.Fatal(err)
	}
	if f.Asks[0].Project != "Demarch" || f.Asks[0].ProjectRoot != "/real/projects/Demarch" {
		t.Errorf("project ask: %+v", f.Asks[0])
	}
	if f.Asks[1].Project != "estate" || f.Asks[1].ProjectRoot != "/srv/uqbar" {
		t.Errorf("estate ask: %+v", f.Asks[1])
	}
}

func TestAddPendingFilesNothingWhenTheRootCannotBeResolved(t *testing.T) {
	f := &MemoryFiler{}
	q := NewDecisionQueue()
	q.SetHomeRoots(f, nil, func(string) (string, string, error) { return "", "", errors.New("no such project") })
	if err := q.AddPending(PendingDecision{Agent: "a", BeadID: "b1", BeadTitle: "t", Labels: []string{"project:x"}}); err == nil || len(f.Asks) != 0 {
		t.Fatalf("err=%v asks=%+v", err, f.Asks)
	}
}

func TestLenReadsHomeAndGoesStale(t *testing.T) {
	l := &MemoryFiler{Rows: []homeask.ListRow{{ID: "1"}, {ID: "2"}}}
	q := NewDecisionQueue()
	q.SetHome(nil, l, "/x")
	if q.Len() != 2 || q.Stale() {
		t.Fatalf("len=%d stale=%v", q.Len(), q.Stale())
	}
	l.ListErr = errors.New("down")
	q.expireCache()
	if q.Len() != 2 {
		t.Errorf("want last known 2, got %d", q.Len())
	}
	if !q.Stale() {
		t.Error("want stale when Home is down")
	}
}

func TestQueueHasNoPrivateListAndFilesOnlyThroughHome(t *testing.T) {
	q := NewDecisionQueue()
	if q.Len() != 0 || len(q.All()) != 0 || q.HasHome() {
		t.Fatal("an unwired queue must be empty")
	}
	if err := q.AddPending(PendingDecision{Agent: "a", BeadID: "b1", BeadTitle: "t"}); err == nil {
		t.Fatal("an unwired queue must refuse, not queue locally")
	}
	if q.Len() != 0 {
		t.Fatal("a refused suggestion appeared in the queue")
	}
}

func TestQueueReadsRowsAndCardOutcomesFromHome(t *testing.T) {
	m := &MemoryFiler{}
	q := NewDecisionQueue()
	q.SetHome(m, m, "/srv/estate")
	if err := q.AddPending(PendingDecision{Agent: "a", BeadID: "b1", BeadTitle: "t", Priority: 0}); err != nil {
		t.Fatal(err)
	}
	if err := q.AddPending(PendingDecision{Agent: "a", BeadID: "b1", BeadTitle: "t", Priority: 0}); err != nil {
		t.Fatal(err)
	}
	if len(m.Asks) != 1 || m.Asks[0].Asker != "mycroft" || m.Asks[0].Thread != "" {
		t.Fatalf("asks: %+v", m.Asks)
	}
	q.expireCache()
	if q.Len() != 1 {
		t.Fatalf("len=%d", q.Len())
	}
	if r, ok := q.Get("T1"); !ok || r.Project != "estate" {
		t.Fatalf("get: %+v %v", r, ok)
	}
	if _, ok := q.Get("nope"); ok {
		t.Fatal("unknown id found")
	}
	m.Rows[0].Priority = 0
	q.expireCache()
	if q.HighestSeverity() != SeverityHigh {
		t.Fatal("severity must come from the Home row")
	}
	m.CardView = homeask.CardView{DecisionState: "ruled"}
	v, err := q.Outcome(context.Background(), "T1")
	if err != nil || v.DecisionState != "ruled" || len(m.Cards) != 1 || m.Cards[0] != "T1" {
		t.Fatalf("outcome %+v %v %v", v, err, m.Cards)
	}
	q.Remove("T1") // documented no-op
	q.expireCache()
	if q.Len() != 1 {
		t.Fatal("Remove must not change what Home lists")
	}
}

func mustJSON(t *testing.T, a homeask.Ask) []byte {
	t.Helper()
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
