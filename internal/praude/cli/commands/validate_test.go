package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCmdRejectsInvalidMode(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".praude", "config.toml"), []byte("validation_mode = \"weird\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := "id: \"PRD-001\"\ntitle: \"T\"\nsummary: \"S\"\n"
	if err := os.WriteFile(filepath.Join(root, ".praude", "specs", "PRD-001.yaml"), []byte(spec), 0o644); err != nil {
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
	cmd := ValidateCmd()
	cmd.SetArgs([]string{"PRD-001"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected invalid mode error")
	}
}

func TestValidateCmdSoftModeStoresWarnings(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".praude", "config.toml"), []byte("validation_mode = \"soft\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := "id: \"PRD-001\"\ntitle: \"T\"\nsummary: \"S\"\nrequirements:\n  - \"REQ-001: R\"\n"
	specPath := filepath.Join(root, ".praude", "specs", "PRD-001.yaml")
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
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
	cmd := ValidateCmd()
	cmd.SetArgs([]string{"PRD-001"})
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(updated, []byte("validation_warnings:")) {
		t.Fatalf("expected validation warnings to be stored")
	}
	if !strings.Contains(buf.String(), "WARN:") {
		t.Fatalf("expected warnings in output")
	}
}

func TestValidateCmdPrintsErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".praude", "config.toml"), []byte("validation_mode = \"hard\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := "id: \"PRD-001\"\ntitle: \"T\"\nsummary: \"S\"\ncritical_user_journeys:\n  - id: \"CUJ-001\"\n    title: \"Journey\"\n    priority: \"high\"\n    steps:\n      - \"Step\"\n    success_criteria:\n      - \"Outcome\"\n    linked_requirements:\n      - \"REQ-404\"\n"
	specPath := filepath.Join(root, ".praude", "specs", "PRD-001.yaml")
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
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
	cmd := ValidateCmd()
	cmd.SetArgs([]string{"PRD-001"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected validation error")
	}
}
