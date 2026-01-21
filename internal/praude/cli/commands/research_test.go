package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/agents"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

func TestResearchCommandCreatesResearch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `id: "PRD-001"
title: "Alpha"
summary: "Summary"
requirements:
  - "REQ-001: R"
acceptance_criteria:
  - id: "ac-1"
    description: "Do thing"
`
	if err := os.WriteFile(filepath.Join(root, ".praude", "specs", "PRD-001.yaml"), []byte(spec), 0o644); err != nil {
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
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	cmd := ResearchCmd()
	cmd.SetArgs([]string{"PRD-001", "--agent=codex"})
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected output lines")
	}
	out := lines[0]
	if !strings.HasPrefix(out, "PRD-001-") || !strings.HasSuffix(out, ".md") {
		t.Fatalf("expected research filename, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".praude", "research", out)); err != nil {
		t.Fatalf("expected research file created: %v", err)
	}
	briefsDir := filepath.Join(root, ".praude", "briefs")
	entries, err := os.ReadDir(briefsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected brief file created")
	}
	if !strings.Contains(buf.String(), "agent not found") {
		t.Fatalf("expected agent not found message")
	}
}

func TestResearchCommandUsesSubagentForClaude(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
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
	spec := `id: "PRD-001"
title: "Alpha"
summary: "Summary"
requirements:
  - "REQ-001: R"
acceptance_criteria:
  - id: "ac-1"
    description: "Do thing"
`
	if err := os.WriteFile(filepath.Join(root, ".praude", "specs", "PRD-001.yaml"), []byte(spec), 0o644); err != nil {
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
	cmd := ResearchCmd()
	cmd.SetArgs([]string{"PRD-001", "--agent=claude"})
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
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

func TestBuildResearchBriefIncludesOssScan(t *testing.T) {
	spec := specs.Spec{
		ID:           "PRD-001",
		Title:        "Alpha",
		Summary:      "Summary",
		Requirements: []string{"REQ-1"},
	}
	brief := buildResearchBrief(spec, ".praude/research/PRD-001-20260115-120000.md", []string{"Do thing"})
	if !strings.Contains(brief, "OSS project scan") {
		t.Fatalf("expected OSS scan instructions")
	}
	if !strings.Contains(brief, "evidence refs") {
		t.Fatalf("expected evidence refs instructions")
	}
}
