package patrol

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// mockSource is a test DataSource that returns preconfigured data.
type mockSource struct {
	fleet  mycroft.FleetView
	beads  []mycroft.BeadView
	fleetErr error
	beadErr  error
	fleetCalls int
	beadCalls  int
}

func (m *mockSource) FleetState() (mycroft.FleetView, error) {
	m.fleetCalls++
	return m.fleet, m.fleetErr
}

func (m *mockSource) AgentHealth(name string) (string, error) {
	for _, a := range m.fleet.Agents {
		if a.Name == name {
			return a.Status, nil
		}
	}
	return "unknown", nil
}

func (m *mockSource) BeadQueue() ([]mycroft.BeadView, error) {
	m.beadCalls++
	return m.beads, m.beadErr
}

func TestPatrolRunOnce(t *testing.T) {
	src := &mockSource{
		fleet: mycroft.FleetView{
			Agents: []mycroft.AgentView{
				{Name: "grey-area", Status: "active"},
			},
		},
		beads: []mycroft.BeadView{
			{ID: "Demarch-42", Title: "Fix test", Priority: 1},
		},
	}

	dir := t.TempDir()
	cfg := mycroft.DefaultConfig()
	p := New(src, cfg, dir,
		WithFleetInterval(time.Millisecond),
		WithWorkInterval(time.Millisecond),
	)

	ctx := context.Background()
	view := p.RunOnce(ctx)

	if len(view.Agents) != 1 {
		t.Fatalf("agents: got %d, want 1", len(view.Agents))
	}
	if view.Agents[0].Name != "grey-area" {
		t.Errorf("agent name: got %q", view.Agents[0].Name)
	}
	if len(view.Work) != 1 {
		t.Fatalf("work: got %d, want 1", len(view.Work))
	}
	if view.Work[0].ID != "Demarch-42" {
		t.Errorf("bead id: got %q", view.Work[0].ID)
	}

	// Check freshness timestamps.
	if _, ok := view.Freshness["intermux"]; !ok {
		t.Error("missing intermux freshness")
	}
	if _, ok := view.Freshness["beads"]; !ok {
		t.Error("missing beads freshness")
	}
}

func TestPatrolHeartbeat(t *testing.T) {
	src := &mockSource{}
	dir := filepath.Join(t.TempDir(), "mycroft")
	cfg := mycroft.DefaultConfig()
	p := New(src, cfg, dir)

	p.RunOnce(context.Background())

	hbPath := filepath.Join(dir, "heartbeat")
	data, err := os.ReadFile(hbPath)
	if err != nil {
		t.Fatalf("read heartbeat: %v", err)
	}

	ts, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		t.Fatalf("parse heartbeat: %v", err)
	}

	age := time.Since(time.Unix(ts, 0))
	if age > 5*time.Second {
		t.Errorf("heartbeat too old: %v", age)
	}
}

func TestPatrolStaleness(t *testing.T) {
	src := &mockSource{}
	cfg := mycroft.DefaultConfig()
	p := New(src, cfg, "",
		WithFleetInterval(10*time.Millisecond),
		WithWorkInterval(10*time.Millisecond),
	)

	// Before any cycle, everything is stale.
	if !p.IsStale("intermux") {
		t.Error("intermux should be stale before first cycle")
	}
	if !p.IsStale("beads") {
		t.Error("beads should be stale before first cycle")
	}

	// Run a cycle — should be fresh.
	p.RunOnce(context.Background())
	if p.IsStale("intermux") {
		t.Error("intermux should be fresh after cycle")
	}
	if p.IsStale("beads") {
		t.Error("beads should be fresh after cycle")
	}

	// Wait for staleness (2x interval = 20ms).
	time.Sleep(25 * time.Millisecond)
	if !p.IsStale("intermux") {
		t.Error("intermux should be stale after 2x interval")
	}
}

func TestPatrolWorkInterval(t *testing.T) {
	src := &mockSource{
		beads: []mycroft.BeadView{
			{ID: "Demarch-1", Priority: 0},
		},
	}
	cfg := mycroft.DefaultConfig()
	p := New(src, cfg, "",
		WithFleetInterval(time.Millisecond),
		WithWorkInterval(time.Hour), // very long work interval
	)

	// First cycle queries both.
	p.RunOnce(context.Background())
	if src.beadCalls != 1 {
		t.Fatalf("first cycle bead calls: got %d, want 1", src.beadCalls)
	}

	// Second cycle should NOT re-query beads (work interval not reached).
	p.RunOnce(context.Background())
	if src.beadCalls != 1 {
		t.Errorf("second cycle bead calls: got %d, want 1 (carried forward)", src.beadCalls)
	}

	// Work should be carried forward.
	view := p.LastView()
	if len(view.Work) != 1 {
		t.Errorf("carried work: got %d, want 1", len(view.Work))
	}
}

func TestPatrolOnCycleCallback(t *testing.T) {
	src := &mockSource{}
	cfg := mycroft.DefaultConfig()

	var called int
	p := New(src, cfg, "",
		WithOnCycle(func(v mycroft.FleetView) { called++ }),
	)

	p.RunOnce(context.Background())
	if called != 1 {
		t.Errorf("onCycle called %d times, want 1", called)
	}
}

func TestExtractComplexity(t *testing.T) {
	tests := []struct {
		labels []string
		want   string
	}{
		{[]string{"complexity/simple"}, "simple"},
		{[]string{"complexity/medium"}, "medium"},
		{[]string{"complexity/complex"}, "complex"},
		{[]string{"bug", "priority/1"}, "unknown"},
		{nil, "unknown"},
		{[]string{"complexity/simple", "other"}, "simple"},
	}
	for _, tt := range tests {
		got := extractComplexity(tt.labels)
		if got != tt.want {
			t.Errorf("extractComplexity(%v) = %q, want %q", tt.labels, got, tt.want)
		}
	}
}

func TestPatrolFleetError(t *testing.T) {
	src := &mockSource{
		fleetErr: os.ErrNotExist,
		beads:    []mycroft.BeadView{{ID: "Demarch-1"}},
	}
	cfg := mycroft.DefaultConfig()
	p := New(src, cfg, "",
		WithFleetInterval(time.Millisecond),
		WithWorkInterval(time.Millisecond),
	)

	view := p.RunOnce(context.Background())

	// Fleet error should result in no agents but beads should still be queried.
	if len(view.Agents) != 0 {
		t.Errorf("agents should be empty on error, got %d", len(view.Agents))
	}
	if len(view.Work) != 1 {
		t.Errorf("work should still be populated, got %d", len(view.Work))
	}
}
