package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

func TestScanCommandWritesSummary(t *testing.T) {
	root := t.TempDir()
	if err := project.Init(root); err != nil {
		t.Fatal(err)
	}

	cmd := ScanCmd()
	cmd.SetArgs([]string{root})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, ".tandemonium", "plan", "exploration.md")); err != nil {
		t.Fatalf("expected summary file: %v", err)
	}
}
