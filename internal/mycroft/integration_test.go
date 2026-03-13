package mycroft_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
	"github.com/mistakeknot/autarch/internal/mycroft/escalate"
	"github.com/mistakeknot/autarch/internal/mycroft/patrol"
	"github.com/mistakeknot/autarch/internal/mycroft/scheduler"
	"github.com/mistakeknot/autarch/internal/mycroft/tier"
)

// mockDataSource provides canned responses for integration tests.
type mockDataSource struct {
	agents []mycroft.AgentView
	beads  []mycroft.BeadView
}

func (m *mockDataSource) FleetState() (mycroft.FleetView, error) {
	return mycroft.FleetView{
		Agents:    m.agents,
		Freshness: map[string]time.Time{"intermux": time.Now()},
	}, nil
}

func (m *mockDataSource) AgentHealth(name string) (string, error) {
	for _, a := range m.agents {
		if a.Name == name {
			return a.Status, nil
		}
	}
	return "unknown", nil
}

func (m *mockDataSource) BeadQueue() ([]mycroft.BeadView, error) {
	return m.beads, nil
}

func TestIntegration_T0FullCycle(t *testing.T) {
	dir := t.TempDir()
	db, err := mycroft.OpenDB(filepath.Join(dir, "decisions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	src := &mockDataSource{
		agents: []mycroft.AgentView{
			{Name: "grey-area", Status: "idle", Runtime: "claude-code"},
		},
		beads: []mycroft.BeadView{
			{ID: "Demarch-42", Title: "Fix test", Priority: 1, DepsResolved: true,
				Complexity: "simple", CreatedAt: time.Now().Add(-1 * time.Hour)},
			{ID: "Demarch-99", Title: "Add feature", Priority: 2, DepsResolved: true,
				Complexity: "medium", CreatedAt: time.Now()},
		},
	}

	cfg := mycroft.DefaultConfig()
	p := patrol.New(src, cfg, filepath.Join(dir, "heartbeat"),
		patrol.WithFleetInterval(time.Millisecond),
		patrol.WithWorkInterval(time.Millisecond),
	)

	// Run one patrol cycle.
	view := p.RunOnce(context.Background())

	// Selector ranks beads.
	ranked := scheduler.RankBeads(view.Work)
	if len(ranked) != 2 {
		t.Fatalf("ranked: got %d, want 2", len(ranked))
	}
	if ranked[0].ID != "Demarch-42" {
		t.Errorf("top ranked: got %q, want Demarch-42 (P1 older)", ranked[0].ID)
	}

	// At T0, log shadow suggestion (no action taken).
	disp := scheduler.NewDispatcher(db, nil, "test")
	for _, agent := range view.Agents {
		selected := scheduler.SelectForAgent(ranked, agent, nil, 3)
		for _, bead := range selected {
			if err := disp.LogShadow(agent.Name, bead, "priority match"); err != nil {
				t.Fatalf("LogShadow: %v", err)
			}
		}
	}

	// Verify shadow entries in dispatch_log.
	shadows, err := disp.ShadowDigest(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(shadows) != 2 {
		t.Errorf("shadow entries: got %d, want 2", len(shadows))
	}
}

func TestIntegration_T1ApproveReject(t *testing.T) {
	dir := t.TempDir()
	db, err := mycroft.OpenDB(filepath.Join(dir, "decisions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Set tier to T1.
	fsm := tier.New(db, "test")
	fsm.Promote(tier.Evidence{Reason: "integration test"})

	current, _ := fsm.Current()
	if current != mycroft.T1 {
		t.Fatalf("tier: got %v, want T1", current)
	}

	disp := scheduler.NewDispatcher(db, nil, "test")

	// Simulate T1 suggest → user approves one, rejects another.
	bead1 := mycroft.BeadView{ID: "Demarch-42", Title: "Fix test", Priority: 1}
	bead2 := mycroft.BeadView{ID: "Demarch-99", Title: "Risky change", Priority: 2}

	disp.LogSuggestion("grey-area", bead1, "P1 match")
	disp.LogSuggestion("mistake-not", bead2, "available agent")

	// User approves bead1, rejects bead2.
	disp.LogApproval("grey-area", "Demarch-42")
	disp.LogRejection("mistake-not", "Demarch-99", "too risky for now")

	// Verify history.
	history, err := disp.DispatchHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 4 {
		t.Fatalf("history: got %d, want 4", len(history))
	}

	// Check rejection reason persisted.
	var foundRejection bool
	for _, e := range history {
		if e.Bead == "Demarch-99" && e.Outcome == "rejected" {
			foundRejection = true
			if e.Reason != "too risky for now" {
				t.Errorf("rejection reason: got %q", e.Reason)
			}
		}
	}
	if !foundRejection {
		t.Error("rejection not found in history")
	}
}

func TestIntegration_FailureDetection(t *testing.T) {
	dirtyAgent := mycroft.AgentView{
		Name:   "grey-area",
		Status: "stuck",
		Health: mycroft.HealthReport{IsHealthy: false},
	}

	class := patrol.ClassifyFailure(dirtyAgent, true, false, "build")
	if class != mycroft.FailureDirty {
		t.Errorf("dirty stuck agent: got %q, want dirty", class)
	}

	cleanAgent := mycroft.AgentView{Name: "mistake-not", Status: "crashed"}
	class = patrol.ClassifyFailure(cleanAgent, false, false, "")
	if class != mycroft.FailureClean {
		t.Errorf("clean crashed agent: got %q, want clean", class)
	}
}

func TestIntegration_StalenessGating(t *testing.T) {
	src := &mockDataSource{}
	cfg := mycroft.DefaultConfig()
	p := patrol.New(src, cfg, "",
		patrol.WithFleetInterval(10*time.Millisecond),
		patrol.WithWorkInterval(10*time.Millisecond),
	)

	// Before cycle — stale.
	if !p.IsStale("intermux") {
		t.Error("intermux should be stale before first cycle")
	}

	p.RunOnce(context.Background())

	// After cycle — fresh.
	if p.IsStale("intermux") {
		t.Error("intermux should be fresh right after cycle")
	}

	// Wait for staleness.
	time.Sleep(25 * time.Millisecond)
	if !p.IsStale("intermux") {
		t.Error("intermux should be stale after 2x interval")
	}
}

func TestIntegration_DecisionQueueWithBadge(t *testing.T) {
	q := escalate.NewDecisionQueue()
	q.Add("grey-area", "Demarch-1", "Critical fix", 0, "P0 match")
	q.Add("mistake-not", "Demarch-2", "Feature", 3, "available")

	badge := escalate.Badge(q.Len(), q.HighestSeverity())
	if badge != "⚠ 2 pending" {
		t.Errorf("badge: got %q, want '⚠ 2 pending'", badge)
	}

	q.Remove(1) // Remove P0 decision.
	badge = escalate.Badge(q.Len(), q.HighestSeverity())
	if badge != "● 1 pending" {
		t.Errorf("badge after remove P0: got %q, want '● 1 pending'", badge)
	}
}

func TestIntegration_TierDemotionFromHistory(t *testing.T) {
	cfg := mycroft.DemotionTriggers{
		MinSampleSize:            5,
		ConsecutiveFailureLimit:  3,
		T2FailureRateThreshold:   0.15,
	}

	// 3 consecutive failures out of 5.
	history := []tier.DispatchRecord{
		{Outcome: "success"},
		{Outcome: "success"},
		{Outcome: "failure"},
		{Outcome: "failure"},
		{Outcome: "failure"},
	}

	demote, trigger, evidence := tier.ShouldDemote(mycroft.T2, history, cfg)
	if !demote {
		t.Error("should demote on 3 consecutive failures")
	}
	if trigger != "consecutive_failures" {
		t.Errorf("trigger: got %q", trigger)
	}
	if evidence.SampleSize != 5 {
		t.Errorf("sample size: got %d", evidence.SampleSize)
	}
}
