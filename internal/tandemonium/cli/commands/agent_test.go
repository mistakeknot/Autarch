package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

func TestAgentRegisterAndWhois(t *testing.T) {
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

	register := AgentCmd()
	register.SetOut(bytes.NewBuffer(nil))
	register.SetArgs([]string{"register", "--name", "BlueLake", "--program", "codex-cli", "--model", "gpt-5", "--task", "Auth refactor", "--json"})
	if err := register.Execute(); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	whois := AgentCmd()
	out := bytes.NewBuffer(nil)
	whois.SetOut(out)
	whois.SetArgs([]string{"whois", "--name", "BlueLake", "--json"})
	if err := whois.Execute(); err != nil {
		t.Fatalf("whois failed: %v", err)
	}

	var payload struct {
		Name         string `json:"name"`
		Program      string `json:"program"`
		Model        string `json:"model"`
		Task         string `json:"task_description"`
		CreatedAt    string `json:"created_ts"`
		LastActiveAt string `json:"last_active_ts"`
		UpdatedAt    string `json:"updated_ts"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if payload.Name != "BlueLake" {
		t.Fatalf("expected name BlueLake, got %q", payload.Name)
	}
	if payload.Program != "codex-cli" || payload.Model != "gpt-5" {
		t.Fatalf("unexpected program/model")
	}
	if payload.Task != "Auth refactor" {
		t.Fatalf("unexpected task description")
	}
	if payload.CreatedAt == "" || payload.LastActiveAt == "" || payload.UpdatedAt == "" {
		t.Fatalf("expected timestamps")
	}
}

func TestAgentEnsureAndHealth(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	ensure := AgentCmd()
	ensureOut := bytes.NewBuffer(nil)
	ensure.SetOut(ensureOut)
	ensure.SetArgs([]string{"ensure", "--json"})
	if err := ensure.Execute(); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}

	var ensurePayload struct {
		ProjectRoot string `json:"project_root"`
		Initialized bool   `json:"initialized"`
	}
	if err := json.Unmarshal(ensureOut.Bytes(), &ensurePayload); err != nil {
		t.Fatalf("decode ensure json: %v", err)
	}
	if !ensurePayload.Initialized || ensurePayload.ProjectRoot == "" {
		t.Fatalf("expected initialized project")
	}

	health := AgentCmd()
	healthOut := bytes.NewBuffer(nil)
	health.SetOut(healthOut)
	health.SetArgs([]string{"health", "--json"})
	if err := health.Execute(); err != nil {
		t.Fatalf("health failed: %v", err)
	}

	var healthPayload struct {
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(healthOut.Bytes(), &healthPayload); err != nil {
		t.Fatalf("decode health json: %v", err)
	}
	if healthPayload.Status != "ok" || healthPayload.Timestamp == "" {
		t.Fatalf("unexpected health payload")
	}
}
