package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/initflow"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

type failingGenerator struct{}

func (f failingGenerator) Generate(_ context.Context, _ initflow.Input) (initflow.Result, error) {
	return initflow.Result{}, errors.New("boom")
}

func TestInitRunsPlanningWhenConfirmed(t *testing.T) {
	dir := t.TempDir()
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
	prevFactory := initGeneratorFactory
	initGeneratorFactory = func(root, agentName string, out io.Writer) initflow.Generator {
		return failingGenerator{}
	}
	defer func() {
		initGeneratorFactory = prevFactory
	}()
	cmd := newRootCommand()
	cmd.SetArgs([]string{"init"})
	cmd.SetIn(strings.NewReader("2\ny\n"))
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tandemonium", "plan", "exploration.md")); err != nil {
		t.Fatalf("expected exploration.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tandemonium", "specs", "EPIC-001.yaml")); err != nil {
		t.Fatalf("expected EPIC-001.yaml: %v", err)
	}
}

func TestQuickTaskCreatesSpec(t *testing.T) {
	dir := t.TempDir()
	if err := project.Init(dir); err != nil {
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
	cmd := newRootCommand()
	cmd.SetArgs([]string{"-q", "fix", "login", "timeout"})
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".tandemonium", "specs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected a spec file")
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".tandemonium", "specs", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "quick_mode: true") {
		t.Fatal("expected quick_mode true in spec")
	}
}
