package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCmdJSONOutput(t *testing.T) {
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
	cmd.SetArgs([]string{"PRD-001", "--json"})
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("expected json output, got %q", buf.String())
	}
	if payload["id"] != "PRD-001" {
		t.Fatalf("expected id in json")
	}
}
