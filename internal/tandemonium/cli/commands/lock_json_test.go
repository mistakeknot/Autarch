package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

func TestLockReserveJSONOutput(t *testing.T) {
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

	cmd := LockCmd()
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetArgs([]string{"reserve", "--owner", "alice", "--exclusive", "--json", "a.go"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("reserve failed: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if _, ok := payload["granted"]; !ok {
		t.Fatalf("expected granted field")
	}
}
