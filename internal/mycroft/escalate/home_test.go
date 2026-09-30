package escalate

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
)

type fakeFiler struct{ asks []homeask.Ask }

func (f *fakeFiler) File(_ context.Context, a homeask.Ask) (string, error) {
	f.asks = append(f.asks, a)
	return "d1", nil
}

type fakeLister struct {
	rows []homeask.ListRow
	err  error
}

func (l *fakeLister) List(context.Context, string) ([]homeask.ListRow, error) {
	return l.rows, l.err
}

func TestAddPendingFilesOnceWithRulingOnlyOptions(t *testing.T) {
	f := &fakeFiler{}
	q := NewDecisionQueue()
	q.SetHome(f, nil, "/srv/estate")
	p := PendingDecision{Agent: "grey-area", BeadID: "Demarch-1", BeadTitle: "Fix test", Priority: 1, Reasoning: "match", Labels: []string{"project:demarch"}}
	if err := q.AddPending(p); err != nil {
		t.Fatal(err)
	}
	if err := q.AddPending(p); err != nil {
		t.Fatal(err)
	}
	if len(f.asks) != 1 {
		t.Fatalf("filed %d, want 1", len(f.asks))
	}
	a := f.asks[0]
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
	f := &fakeFiler{}
	q := NewDecisionQueue()
	q.SetHome(f, nil, "")
	err := q.AddPending(PendingDecision{Agent: "a", BeadID: "b1", BeadTitle: "t"})
	if err == nil || len(f.asks) != 0 {
		t.Fatalf("filed with a made-up root: err=%v asks=%+v", err, f.asks)
	}
}

func TestAddPendingUsesTheResolvedProjectNameAndRoot(t *testing.T) {
	f := &fakeFiler{}
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
	if f.asks[0].Project != "Demarch" || f.asks[0].ProjectRoot != "/real/projects/Demarch" {
		t.Errorf("project ask: %+v", f.asks[0])
	}
	if f.asks[1].Project != "estate" || f.asks[1].ProjectRoot != "/srv/uqbar" {
		t.Errorf("estate ask: %+v", f.asks[1])
	}
}

func TestAddPendingFilesNothingWhenTheRootCannotBeResolved(t *testing.T) {
	f := &fakeFiler{}
	q := NewDecisionQueue()
	q.SetHomeRoots(f, nil, func(string) (string, string, error) { return "", "", errors.New("no such project") })
	if err := q.AddPending(PendingDecision{Agent: "a", BeadID: "b1", BeadTitle: "t", Labels: []string{"project:x"}}); err == nil || len(f.asks) != 0 {
		t.Fatalf("err=%v asks=%+v", err, f.asks)
	}
}

func TestLenReadsHomeAndGoesStale(t *testing.T) {
	l := &fakeLister{rows: []homeask.ListRow{{ID: "1"}, {ID: "2"}}}
	q := NewDecisionQueue()
	q.SetHome(nil, l, "/x")
	if q.Len() != 2 || q.Stale() {
		t.Fatalf("len=%d stale=%v", q.Len(), q.Stale())
	}
	l.err = errors.New("down")
	q.expireCache()
	if q.Len() != 2 {
		t.Errorf("want last known 2, got %d", q.Len())
	}
	if !q.Stale() {
		t.Error("want stale when Home is down")
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
