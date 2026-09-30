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

func TestAddPendingDefaultsProjectToEstate(t *testing.T) {
	f := &fakeFiler{}
	q := NewDecisionQueue()
	q.SetHome(f, nil, "")
	if err := q.AddPending(PendingDecision{Agent: "a", BeadID: "b1", BeadTitle: "t"}); err != nil {
		t.Fatal(err)
	}
	if f.asks[0].Project != "estate" || f.asks[0].ProjectRoot == "" || f.asks[0].ProjectRoot[0] != '/' {
		t.Errorf("bad defaults: %+v", f.asks[0])
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
