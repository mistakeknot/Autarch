package planstatus

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPlanStatusCommandWritesFile(t *testing.T) {
	tmp := t.TempDir()
	plansDir := filepath.Join(tmp, "docs", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "2026-01-22-sample-plan.md"), []byte("# plan\n"), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	outPath := filepath.Join(tmp, "STATUS.md")
	cmd := NewCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--repo", tmp, "--intermute", tmp, "--output", outPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output file, got error: %v", err)
	}
}
