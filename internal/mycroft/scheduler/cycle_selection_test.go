package scheduler

import (
	"fmt"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

func TestCycleSelection(t *testing.T) {
	for _, tier := range []mycroft.Tier{mycroft.T0, mycroft.T1, mycroft.T2, mycroft.T3} {
		for _, scenario := range []struct {
			name    string
			beads   int
			status  string
			healthy bool
			current string
			claimed bool
			outside bool
			want    int
		}{
			{name: "one bead", beads: 1, status: "idle", want: 1},
			{name: "two beads", beads: 2, status: "idle", want: 2},
			{name: "outside allowlist", beads: 2, status: "idle", outside: true, want: 2},
			{name: "claimed by other", beads: 2, status: "idle", claimed: true, want: 1},
			{name: "unhealthy active is busy", beads: 2, status: "active"},
			{name: "healthy active is available", beads: 2, status: "active", healthy: true, want: 2},
			{name: "healthy active with current bead is busy", beads: 2, status: "active", healthy: true, current: "existing"},
			{name: "idle with current bead is busy", beads: 2, status: "idle", current: "existing"},
		} {
			t.Run(fmt.Sprintf("%v/%s", tier, scenario.name), func(t *testing.T) {
				o := newTestOrchestrator(t, tier)
				view := mycroft.FleetView{Agents: []mycroft.AgentView{
					{Name: "agent-a", Status: scenario.status, CurrentBead: scenario.current, Health: mycroft.HealthReport{IsHealthy: scenario.healthy}},
					{Name: "agent-b", Status: scenario.status, CurrentBead: scenario.current, Health: mycroft.HealthReport{IsHealthy: scenario.healthy}},
				}}
				for i := 0; i < scenario.beads; i++ {
					view.Work = append(view.Work, mycroft.BeadView{ID: fmt.Sprintf("bead-%d", i), Type: "task", Priority: i + 3, Complexity: "simple", DepsResolved: true})
				}
				if scenario.outside {
					for i := range view.Work {
						view.Work[i].Type = "feature"
					}
				}
				if scenario.claimed {
					view.Work[0].ClaimedBy = "other"
				}
				// Each call is a fresh cycle: suggestions must not reserve work across cycles.
				for cycle := 0; cycle < 2; cycle++ {
					o.OnCycle(view)
					rows, err := o.db.Query("SELECT agent, bead, action FROM dispatch_log")
					if err != nil {
						t.Fatal(err)
					}
					seen := map[string]string{}
					count := 0
					for rows.Next() {
						var agent, bead, action string
						if err := rows.Scan(&agent, &bead, &action); err != nil {
							t.Fatal(err)
						}
						if owner, ok := seen[bead]; ok {
							t.Errorf("bead %s offered to both %s and %s", bead, owner, agent)
						}
						seen[bead] = agent
						if scenario.claimed && bead == "bead-0" {
							t.Error("selected bead claimed by other")
						}
						wantAction := "auto_dispatch"
						if tier == mycroft.T0 {
							wantAction = "shadow_suggest"
						}
						if tier == mycroft.T1 || (tier == mycroft.T2 && scenario.outside) {
							wantAction = "suggest"
						}
						if action != wantAction {
							t.Errorf("action = %s, want %s", action, wantAction)
						}
						count++
					}
					if err := rows.Err(); err != nil {
						t.Fatal(err)
					}
					rows.Close()
					if count != scenario.want {
						t.Errorf("got %d assignments, want %d", count, scenario.want)
					}
					if _, err := o.db.Exec("DELETE FROM dispatch_log"); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

// These fields match AggregatorSource's epoch-activity result. Parsing and
// health semantics are intentionally unchanged in this scheduler regression.
func TestAvailableAgentsAggregatorActive(t *testing.T) {
	now := time.Now()
	health := mycroft.HealthReport{LastSeen: now, IsHealthy: true}
	agents := []mycroft.AgentView{
		{Name: "agent-active-a", Runtime: "claude-code", Status: "active", Health: health},
		{Name: "agent-active-b", Runtime: "claude-code", Status: "active", Health: health},
		{Name: "agent-claimed-c", Status: "active", Health: health, CurrentBead: "existing"},
		{Name: "agent-unhealthy-d", Status: "active"},
		{Name: "agent-claimed-idle", Status: "idle", CurrentBead: "existing"},
		{Name: "agent-unknown", Status: "unknown", Health: health},
		{Name: "agent-stuck", Status: "stuck", Health: health},
		{Name: "agent-offline", Status: "offline", Health: health},
	}
	got := availableAgents(agents)
	if len(got) != 2 {
		t.Fatalf("got %d available aggregator agents, want 2", len(got))
	}
	if got[0].Name != agents[0].Name || got[1].Name != agents[1].Name {
		t.Fatalf("unexpected available agents: %+v", got)
	}
}
