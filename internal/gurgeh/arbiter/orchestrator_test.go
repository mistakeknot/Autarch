package arbiter_test

import (
	"context"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
)

func TestOrchestratorStartsSprint(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	state, err := o.Start(ctx, "Users can't find relevant research papers quickly")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if state.Phase != arbiter.PhaseProblem {
		t.Errorf("expected PhaseProblem, got %v", state.Phase)
	}

	section := state.Sections[arbiter.PhaseProblem]
	if section == nil {
		t.Fatal("Problem section is nil")
	}
	if section.Content == "" {
		t.Error("Problem section has no content")
	}
	if section.Status != arbiter.DraftProposed {
		t.Errorf("expected DraftProposed, got %v", section.Status)
	}
}

func TestOrchestratorAdvancesPhase(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	state, err := o.Start(ctx, "Users need better search")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Accept Problem draft
	state = o.AcceptDraft(state)

	// Advance to Users
	state, err = o.Advance(ctx, state)
	if err != nil {
		t.Fatalf("Advance failed: %v", err)
	}

	if state.Phase != arbiter.PhaseUsers {
		t.Errorf("expected PhaseUsers, got %v", state.Phase)
	}
}

func TestOrchestratorTriggersQuickScan(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	state, err := o.Start(ctx, "Users need better search")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Advance through Problem -> Users -> FeaturesGoals
	state = o.AcceptDraft(state)
	state, err = o.Advance(ctx, state)
	if err != nil {
		t.Fatalf("Advance to Users failed: %v", err)
	}

	state = o.AcceptDraft(state)
	state, err = o.Advance(ctx, state)
	if err != nil {
		t.Fatalf("Advance to FeaturesGoals failed: %v", err)
	}

	if state.Phase != arbiter.PhaseFeaturesGoals {
		t.Errorf("expected PhaseFeaturesGoals, got %v", state.Phase)
	}
}

func TestOrchestratorBlocksOnConflicts(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	state, err := o.Start(ctx, "solo developers struggle with code review")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Accept problem about solo developers
	state = o.AcceptDraft(state)

	// Advance to Users
	state, err = o.Advance(ctx, state)
	if err != nil {
		t.Fatalf("Advance to Users failed: %v", err)
	}
	state = o.AcceptDraft(state)

	// Advance to FeaturesGoals
	state, err = o.Advance(ctx, state)
	if err != nil {
		t.Fatalf("Advance to FeaturesGoals failed: %v", err)
	}

	// Manually set conflicting feature content about enterprise
	state.Sections[arbiter.PhaseFeaturesGoals].Content = "enterprise admin dashboard for 100+ users"
	state.Sections[arbiter.PhaseFeaturesGoals].Status = arbiter.DraftAccepted

	// Try to advance - should be blocked
	_, err = o.Advance(ctx, state)
	if err == nil {
		t.Fatal("expected blocker error, got nil")
	}
	if !arbiter.IsBlockerError(err) {
		t.Errorf("expected blocker error, got: %v", err)
	}
}

// testResearchProvider records calls for testing.
type testResearchProvider struct {
	createdSpecs []string // titles passed to CreateSpec
	specID       string   // returned from CreateSpec
	published    []arbiter.ResearchFinding
	findings     []arbiter.ResearchFinding // static override; if nil, returns published
}

func (p *testResearchProvider) CreateSpec(_ context.Context, id, title string) (string, error) {
	p.createdSpecs = append(p.createdSpecs, title)
	return p.specID, nil
}

func (p *testResearchProvider) PublishInsight(_ context.Context, specID string, finding arbiter.ResearchFinding) (string, error) {
	p.published = append(p.published, finding)
	return "insight-1", nil
}

func (p *testResearchProvider) LinkInsight(_ context.Context, insightID, specID string) error {
	return nil
}

func (p *testResearchProvider) FetchLinkedInsights(_ context.Context, specID string) ([]arbiter.ResearchFinding, error) {
	if p.findings != nil {
		return p.findings, nil
	}
	return p.published, nil
}

func (p *testResearchProvider) StartDeepScan(_ context.Context, specID string) (string, error) {
	return "scan-" + specID, nil
}

func (p *testResearchProvider) CheckDeepScan(_ context.Context, scanID string) (bool, error) {
	return true, nil
}

func TestOrchestratorWithResearch_CreatesSpec(t *testing.T) {
	provider := &testResearchProvider{specID: "spec-abc"}
	o := arbiter.NewOrchestratorWithResearch("/tmp/test-project", provider)

	state, err := o.Start(context.Background(), "AI-powered search tool")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if state.SpecID != "spec-abc" {
		t.Errorf("expected SpecID=spec-abc, got %q", state.SpecID)
	}
	if len(provider.createdSpecs) != 1 {
		t.Fatalf("expected 1 CreateSpec call, got %d", len(provider.createdSpecs))
	}
	if provider.createdSpecs[0] != "AI-powered search tool" {
		t.Errorf("unexpected title: %q", provider.createdSpecs[0])
	}
}

func TestOrchestratorWithoutResearch_NoSpecID(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	state, err := o.Start(context.Background(), "simple sprint")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if state.SpecID != "" {
		t.Errorf("expected empty SpecID without research, got %q", state.SpecID)
	}
}

func TestStartWithResearch_PublishesFindings(t *testing.T) {
	provider := &testResearchProvider{specID: "spec-xyz"}
	o := arbiter.NewOrchestratorWithResearch("/tmp/test-project", provider)

	pollardFindings := []arbiter.ResearchFinding{
		{Title: "Competitor A", Summary: "Does X", SourceType: "github", Relevance: 0.9, Tags: []string{"competitive"}},
		{Title: "Trend B", Summary: "Growing fast", SourceType: "hackernews", Relevance: 0.7, Tags: []string{"trends"}},
	}

	state, err := o.StartWithResearch(context.Background(), "search tool", pollardFindings)
	if err != nil {
		t.Fatalf("StartWithResearch failed: %v", err)
	}
	if state.SpecID != "spec-xyz" {
		t.Errorf("expected SpecID=spec-xyz, got %q", state.SpecID)
	}
	if len(provider.published) != 2 {
		t.Fatalf("expected 2 published insights, got %d", len(provider.published))
	}
	if len(state.Findings) != 2 {
		t.Fatalf("expected 2 findings on state, got %d", len(state.Findings))
	}
	if state.Findings[0].Title != "Competitor A" {
		t.Errorf("unexpected first finding: %q", state.Findings[0].Title)
	}
}

func TestOrchestratorAdvanceNilState(t *testing.T) {
	orch := arbiter.NewOrchestrator("/tmp/test")
	_, err := orch.Advance(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil state")
	}
}
