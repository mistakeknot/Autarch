package arbiter_test

import (
	"context"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestOrchestratorStartsSprint(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	state, err := o.Start(ctx, "Users can't find relevant research papers quickly")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if state.Phase != arbiter.PhaseVision {
		t.Errorf("expected PhaseVision, got %v", state.Phase)
	}

	section := state.Sections[arbiter.PhaseVision]
	if section == nil {
		t.Fatal("Vision section is nil")
	}
	if section.Content == "" {
		t.Error("Vision section has no content")
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

	// Accept Vision draft, advance to Problem
	state = o.AcceptDraft(state)
	state, err = o.Advance(ctx, state)
	if err != nil {
		t.Fatalf("Advance to Problem failed: %v", err)
	}
	if state.Phase != arbiter.PhaseProblem {
		t.Errorf("expected PhaseProblem, got %v", state.Phase)
	}

	// Accept Problem draft, advance to Users
	state = o.AcceptDraft(state)
	state, err = o.Advance(ctx, state)
	if err != nil {
		t.Fatalf("Advance to Users failed: %v", err)
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

	// Advance through Vision -> Problem -> Users -> FeaturesGoals
	for _, expected := range []arbiter.Phase{arbiter.PhaseProblem, arbiter.PhaseUsers, arbiter.PhaseFeaturesGoals} {
		state = o.AcceptDraft(state)
		state, err = o.Advance(ctx, state)
		if err != nil {
			t.Fatalf("Advance to %v failed: %v", expected, err)
		}
		if state.Phase != expected {
			t.Fatalf("expected %v, got %v", expected, state.Phase)
		}
	}
}

func TestOrchestratorBlocksOnConflicts(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	state, err := o.Start(ctx, "solo developers struggle with code review")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Advance through Vision -> Problem -> Users -> FeaturesGoals
	for _, expected := range []arbiter.Phase{arbiter.PhaseProblem, arbiter.PhaseUsers, arbiter.PhaseFeaturesGoals} {
		state = o.AcceptDraft(state)
		state, err = o.Advance(ctx, state)
		if err != nil {
			t.Fatalf("Advance to %v failed: %v", expected, err)
		}
	}

	// Set problem content about solo developers (matches consistency check)
	state.Sections[arbiter.PhaseProblem].Content = "solo developers struggle with code review"
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftAccepted

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

func (p *testResearchProvider) RunTargetedScan(_ context.Context, _ string, _ []string, _ string, _ string) error {
	return nil
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

func TestStartReview_SeedsFromSpec(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")

	spec := &specs.Spec{
		ID:      "VIS-1",
		Type:    "vision",
		Summary: "A unified platform for AI agent development",
		Goals: []specs.Goal{
			{ID: "G1", Description: "Enable seamless agent coordination"},
		},
		Assumptions: []specs.Assumption{
			{ID: "A1", Description: "Users prefer TUI over web"},
		},
	}

	state, err := o.StartReview(context.Background(), spec, nil)
	if err != nil {
		t.Fatalf("StartReview failed: %v", err)
	}
	if !state.IsReview {
		t.Error("expected IsReview=true")
	}
	if state.ReviewingSpecID != "VIS-1" {
		t.Errorf("expected ReviewingSpecID=VIS-1, got %q", state.ReviewingSpecID)
	}

	// Vision section should have summary content
	vision := state.Sections[arbiter.PhaseVision]
	if vision.Content != "A unified platform for AI agent development" {
		t.Errorf("unexpected vision content: %q", vision.Content)
	}

	// All sections should be auto-accepted when no signals
	for _, phase := range arbiter.AllPhases() {
		s := state.Sections[phase]
		if !s.AutoAccept {
			t.Errorf("phase %v should be auto-accepted with no signals", phase)
		}
		if s.Status != arbiter.DraftAccepted {
			t.Errorf("phase %v should be DraftAccepted, got %v", phase, s.Status)
		}
	}
}

func TestStartReview_SignalsFlagSections(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")

	spec := &specs.Spec{
		ID:   "VIS-1",
		Type: "vision",
		Assumptions: []specs.Assumption{
			{ID: "A1", Description: "Users prefer TUI"},
		},
	}

	signals := []string{"sig-1", "sig-2"}
	state, err := o.StartReview(context.Background(), spec, signals)
	if err != nil {
		t.Fatalf("StartReview failed: %v", err)
	}

	// ScopeAssumptions should be flagged (that's where signals route)
	assumptions := state.Sections[arbiter.PhaseScopeAssumptions]
	if assumptions.AutoAccept {
		t.Error("assumptions should NOT be auto-accepted with active signals")
	}
	if assumptions.Status != arbiter.DraftPending {
		t.Errorf("expected DraftPending, got %v", assumptions.Status)
	}
	if len(assumptions.ActiveSignals) != 2 {
		t.Errorf("expected 2 active signals, got %d", len(assumptions.ActiveSignals))
	}

	// Other sections should still be auto-accepted
	vision := state.Sections[arbiter.PhaseVision]
	if !vision.AutoAccept {
		t.Error("vision should be auto-accepted")
	}
}

func TestOrchestratorAdvanceNilState(t *testing.T) {
	orch := arbiter.NewOrchestrator("/tmp/test")
	_, err := orch.Advance(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil state")
	}
}

func TestProcessChatMessage_NoSprint(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test")
	ctx := context.Background()
	ch := o.ProcessChatMessage(ctx, "hello")
	var msgs []string
	for msg := range ch {
		msgs = append(msgs, msg)
	}
	if len(msgs) != 1 || msgs[0] != "No active sprint. Start a sprint first." {
		t.Errorf("unexpected messages: %v", msgs)
	}
}

func TestProcessChatMessage_StreamsResponse(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test")
	ctx := context.Background()

	_, err := o.Start(ctx, "build a task tracker")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ch := o.ProcessChatMessage(ctx, "make it about developers")
	var msgs []string
	for msg := range ch {
		msgs = append(msgs, msg)
	}
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	if msgs[0] == "" {
		t.Error("expected non-empty response")
	}
}

func TestProcessChatMessage_CancelCtx(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test")
	ctx := context.Background()

	_, err := o.Start(ctx, "build a tracker")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // cancel immediately

	ch := o.ProcessChatMessage(cancelCtx, "should be cancelled")
	var msgs []string
	for msg := range ch {
		msgs = append(msgs, msg)
	}
	// Channel should close quickly; may or may not have messages
	// The key thing is no goroutine leak (channel closes)
}

func TestChatAcceptDraft_AdvancesPhase(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test")
	ctx := context.Background()

	_, err := o.Start(ctx, "build a tool")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = o.ChatAcceptDraft(ctx)
	if err != nil {
		t.Fatalf("ChatAcceptDraft failed: %v", err)
	}

	state := o.State()
	if state.Phase != arbiter.PhaseProblem {
		t.Errorf("expected PhaseProblem after accept, got %v", state.Phase)
	}
}

func TestChatAcceptDraft_NoSprint(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test")
	err := o.ChatAcceptDraft(context.Background())
	if err == nil {
		t.Error("expected error with no active sprint")
	}
}

func TestChatReviseDraft_RecordsFeedback(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test")
	ctx := context.Background()

	_, err := o.Start(ctx, "build a tool")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = o.ChatReviseDraft("needs more focus on developers")
	if err != nil {
		t.Fatalf("ChatReviseDraft failed: %v", err)
	}

	state := o.State()
	section := state.Sections[arbiter.PhaseVision]
	if section.Status != arbiter.DraftNeedsRevision {
		t.Errorf("expected DraftNeedsRevision, got %v", section.Status)
	}
	if len(section.UserEdits) != 1 {
		t.Fatalf("expected 1 edit, got %d", len(section.UserEdits))
	}
	if section.UserEdits[0].Reason != "needs more focus on developers" {
		t.Errorf("unexpected edit reason: %q", section.UserEdits[0].Reason)
	}
}

func TestChatReviseDraft_NoSprint(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test")
	err := o.ChatReviseDraft("feedback")
	if err == nil {
		t.Error("expected error with no active sprint")
	}
}

func TestState_ReturnsNilBeforeStart(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test")
	if o.State() != nil {
		t.Error("expected nil state before Start")
	}
}

func TestState_ReturnsStateAfterStart(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test")
	_, err := o.Start(context.Background(), "test")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if o.State() == nil {
		t.Error("expected non-nil state after Start")
	}
}
