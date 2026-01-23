package plan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPlanningCreatesPlanDocs(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, ".tandemonium", "plan")
	input := strings.NewReader("y\nmy vision\nmy mvp\n")
	var out bytes.Buffer
	if err := Run(input, &out, planDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(planDir, "vision.md")); err != nil {
		t.Fatalf("expected vision.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(planDir, "mvp.md")); err != nil {
		t.Fatalf("expected mvp.md: %v", err)
	}
}

func TestRunPlanningPromptsForVisionAndMVP(t *testing.T) {
	planDir := t.TempDir()
	input := strings.NewReader("y\nvision\nmvp\n")
	var out bytes.Buffer
	if err := Run(input, &out, planDir); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Vision (leave blank to skip):") {
		t.Fatalf("expected vision prompt")
	}
	if !strings.Contains(out.String(), "MVP (leave blank to skip):") {
		t.Fatalf("expected mvp prompt")
	}
}
