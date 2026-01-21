package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/agents"
)

func TestRunCommandUnknownAgent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `validation_mode = "soft"

[agents.codex]
command = "does-not-exist"
args = []
`
	if err := os.WriteFile(filepath.Join(root, ".praude", "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	briefPath := filepath.Join(root, "brief.md")
	if err := os.WriteFile(briefPath, []byte("brief"), 0o644); err != nil {
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
	cmd := RunCmd()
	cmd.SetArgs([]string{briefPath, "--agent=unknown"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for unknown agent")
	}
}

func TestRunCommandAgentNotFound(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `validation_mode = "soft"

[agents.codex]
command = "does-not-exist"
args = []
`
	if err := os.WriteFile(filepath.Join(root, ".praude", "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	briefPath := filepath.Join(root, "brief.md")
	if err := os.WriteFile(briefPath, []byte("brief"), 0o644); err != nil {
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
	cmd := RunCmd()
	cmd.SetArgs([]string{briefPath, "--agent=codex"})
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "agent not found") {
		t.Fatalf("expected agent not found message")
	}
	if !strings.Contains(out, briefPath) {
		t.Fatalf("expected brief path in output")
	}
}

func TestRunCommandUsesSubagentForClaude(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `validation_mode = "soft"

[agents.claude]
command = "claude"
args = []
`
	if err := os.WriteFile(filepath.Join(root, ".praude", "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	briefPath := filepath.Join(root, "brief.md")
	if err := os.WriteFile(briefPath, []byte("brief"), 0o644); err != nil {
		t.Fatal(err)
	}
	calledSub := false
	calledAgent := false
	oldSub := launchSubagent
	oldAgent := launchAgent
	launchSubagent = func(p agents.Profile, briefPath string) error {
		calledSub = true
		return nil
	}
	launchAgent = func(p agents.Profile, briefPath string) error {
		calledAgent = true
		return nil
	}
	defer func() {
		launchSubagent = oldSub
		launchAgent = oldAgent
	}()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	cmd := RunCmd()
	cmd.SetArgs([]string{briefPath, "--agent=claude"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !calledSub {
		t.Fatalf("expected subagent launch")
	}
	if calledAgent {
		t.Fatalf("expected main launch to be skipped")
	}
}
