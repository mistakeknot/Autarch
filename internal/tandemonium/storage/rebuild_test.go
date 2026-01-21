package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

func TestRebuildFromSpecsLoadsTasks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(project.SpecsDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(project.SpecsDir(root), "TAND-001.yaml")
	if err := os.WriteFile(specPath, []byte("id: TAND-001\ntitle: Test\nstatus: review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RebuildFromSpecs(root); err != nil {
		t.Fatal(err)
	}
	db, err := OpenShared(project.StateDBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GetTask(db, "TAND-001"); err != nil {
		t.Fatalf("expected task from spec: %v", err)
	}
}
