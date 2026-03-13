package fleet

import (
	"testing"
	"time"
)

func TestNewAggregatorSource(t *testing.T) {
	a := NewAggregatorSource("/nonexistent/registry.yaml")
	if a == nil {
		t.Fatal("expected non-nil AggregatorSource")
	}
	if a.cacheTTL != 10*time.Second {
		t.Errorf("default cacheTTL = %v, want 10s", a.cacheTTL)
	}
}

func TestNewAggregatorSourceWithOptions(t *testing.T) {
	a := NewAggregatorSource("/nonexistent/registry.yaml", WithCacheTTL(30*time.Second))
	if a.cacheTTL != 30*time.Second {
		t.Errorf("cacheTTL = %v, want 30s", a.cacheTTL)
	}
}

func TestAggregatorSourceFleetState(t *testing.T) {
	// With a missing registry, we should still get agents from tmux (if running)
	// or an empty result — but never an error from the registry.
	a := NewAggregatorSource("/nonexistent/registry.yaml")
	view, err := a.FleetState()
	if err != nil {
		t.Fatalf("FleetState: %v", err)
	}
	// Freshness should have tmux timestamp.
	if _, ok := view.Freshness["tmux"]; !ok {
		t.Error("expected tmux freshness timestamp")
	}
}

func TestAggregatorSourceCaching(t *testing.T) {
	a := NewAggregatorSource("/nonexistent/registry.yaml", WithCacheTTL(1*time.Hour))

	// First call populates cache.
	view1, err := a.FleetState()
	if err != nil {
		t.Fatalf("FleetState 1: %v", err)
	}

	// Second call should return cached result.
	view2, err := a.FleetState()
	if err != nil {
		t.Fatalf("FleetState 2: %v", err)
	}

	// Both should have the same freshness timestamp (from cache).
	if !view1.Freshness["tmux"].Equal(view2.Freshness["tmux"]) {
		t.Error("expected cached result to have same tmux timestamp")
	}
}

func TestAggregatorSourceAgentHealth(t *testing.T) {
	a := NewAggregatorSource("/nonexistent/registry.yaml")
	status, err := a.AgentHealth("nonexistent-agent")
	if err != nil {
		t.Fatalf("AgentHealth: %v", err)
	}
	if status != "unknown" {
		t.Errorf("status = %q, want unknown", status)
	}
}

func TestAggregatorSourceBeadQueue(t *testing.T) {
	a := NewAggregatorSource("/nonexistent/registry.yaml")
	// BeadQueue may fail if bd is not installed or not in a beads project.
	// We just verify it doesn't panic.
	beads, _ := a.BeadQueue()
	_ = beads
}

func TestEnrichAgent(t *testing.T) {
	a := NewAggregatorSource("/nonexistent/registry.yaml")
	a.registry = []AgentSpec{
		{
			Name:         "grey-area",
			Capabilities: []string{"debugging", "review"},
			Models:       Models{Preferred: "opus"},
			Runtime:      Runtime{Mode: "cli", Binary: "claude"},
		},
	}

	agent := &AgentView{Name: "grey-area"}
	a.enrichAgent(agent)

	if len(agent.Capabilities) != 2 {
		t.Errorf("capabilities = %d, want 2", len(agent.Capabilities))
	}
	if agent.CostProfile.Model != "opus" {
		t.Errorf("model = %q, want opus", agent.CostProfile.Model)
	}
	if agent.Runtime != "claude" {
		t.Errorf("runtime = %q, want claude", agent.Runtime)
	}
}

func TestEnrichAgentUnknown(t *testing.T) {
	a := NewAggregatorSource("/nonexistent/registry.yaml")
	a.registry = []AgentSpec{}

	agent := &AgentView{Name: "unknown-agent", Status: "active"}
	a.enrichAgent(agent)

	// Should not modify anything.
	if agent.Status != "active" {
		t.Errorf("status changed unexpectedly: %q", agent.Status)
	}
}

func TestRuntimeLabel(t *testing.T) {
	tests := []struct {
		runtime Runtime
		want    string
	}{
		{Runtime{Mode: "cli", Binary: "claude"}, "claude"},
		{Runtime{Mode: "cli"}, "cli"},
		{Runtime{Mode: "subagent"}, "subagent"},
		{Runtime{Mode: "daemon"}, "daemon"},
	}
	for _, tt := range tests {
		if got := runtimeLabel(tt.runtime); got != tt.want {
			t.Errorf("runtimeLabel(%v) = %q, want %q", tt.runtime, got, tt.want)
		}
	}
}

func TestExtractComplexity(t *testing.T) {
	tests := []struct {
		labels []string
		want   string
	}{
		{[]string{"bug", "complexity/simple"}, "simple"},
		{[]string{"feature"}, "unknown"},
		{nil, "unknown"},
	}
	for _, tt := range tests {
		if got := extractComplexity(tt.labels); got != tt.want {
			t.Errorf("extractComplexity(%v) = %q, want %q", tt.labels, got, tt.want)
		}
	}
}

func TestDataSourceInterface(t *testing.T) {
	// Compile-time check that AggregatorSource implements DataSource.
	var _ DataSource = (*AggregatorSource)(nil)
}
