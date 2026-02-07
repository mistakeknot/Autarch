package drift

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReconcileActionItems(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, ".coldwine", "tasks"), 0o755); err != nil {
		t.Fatalf("mkdir tasks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".coldwine", "tasks", "TASK-001.yaml"), []byte("id: TASK-001\ntitle: Fix parser timeout\n"), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatalf("mkdir beads: %v", err)
	}
	beadsContent := `{"id":"Autarch-abc","title":"Harden parser timeout behavior"}`
	if err := os.WriteFile(filepath.Join(root, ".beads", "issues.json"), []byte(beadsContent), 0o644); err != nil {
		t.Fatalf("write beads: %v", err)
	}

	items := []ActionItem{
		{Text: "Implement TASK-001 acceptance checks"},
		{Text: "Harden parser timeout behavior in docs"},
		{Text: "Completely unrelated untracked item"},
	}

	reconciled, err := ReconcileActionItems(root, items)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(reconciled) != len(items) {
		t.Fatalf("expected %d results, got %d", len(items), len(reconciled))
	}
	if !reconciled[0].Tracked {
		t.Fatalf("expected TASK-001 item to be tracked")
	}
	if !reconciled[1].Tracked {
		t.Fatalf("expected title-matched item to be tracked")
	}
	if reconciled[2].Tracked {
		t.Fatalf("expected unrelated item to remain untracked")
	}
}
