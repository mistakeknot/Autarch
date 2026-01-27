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

func TestOrchestratorAdvanceNilState(t *testing.T) {
	orch := arbiter.NewOrchestrator("/tmp/test")
	_, err := orch.Advance(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil state")
	}
}
