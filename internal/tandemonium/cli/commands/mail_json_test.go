package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

func TestMailSendJSONOutput(t *testing.T) {
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

	cmd := MailCmd()
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetArgs([]string{"send", "--to", "bob", "--subject", "Hello", "--body", "Body", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if payload["id"] == "" {
		t.Fatalf("expected id in json output")
	}
}
