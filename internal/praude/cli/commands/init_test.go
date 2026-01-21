package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCommandCreatesLayoutAndTemplate(t *testing.T) {
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	cmd := InitCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".praude", "specs", "PRD-001.yaml")); err != nil {
		t.Fatalf("expected template spec: %v", err)
	}
}
