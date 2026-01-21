package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
)

func TestListCommandOutputsSpecs(t *testing.T) {
	root := t.TempDir()
	if err := project.Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project.SpecsDir(root), "PRD-001.yaml"), []byte("id: \"PRD-001\"\ntitle: \"A\"\nsummary: \"S\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	cmd := ListCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("PRD-001")) {
		t.Fatalf("expected PRD-001 in output")
	}
}
