package aggregator

import (
	"context"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/discovery"
	"github.com/mistakeknot/autarch/internal/icdata"
)

func TestEnrichWithKernelStateNilForNoKernelProjects(t *testing.T) {
	agg := &Aggregator{}
	projects := []discovery.Project{
		{Path: "/tmp/no-kernel", HasGurgeh: true},
	}
	ks := agg.enrichWithKernelState(context.Background(), projects)
	if ks != nil {
		t.Error("expected nil KernelState when no projects have Intercore")
	}
}

func TestPileupGuard(t *testing.T) {
	agg := &Aggregator{}
	// Simulate an in-progress refresh
	agg.refreshing.Store(true)

	err := agg.Refresh(context.Background())
	if err != nil {
		t.Errorf("expected nil error from pileup guard, got %v", err)
	}
	// The refresh should have been skipped (no panic, no scanner nil deref)
}

func TestComputeKernelMetrics(t *testing.T) {
	agg := &Aggregator{}
	ks := &KernelState{
		Runs: map[string][]icdata.Run{
			"/p1": {
				{ID: "r1", Status: "active"},
				{ID: "r2", Status: "done"},
			},
		},
		Dispatches: map[string][]icdata.Dispatch{
			"/p1": {
				{ID: "d1", Status: "running", InTokens: 1000, OutTokens: 500},
				{ID: "d2", Status: "blocked", InTokens: 200, OutTokens: 100},
				{ID: "d3", Status: "completed", InTokens: 3000, OutTokens: 1500},
			},
		},
		Events: map[string][]icdata.Event{},
		Metrics: KernelMetrics{
			KernelErrors: make(map[string]string),
		},
	}

	agg.computeKernelMetrics(ks)

	if ks.Metrics.ActiveRuns != 1 {
		t.Errorf("ActiveRuns = %d, want 1", ks.Metrics.ActiveRuns)
	}
	if ks.Metrics.ActiveDispatches != 1 {
		t.Errorf("ActiveDispatches = %d, want 1", ks.Metrics.ActiveDispatches)
	}
	if ks.Metrics.BlockedAgents != 1 {
		t.Errorf("BlockedAgents = %d, want 1", ks.Metrics.BlockedAgents)
	}
	if ks.Metrics.TotalTokensIn != 4200 {
		t.Errorf("TotalTokensIn = %d, want 4200", ks.Metrics.TotalTokensIn)
	}
	if ks.Metrics.TotalTokensOut != 2100 {
		t.Errorf("TotalTokensOut = %d, want 2100", ks.Metrics.TotalTokensOut)
	}
}

func TestMergeActivitiesDedup(t *testing.T) {
	now := time.Now()
	existing := []Activity{
		{Time: now, Summary: "existing", SyntheticID: "kernel:/p1:1", Source: "kernel"},
	}
	incoming := []Activity{
		{Time: now, Summary: "dupe", SyntheticID: "kernel:/p1:1", Source: "kernel"},    // should be deduped
		{Time: now.Add(-time.Second), Summary: "new", SyntheticID: "kernel:/p1:2", Source: "kernel"}, // should be added
	}

	merged := mergeActivities(existing, incoming, 100)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged activities, got %d", len(merged))
	}
	// Verify sorted by time descending
	if merged[0].Summary != "existing" {
		t.Errorf("first activity should be 'existing', got %q", merged[0].Summary)
	}
	if merged[1].Summary != "new" {
		t.Errorf("second activity should be 'new', got %q", merged[1].Summary)
	}
}

func TestMergeActivitiesLimit(t *testing.T) {
	var existing []Activity
	for i := 0; i < 150; i++ {
		existing = append(existing, Activity{
			Time:    time.Now().Add(-time.Duration(i) * time.Second),
			Summary: "old",
		})
	}

	merged := mergeActivities(existing, nil, 100)
	if len(merged) != 100 {
		t.Errorf("expected 100 activities after limit, got %d", len(merged))
	}
}

func TestKernelEventsToActivities(t *testing.T) {
	ks := &KernelState{
		Events: map[string][]icdata.Event{
			"/projects/myapp": {
				{ID: 1, RunID: "r1", Source: "phase", Type: "advance", FromState: "brainstorm", ToState: "planned", Timestamp: "2026-02-20T10:00:00Z"},
			},
		},
	}

	acts := kernelEventsToActivities(ks)
	if len(acts) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(acts))
	}
	if acts[0].Source != "kernel" {
		t.Errorf("Source = %q, want kernel", acts[0].Source)
	}
	if acts[0].SyntheticID != "kernel:/projects/myapp:1" {
		t.Errorf("SyntheticID = %q", acts[0].SyntheticID)
	}
}

func TestKernelEventsToActivitiesNil(t *testing.T) {
	acts := kernelEventsToActivities(nil)
	if acts != nil {
		t.Errorf("expected nil for nil KernelState, got %v", acts)
	}
}
