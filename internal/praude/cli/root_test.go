package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCommandHasInit(t *testing.T) {
	cmd := NewRoot()
	if cmd == nil || cmd.Use != "praude" {
		t.Fatalf("expected root command")
	}
}

func TestRootCommandHasValidate(t *testing.T) {
	cmd := NewRoot()
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "validate" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected validate command")
	}
}

func TestRootRunPromptsWhenNotInitialized(t *testing.T) {
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	cmd := NewRoot()
	cmd.SetArgs([]string{})
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(bytes.NewBuffer(nil))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "praude init") {
		t.Fatalf("expected init prompt, got %q", buf.String())
	}
}

func TestRootRunLaunchesTUIWhenInitialized(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude"), 0o755); err != nil {
		t.Fatal(err)
	}
	origRun := runTUI
	called := false
	runTUI = func() error {
		called = true
		return nil
	}
	defer func() { runTUI = origRun }()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	cmd := NewRoot()
	cmd.SetArgs([]string{})
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(bytes.NewBuffer(nil))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected TUI run")
	}
}
