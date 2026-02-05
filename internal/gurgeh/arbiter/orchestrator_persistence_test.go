package arbiter

import (
	"context"
	"os"
	"testing"
)

func TestOrchestratorSaveAndResume(t *testing.T) {
	// Create a temp dir to act as project path
	tmpDir, err := os.MkdirTemp("", "orch-persist-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create orchestrator and start a sprint
	orch := NewOrchestrator(tmpDir)
	state, err := orch.Start(context.Background(), "Test project description")
	if err != nil {
		t.Fatalf("failed to start sprint: %v", err)
	}

	// Check that sprint file was saved
	sprintDir := tmpDir + "/.gurgeh/sprints"
	entries, err := os.ReadDir(sprintDir)
	if err != nil {
		t.Fatalf("sprint dir not created: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 sprint file, got %d", len(entries))
	}

	// Create a new orchestrator and resume
	orch2 := NewOrchestrator(tmpDir)

	// List sprints
	ids, err := orch2.ListSprints()
	if err != nil {
		t.Fatalf("failed to list sprints: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 sprint, got %d", len(ids))
	}

	// Resume
	resumed, err := orch2.Resume(ids[0])
	if err != nil {
		t.Fatalf("failed to resume: %v", err)
	}

	// Verify it matches
	if resumed.ID != state.ID {
		t.Errorf("ID mismatch: %s vs %s", resumed.ID, state.ID)
	}
	if resumed.Phase != state.Phase {
		t.Errorf("Phase mismatch: %s vs %s", resumed.Phase, state.Phase)
	}
	visionContent := state.Sections[PhaseVision].Content
	resumedContent := resumed.Sections[PhaseVision].Content
	if resumedContent != visionContent {
		t.Errorf("Content mismatch:\n  Original: %.100s...\n  Resumed: %.100s...", visionContent, resumedContent)
	}
}

func TestOrchestratorSavesOnAdvance(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "orch-advance-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	orch := NewOrchestrator(tmpDir)
	_, err = orch.Start(context.Background(), "Test project")
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	// Accept and advance
	err = orch.ChatAcceptDraft(context.Background())
	if err != nil {
		t.Fatalf("failed to advance: %v", err)
	}

	// Get current state for comparison
	currentState, _ := orch.State()

	// Resume in new orchestrator - should be on Phase 2
	orch2 := NewOrchestrator(tmpDir)
	ids, _ := orch2.ListSprints()
	resumed, err := orch2.Resume(ids[0])
	if err != nil {
		t.Fatalf("failed to resume: %v", err)
	}

	if resumed.Phase != currentState.Phase {
		t.Errorf("Phase not persisted after advance: got %s, want %s", resumed.Phase, currentState.Phase)
	}
}

func TestOrchestratorRevert(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "orch-revert-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	orch := NewOrchestrator(tmpDir)
	_, err = orch.Start(context.Background(), "Test project")
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	// Verify starting on first phase (Vision)
	state, _ := orch.State()
	if state.Phase != PhaseVision {
		t.Fatalf("expected to start on PhaseVision, got %s", state.Phase)
	}

	// Try to revert from first phase - should fail
	_, reverted := orch.Revert()
	if reverted {
		t.Error("should not be able to revert from first phase")
	}

	// Advance to next phase
	err = orch.ChatAcceptDraft(context.Background())
	if err != nil {
		t.Fatalf("failed to advance: %v", err)
	}

	state, _ = orch.State()
	if state.Phase != PhaseProblem {
		t.Fatalf("expected PhaseProblem after advance, got %s", state.Phase)
	}

	// Revert should now work
	revertedState, reverted := orch.Revert()
	if !reverted {
		t.Error("should be able to revert from second phase")
	}
	if revertedState.Phase != PhaseVision {
		t.Errorf("expected to revert to PhaseVision, got %s", revertedState.Phase)
	}

	// Verify state is updated
	state, _ = orch.State()
	if state.Phase != PhaseVision {
		t.Errorf("orchestrator state should be on PhaseVision, got %s", state.Phase)
	}
}
