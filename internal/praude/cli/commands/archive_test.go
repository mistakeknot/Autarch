package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
)

func TestArchiveCommandMovesSpec(t *testing.T) {
	root := t.TempDir()
	if err := project.Init(root); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(project.SpecsDir(root), "PRD-001.yaml")
	if err := os.WriteFile(src, []byte("id: \"PRD-001\"\nsummary: \"S\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := archiveCmdRun(root, "PRD-001"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project.ArchivedSpecsDir(root), "PRD-001.yaml")); err != nil {
		t.Fatalf("expected archived spec")
	}
}
