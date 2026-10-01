package scheduler

import (
	"context"
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
	"github.com/mistakeknot/autarch/internal/mycroft"
	"github.com/mistakeknot/autarch/internal/mycroft/escalate"
)

type recFiler struct{ asks []homeask.Ask }

func (f *recFiler) FileForPull(_ context.Context, a homeask.Ask) (string, error) {
	f.asks = append(f.asks, a)
	return "d", nil
}

func TestT1SuggestFilesOnceAcrossCycles(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T1)
	f := &recFiler{}
	q := escalate.NewDecisionQueue()
	q.SetHome(f, nil, "/srv/estate")
	o.SetQueue(q)
	view := mycroft.FleetView{
		Agents: []mycroft.AgentView{{Name: "grey-area", Status: "active"}},
		Work:   []mycroft.BeadView{{ID: "b1", Title: "Fix", Type: "bug", Priority: 2, DepsResolved: true}},
	}
	o.OnCycle(view)
	o.OnCycle(view)
	if len(f.asks) != 1 {
		t.Fatalf("filed %d, want 1", len(f.asks))
	}
}

func TestT2OutsideAllowlistFilesAsk(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T2)
	f := &recFiler{}
	q := escalate.NewDecisionQueue()
	q.SetHome(f, nil, "/srv/estate")
	o.SetQueue(q)
	o.OnCycle(mycroft.FleetView{
		Agents: []mycroft.AgentView{{Name: "grey-area", Status: "active"}},
		Work:   []mycroft.BeadView{{ID: "b9", Title: "Big", Type: "epic", Priority: 0, DepsResolved: true}},
	})
	if len(f.asks) != 1 {
		t.Fatalf("filed %d, want 1", len(f.asks))
	}
}
