package scheduler

import (
	"testing"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

func TestHomeDecisionBeadsAreNeverSuggestedOrDispatched(t *testing.T) {
	for _, tier := range []mycroft.Tier{mycroft.T0, mycroft.T1, mycroft.T2, mycroft.T3} {
		o := newTestOrchestrator(t, tier)
		view := mycroft.FleetView{
			Agents: []mycroft.AgentView{{Name: "grey-area", Status: "active"}},
			Work: []mycroft.BeadView{
				{ID: "d1", Title: "rule", Type: "decision", Priority: 0, DepsResolved: true},
				{ID: "d2", Title: "rule", Type: "task", Priority: 0, DepsResolved: true, Labels: []string{"home:decision"}},
			},
		}
		o.OnCycle(view)
		var n int
		if err := o.db.QueryRow("SELECT COUNT(*) FROM dispatch_log").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("tier %v: %d dispatch rows for home decision beads", tier, n)
		}
	}
}
