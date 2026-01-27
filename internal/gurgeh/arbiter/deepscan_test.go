package arbiter_test

import (
	"context"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
)

func TestDeepScan_FullLifecycle(t *testing.T) {
	provider := &testResearchProvider{
		specID: "spec-deep",
		findings: []arbiter.ResearchFinding{
			{Title: "Deep Finding A", Summary: "Details", SourceType: "arxiv", Relevance: 0.85},
			{Title: "Deep Finding B", Summary: "More", SourceType: "github", Relevance: 0.7},
		},
	}
	o := arbiter.NewOrchestratorWithResearch("/tmp/test", provider)
	ctx := context.Background()

	state, err := o.Start(ctx, "deep scan test")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Start deep scan
	if err := o.StartDeepScan(ctx, state); err != nil {
		t.Fatalf("StartDeepScan: %v", err)
	}
	if state.DeepScan.Status != arbiter.DeepScanRunning {
		t.Errorf("expected DeepScanRunning, got %d", state.DeepScan.Status)
	}
	if state.DeepScan.ScanID == "" {
		t.Error("expected non-empty ScanID")
	}

	// Check scan (mock returns done immediately)
	done, err := o.CheckDeepScan(ctx, state)
	if err != nil {
		t.Fatalf("CheckDeepScan: %v", err)
	}
	if !done {
		t.Error("expected done=true")
	}
	if state.DeepScan.Status != arbiter.DeepScanComplete {
		t.Errorf("expected DeepScanComplete, got %d", state.DeepScan.Status)
	}

	// Import results
	if err := o.ImportDeepScanResults(ctx, state); err != nil {
		t.Fatalf("ImportDeepScanResults: %v", err)
	}
	if len(state.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(state.Findings))
	}
}

func TestDeepScan_NoProvider(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test")
	state, _ := o.Start(context.Background(), "no provider")

	if err := o.StartDeepScan(context.Background(), state); err == nil {
		t.Error("expected error without research provider")
	}
}

func TestDeepScan_NoSpecID(t *testing.T) {
	provider := &testResearchProvider{specID: ""}
	o := arbiter.NewOrchestratorWithResearch("/tmp/test", provider)

	state, _ := o.Start(context.Background(), "no spec")

	if err := o.StartDeepScan(context.Background(), state); err == nil {
		t.Error("expected error without spec ID")
	}
}

func TestDeepScan_Deduplicates(t *testing.T) {
	provider := &testResearchProvider{
		specID: "spec-dedup",
		findings: []arbiter.ResearchFinding{
			{Title: "Existing", Summary: "Already there", Relevance: 0.5},
			{Title: "New One", Summary: "Fresh", Relevance: 0.8},
		},
	}
	o := arbiter.NewOrchestratorWithResearch("/tmp/test", provider)
	ctx := context.Background()

	state, _ := o.Start(ctx, "dedup test")
	// Pre-populate with one finding
	state.Findings = []arbiter.ResearchFinding{
		{Title: "Existing", Summary: "Already there", Relevance: 0.5},
	}

	_ = o.StartDeepScan(ctx, state)
	_, _ = o.CheckDeepScan(ctx, state)
	_ = o.ImportDeepScanResults(ctx, state)

	if len(state.Findings) != 2 {
		t.Fatalf("expected 2 findings after dedup, got %d", len(state.Findings))
	}
}
