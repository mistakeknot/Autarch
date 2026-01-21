package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanCommandRunsPlanning(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".tandemonium", "plan"), 0o755); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	cmd := PlanCmd()
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetIn(strings.NewReader("n\n"))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
