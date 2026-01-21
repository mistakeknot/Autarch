package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

func TestStatusJSONOutput(t *testing.T) {
	dir := t.TempDir()
	if err := project.Init(dir); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	cmd := StatusCmd()
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if payload["initialized"] != true {
		t.Fatalf("expected initialized true")
	}
}
