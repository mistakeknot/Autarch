package arbiter_test

import (
	"context"
	"math"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
)

// --- Full phase-transition walk ---

func TestFullSprintWalkAllPhases(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	state, err := o.Start(ctx, "Build a distributed task runner")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if state.Phase != arbiter.PhaseVision {
		t.Fatalf("expected PhaseVision, got %v", state.Phase)
	}

	expected := []arbiter.Phase{
		arbiter.PhaseProblem,
		arbiter.PhaseUsers,
		arbiter.PhaseFeaturesGoals,
		arbiter.PhaseCUJs,             // User journeys flow from users + features
		arbiter.PhaseRequirements,     // Requirements derived from CUJs
		arbiter.PhaseScopeAssumptions,
		arbiter.PhaseAcceptanceCriteria,
	}

	for _, want := range expected {
		state = o.AcceptDraft(state)
		state, err = o.Advance(ctx, state)
		if err != nil {
			t.Fatalf("Advance to %v failed: %v", want, err)
		}
		if state.Phase != want {
			t.Fatalf("expected %v, got %v", want, state.Phase)
		}
		// Each phase should have a proposed draft
		section := state.Sections[want]
		if section == nil {
			t.Fatalf("section for %v is nil", want)
		}
		if section.Content == "" {
			t.Errorf("section for %v has empty content", want)
		}
		if section.Status != arbiter.DraftProposed {
			t.Errorf("expected DraftProposed for %v, got %v", want, section.Status)
		}
	}
}

// --- Last-phase accept: should not advance past the final phase ---

func TestChatAcceptDraft_FinalPhaseStaysOnPhase(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	_, err := o.Start(ctx, "Build something")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Walk to the last phase
	phases := arbiter.AllPhases()
	for i := 0; i < len(phases)-1; i++ {
		if err := o.ChatAcceptDraft(ctx); err != nil {
			t.Fatalf("ChatAcceptDraft at phase %d failed: %v", i, err)
		}
	}

	// Verify we're on the last phase
	state, ok := o.State()
	if !ok {
		t.Fatal("expected state to exist")
	}
	if state.Phase != arbiter.PhaseAcceptanceCriteria {
		t.Fatalf("expected PhaseAcceptanceCriteria, got %v", state.Phase)
	}

	// Accept the last phase — should not error, phase should remain the same
	err = o.ChatAcceptDraft(ctx)
	if err != nil {
		t.Fatalf("ChatAcceptDraft on last phase failed: %v", err)
	}

	state, _ = o.State()
	if state.Phase != arbiter.PhaseAcceptanceCriteria {
		t.Errorf("expected phase to stay at AcceptanceCriteria, got %v", state.Phase)
	}
}

// --- Blocker gating: resolve conflict and retry ---

func TestBlockerResolution_EditAndRetry(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	state, err := o.Start(ctx, "solo dev code review tool")
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

	// Inject conflict: problem says "solo developers" but features say "enterprise"
	state.Sections[arbiter.PhaseProblem].Content = "solo developers struggle with code review"
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftAccepted
	state.Sections[arbiter.PhaseFeaturesGoals].Content = "enterprise admin dashboard for 100+ users"
	state.Sections[arbiter.PhaseFeaturesGoals].Status = arbiter.DraftAccepted

	// Should be blocked
	_, err = o.Advance(ctx, state)
	if !arbiter.IsBlockerError(err) {
		t.Fatalf("expected blocker error, got: %v", err)
	}

	// Fix the conflict by aligning features with the problem
	state.Sections[arbiter.PhaseFeaturesGoals].Content = "automated code review feedback for solo developers"
	state.Conflicts = nil // clear stale conflicts

	// Should now advance
	state, err = o.Advance(ctx, state)
	if err != nil {
		t.Fatalf("expected advance after resolution, got error: %v", err)
	}
	if state.Phase != arbiter.PhaseCUJs {
		t.Errorf("expected PhaseCUJs after resolution, got %v", state.Phase)
	}
}

// --- Confidence scoring through orchestrator ---

func TestConfidenceIncreasesWithAcceptedPhases(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	_, err := o.Start(ctx, "Build a tool")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Accept first phase and advance
	state, _ := o.State()
	statePtr := &state
	statePtr = o.AcceptDraft(statePtr)
	statePtr, err = o.Advance(ctx, statePtr)
	if err != nil {
		t.Fatalf("Advance failed: %v", err)
	}
	conf1 := statePtr.Confidence.Completeness

	// Accept second phase and advance
	statePtr = o.AcceptDraft(statePtr)
	statePtr, err = o.Advance(ctx, statePtr)
	if err != nil {
		t.Fatalf("Advance failed: %v", err)
	}
	conf2 := statePtr.Confidence.Completeness

	if conf2 <= conf1 {
		t.Errorf("confidence completeness should increase with more accepted phases: %.3f -> %.3f", conf1, conf2)
	}
}

func TestConfidenceTotalWeightedCorrectly(t *testing.T) {
	score := arbiter.ConfidenceScore{
		Completeness: 1.0,
		Consistency:  1.0,
		Specificity:  1.0,
		Research:     1.0,
		Assumptions:  1.0,
	}
	total := score.Total()
	if !approxEqual(total, 1.0) {
		t.Errorf("expected total=1.0 for all-1.0 scores, got %f", total)
	}

	score2 := arbiter.ConfidenceScore{
		Completeness: 0.5,
		Consistency:  0.5,
		Specificity:  0.5,
		Research:     0.5,
		Assumptions:  0.5,
	}
	total2 := score2.Total()
	if !approxEqual(total2, 0.5) {
		t.Errorf("expected total=0.5 for all-0.5 scores, got %f", total2)
	}
}

func TestConfidenceConflictsReduceConsistency(t *testing.T) {
	// Test that the ConfidenceScore.Total() formula correctly penalizes low consistency.
	// The orchestrator's consistency check is deterministic from section content,
	// so we verify the scoring formula directly.
	clean := arbiter.ConfidenceScore{
		Completeness: 0.5,
		Consistency:  1.0,
		Specificity:  0.5,
		Research:     0.5,
		Assumptions:  0.5,
	}
	conflicted := arbiter.ConfidenceScore{
		Completeness: 0.5,
		Consistency:  0.33,
		Specificity:  0.5,
		Research:     0.5,
		Assumptions:  0.5,
	}
	if conflicted.Total() >= clean.Total() {
		t.Errorf("lower consistency should reduce total: clean=%.3f, conflicted=%.3f",
			clean.Total(), conflicted.Total())
	}
}

// --- Advance on unaccepted draft ---

func TestAdvanceWithUnacceptedDraft(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	state, err := o.Start(ctx, "Build a tool")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Try to advance without accepting — should still work (Advance doesn't check acceptance)
	// The draft is in DraftProposed state
	state, err = o.Advance(ctx, state)
	if err != nil {
		// If Advance requires acceptance, that's fine too
		t.Logf("Advance without accept returned error (expected in some designs): %v", err)
		return
	}
	// If it succeeded, we should be on PhaseProblem
	if state.Phase != arbiter.PhaseProblem {
		t.Errorf("expected PhaseProblem, got %v", state.Phase)
	}
}

// --- Multiple revisions before accept ---

func TestMultipleRevisionsBeforeAccept(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/test-project")
	ctx := context.Background()

	_, err := o.Start(ctx, "Build a search engine")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// First revision
	if err := o.ChatReviseDraft("focus on academic papers"); err != nil {
		t.Fatalf("ChatReviseDraft 1 failed: %v", err)
	}

	// Second revision
	if err := o.ChatReviseDraft("also include preprints"); err != nil {
		t.Fatalf("ChatReviseDraft 2 failed: %v", err)
	}

	state, _ := o.State()
	section := state.Sections[arbiter.PhaseVision]
	if len(section.UserEdits) != 2 {
		t.Errorf("expected 2 user edits, got %d", len(section.UserEdits))
	}
	if section.Status != arbiter.DraftNeedsRevision {
		t.Errorf("expected DraftNeedsRevision, got %v", section.Status)
	}

	// Accept after revisions should work
	if err := o.ChatAcceptDraft(ctx); err != nil {
		t.Fatalf("ChatAcceptDraft after revisions failed: %v", err)
	}

	state, _ = o.State()
	if state.Phase != arbiter.PhaseProblem {
		t.Errorf("expected PhaseProblem after accept, got %v", state.Phase)
	}
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}
