package arbiter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadSprintState(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "arbiter-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a sprint state
	state := NewSprintState(tmpDir)
	state.ID = "SPRINT-001"
	state.Sections[PhaseProblem].Content = "Test problem statement"
	state.Sections[PhaseProblem].Status = DraftAccepted
	state.Confidence.Completeness = 0.5

	// Save it
	if err := SaveSprintState(state); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	statePath := filepath.Join(tmpDir, ".gurgeh", "sprints", "SPRINT-001.yaml")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatalf("sprint file not created at %s", statePath)
	}

	// Load it back
	loaded, err := LoadSprintState(tmpDir, "SPRINT-001")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Verify content
	if loaded.Sections[PhaseProblem].Content != "Test problem statement" {
		t.Errorf("content mismatch: got %q", loaded.Sections[PhaseProblem].Content)
	}
	if loaded.Confidence.Completeness != 0.5 {
		t.Errorf("confidence mismatch: got %f", loaded.Confidence.Completeness)
	}
}

func TestListSprints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "arbiter-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create and save two sprints
	state1 := NewSprintState(tmpDir)
	state1.ID = "SPRINT-001"
	if err := SaveSprintState(state1); err != nil {
		t.Fatalf("save state1 failed: %v", err)
	}

	state2 := NewSprintState(tmpDir)
	state2.ID = "SPRINT-002"
	if err := SaveSprintState(state2); err != nil {
		t.Fatalf("save state2 failed: %v", err)
	}

	// List sprints
	ids, err := ListSprints(tmpDir)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if len(ids) != 2 {
		t.Errorf("expected 2 sprints, got %d", len(ids))
	}
}

func TestListSprintsEmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "arbiter-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// List sprints on empty dir should return empty slice, not error
	ids, err := ListSprints(tmpDir)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if len(ids) != 0 {
		t.Errorf("expected 0 sprints, got %d", len(ids))
	}
}

func TestSaveSprintStateRejectsPathTraversal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "arbiter-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	state := NewSprintState(tmpDir)
	state.ID = "../../../etc/passwd"

	err = SaveSprintState(state)
	if err == nil {
		t.Error("expected error for path traversal ID")
	}
}

func TestLoadSprintStateRejectsPathTraversal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "arbiter-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = LoadSprintState(tmpDir, "../../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal ID")
	}
}

func TestSaveSprintStateRejectsEmptyID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "arbiter-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	state := NewSprintState(tmpDir)
	state.ID = ""

	err = SaveSprintState(state)
	if err == nil {
		t.Error("expected error for empty ID")
	}
}
