package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

func newTestOrchestrator(t *testing.T, tier mycroft.Tier) *Orchestrator {
	t.Helper()
	db, err := mycroft.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := mycroft.DefaultConfig()
	cfg.Tier = tier
	cfg.T2DispatchAllowlist = []mycroft.AllowlistEntry{
		{Type: "task", MaxPriority: 3, MaxComplexity: "medium"},
		{Type: "docs", MaxPriority: 2, MaxComplexity: "any"},
	}

	return NewOrchestrator(db, nil, cfg, "test")
}

func TestOrchestratorT0ShadowSuggest(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T0)

	view := mycroft.FleetView{
		Agents: []mycroft.AgentView{
			{Name: "grey-area", Status: "active"},
		},
		Work: []mycroft.BeadView{
			{ID: "b1", Title: "Fix bug", Type: "bug", Priority: 2, DepsResolved: true},
		},
	}

	o.OnCycle(view)

	// Should log a shadow_suggest entry.
	d := NewDispatcher(o.db, nil, "test")
	entries, err := d.ShadowDigest(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 shadow entry, got %d", len(entries))
	}
	if entries[0].Agent != "grey-area" {
		t.Errorf("agent = %q, want grey-area", entries[0].Agent)
	}
}

func TestOrchestratorT1Suggest(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T1)

	view := mycroft.FleetView{
		Agents: []mycroft.AgentView{
			{Name: "mistake-not", Status: "active"},
		},
		Work: []mycroft.BeadView{
			{ID: "b2", Title: "Add feature", Type: "feature", Priority: 1, DepsResolved: true},
		},
	}

	o.OnCycle(view)

	d := NewDispatcher(o.db, nil, "test")
	entries, err := d.DispatchHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "suggest" {
		t.Errorf("action = %q, want suggest", entries[0].Action)
	}
}

func TestOrchestratorT2AllowlistGate(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T2)

	view := mycroft.FleetView{
		Agents: []mycroft.AgentView{
			{Name: "grey-area", Status: "active"},
		},
		Work: []mycroft.BeadView{
			// P1 feature — should be escalated (not in allowlist).
			{ID: "b3", Title: "Big feature", Type: "feature", Priority: 1, Complexity: "complex", DepsResolved: true},
		},
	}

	o.OnCycle(view)

	d := NewDispatcher(o.db, nil, "test")
	entries, err := d.DispatchHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	// Should be suggest (escalated), not auto_dispatch.
	if entries[0].Action != "suggest" {
		t.Errorf("action = %q, want suggest (escalated)", entries[0].Action)
	}
}

func TestOrchestratorT2AllowlistPass(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T2)

	view := mycroft.FleetView{
		Agents: []mycroft.AgentView{
			{Name: "grey-area", Status: "active"},
		},
		Work: []mycroft.BeadView{
			// P3 task, simple — should pass allowlist.
			{ID: "b4", Title: "Small task", Type: "task", Priority: 3, Complexity: "simple", DepsResolved: true},
		},
	}

	o.OnCycle(view)

	d := NewDispatcher(o.db, nil, "test")
	entries, err := d.DispatchHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "auto_dispatch" {
		t.Errorf("action = %q, want auto_dispatch", entries[0].Action)
	}
}

func TestOrchestratorT3DispatchAll(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T3)

	view := mycroft.FleetView{
		Agents: []mycroft.AgentView{
			{Name: "grey-area", Status: "active"},
		},
		Work: []mycroft.BeadView{
			// P1 feature complex — T3 dispatches everything.
			{ID: "b5", Title: "Complex feature", Type: "feature", Priority: 1, Complexity: "complex", DepsResolved: true},
		},
	}

	o.OnCycle(view)

	d := NewDispatcher(o.db, nil, "test")
	entries, err := d.DispatchHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "auto_dispatch" {
		t.Errorf("action = %q, want auto_dispatch", entries[0].Action)
	}
}

func TestOrchestratorPauseResume(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T3)

	o.Pause()
	if !o.IsPaused() {
		t.Error("expected paused")
	}

	view := mycroft.FleetView{
		Agents: []mycroft.AgentView{{Name: "a1", Status: "active"}},
		Work:   []mycroft.BeadView{{ID: "b1", DepsResolved: true}},
	}

	o.OnCycle(view)

	// Should not dispatch while paused.
	d := NewDispatcher(o.db, nil, "test")
	entries, _ := d.DispatchHistory(10)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries while paused, got %d", len(entries))
	}

	o.Resume()
	o.OnCycle(view)

	entries, _ = d.DispatchHistory(10)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after resume, got %d", len(entries))
	}
}

func TestOrchestratorNoWork(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T3)

	view := mycroft.FleetView{
		Agents: []mycroft.AgentView{{Name: "a1", Status: "active"}},
		Work:   nil,
	}

	// Should not panic or log anything.
	o.OnCycle(view)
}

func TestOrchestratorNoAgents(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T3)

	view := mycroft.FleetView{
		Agents: nil,
		Work:   []mycroft.BeadView{{ID: "b1", DepsResolved: true}},
	}

	// Should not panic or dispatch.
	o.OnCycle(view)
}

func TestOrchestratorOfflineAgentsSkipped(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T3)

	view := mycroft.FleetView{
		Agents: []mycroft.AgentView{
			{Name: "offline-agent", Status: "offline"},
		},
		Work: []mycroft.BeadView{{ID: "b1", DepsResolved: true}},
	}

	o.OnCycle(view)

	d := NewDispatcher(o.db, nil, "test")
	entries, _ := d.DispatchHistory(10)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for offline agent, got %d", len(entries))
	}
}

func TestAvailableAgents(t *testing.T) {
	agents := []mycroft.AgentView{
		{Name: "a1", Status: "active"},
		{Name: "a2", Status: "idle"},
		{Name: "a3", Status: "offline"},
		{Name: "a4", Status: "unknown"},
	}

	available := availableAgents(agents)
	if len(available) != 2 {
		t.Errorf("expected 2 available, got %d", len(available))
	}
}
