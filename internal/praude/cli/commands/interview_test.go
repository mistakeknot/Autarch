package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/agents"
)

func TestInterviewCommandCreatesSpec(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `validation_mode = "soft"

[agents.codex]
command = "codex"
args = []
`
	if err := os.WriteFile(filepath.Join(root, ".praude", "config.toml"), []byte(cfg), 0o644); err != nil {
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
	input := "n\ny\nVision\nUsers\nProblem\nREQ-001: Do thing\nn\n"
	cmd := InterviewCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetIn(bytes.NewBufferString(input))
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".praude", "specs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected spec created")
	}
	path := filepath.Join(root, ".praude", "specs", entries[0].Name())
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "critical_user_journeys") {
		t.Fatalf("expected cuj section")
	}
	if !strings.Contains(string(raw), "validation_warnings") {
		t.Fatalf("expected validation warnings metadata")
	}
}

func TestInterviewCommandAutoAppliesSuggestions(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude", "suggestions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude", "briefs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `validation_mode = "soft"

[agents.codex]
command = "codex"
args = []
`
	if err := os.WriteFile(filepath.Join(root, ".praude", "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	oldLaunch := launchAgent
	oldSub := launchSubagent
	launchAgent = func(p agents.Profile, briefPath string) error {
		entries, err := os.ReadDir(filepath.Join(root, ".praude", "suggestions"))
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return nil
		}
		path := filepath.Join(root, ".praude", "suggestions", entries[0].Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updated := strings.ReplaceAll(string(raw), "status: pending", "status: ready")
		updated = strings.Replace(updated, "suggestion: \"\"", "suggestion: \"Agent summary\"", 1)
		updated = strings.Replace(updated, "\"REQ-001: Add requirement\"", "\"REQ-002: Agent requirement\"", 1)
		return os.WriteFile(path, []byte(updated), 0o644)
	}
	launchSubagent = launchAgent
	defer func() {
		launchAgent = oldLaunch
		launchSubagent = oldSub
	}()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	input := "n\ny\nVision\nUsers\nProblem\nREQ-001: Do thing\nn\n"
	cmd := InterviewCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetIn(bytes.NewBufferString(input))
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".praude", "specs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected spec created")
	}
	path := filepath.Join(root, ".praude", "specs", entries[0].Name())
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "summary: Agent summary") {
		t.Fatalf("expected agent summary applied")
	}
	if !strings.Contains(string(raw), "REQ-002: Agent requirement") {
		t.Fatalf("expected agent requirements applied")
	}
}

func TestInterviewCommandRunsResearch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude", "research"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude", "briefs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `validation_mode = "soft"

[agents.codex]
command = "codex"
args = []
`
	if err := os.WriteFile(filepath.Join(root, ".praude", "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	called := false
	oldLaunch := launchAgent
	oldSub := launchSubagent
	launchAgent = func(p agents.Profile, briefPath string) error {
		called = true
		return nil
	}
	launchSubagent = func(p agents.Profile, briefPath string) error {
		called = true
		return nil
	}
	defer func() {
		launchAgent = oldLaunch
		launchSubagent = oldSub
	}()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	input := "n\ny\nVision\nUsers\nProblem\nREQ-001: Do thing\ny\n"
	cmd := InterviewCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetIn(bytes.NewBufferString(input))
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".praude", "research"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected research file created")
	}
	if !called {
		t.Fatalf("expected agent launch")
	}
}
