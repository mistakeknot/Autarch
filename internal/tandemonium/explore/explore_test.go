package explore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExploreWritesSummary(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, ".tandemonium", "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := Run(root, planDir, Options{EmitProgress: func(string) {}})
	if err != nil {
		t.Fatal(err)
	}
	if out.SummaryPath == "" {
		t.Fatalf("expected summary path")
	}
	if _, err := os.Stat(out.SummaryPath); err != nil {
		t.Fatalf("expected summary file: %v", err)
	}
}
