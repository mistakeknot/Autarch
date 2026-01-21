package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPlanningCreatesPlanDocs(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, ".tandemonium", "plan")
	input := strings.NewReader("y\nmy vision\nmy mvp\n")
	if err := Run(input, planDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(planDir, "vision.md")); err != nil {
		t.Fatalf("expected vision.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(planDir, "mvp.md")); err != nil {
		t.Fatalf("expected mvp.md: %v", err)
	}
}
