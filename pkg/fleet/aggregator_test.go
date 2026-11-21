package fleet

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestParseSessionActivity(t *testing.T) {
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	for _, age := range []time.Duration{0, time.Second, 5*time.Minute - time.Second, 5 * time.Minute, 24 * time.Hour} {
		ts := now.Add(-age)
		for _, raw := range []string{strconv.FormatInt(ts.Unix(), 10), ts.Format("2006-01-02T15:04:05")} {
			health, status := parseSessionActivity(raw, now)
			wantStatus := "idle"
			if age < 5*time.Minute {
				wantStatus = "active"
			}
			if !health.LastSeen.Equal(ts) || health.IsHealthy != (wantStatus == "active") || status != wantStatus || health.Details != "" {
				t.Errorf("parseSessionActivity(%q) = %+v, %s; want %s at %s", raw, health, status, wantStatus, ts)
			}
		}
	}
	for _, raw := range []string{"", "garbage", "0", "-1", "9999999999999999999999", "9223372036854775807", "1.5", "2026-99-99T00:00:00", strconv.FormatInt(now.Unix()+1, 10), "2027-01-01T00:00:00"} {
		health, status := parseSessionActivity(raw, now)
		if status != "unknown" || health.IsHealthy || !health.LastSeen.IsZero() || health.Details == "" {
			t.Errorf("invalid %q = %+v, %s", raw, health, status)
		}
	}
}

func TestDetectAgentsFromTmuxEpoch(t *testing.T) {
	dir := t.TempDir()
	fixture := "#!/bin/sh\nprintf '%s\\n' 'test-agent-session:1700000000' 'bad-agent-session:garbage'\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(fixture), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	agents, err := detectAgentsFromTmux()
	if err != nil || len(agents) != 2 {
		t.Fatalf("agents = %v, err = %v", agents, err)
	}
	if agents[0].Status != "idle" || agents[0].Health.IsHealthy || agents[0].Health.LastSeen.Unix() != 1700000000 {
		t.Errorf("epoch activity was lost: %+v", agents[0])
	}
	if agents[1].Status != "unknown" || agents[1].Health.IsHealthy || !agents[1].Health.LastSeen.IsZero() || agents[1].Health.Details == "" {
		t.Errorf("invalid activity must remain unknown: %+v", agents[1])
	}
}

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
