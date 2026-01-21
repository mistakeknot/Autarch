package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

func TestRecoverRebuildsFromSpecs(t *testing.T) {
	root := t.TempDir()
	if err := project.Init(root); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(project.SpecsDir(root), "TAND-001.yaml")
	if err := os.WriteFile(specPath, []byte("id: TAND-001\ntitle: Test\nstatus: review\n"), 0o644); err != nil {
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
	cmd := RecoverCmd()
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetIn(strings.NewReader("y\n"))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Rebuilt state") {
		t.Fatalf("expected rebuild output, got %s", out.String())
	}
}
