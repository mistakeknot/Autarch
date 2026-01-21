package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/agents"
)

func TestViewIncludesHeaders(t *testing.T) {
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "PRDs") || !strings.Contains(out, "DETAILS") {
		t.Fatalf("expected headers")
	}
}

func TestViewShowsFirstSpec(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := "id: \"PRD-001\"\ntitle: \"Alpha\"\nsummary: \"First\"\n"
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
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "First") {
		t.Fatalf("expected spec content in view")
	}
}

func TestViewShowsCompleteness(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `id: "PRD-001"
title: "Alpha"
summary: "First"
requirements:
  - "REQ-001: R"
critical_user_journeys:
  - id: "CUJ-001"
    title: "Journey"
    priority: "high"
    steps:
      - "Step"
    success_criteria:
      - "Outcome"
    linked_requirements:
      - "REQ-001"
market_research:
  - id: "MR-001"
    claim: "Market"
    evidence_refs:
      - path: ".praude/research/PRD-001-20260115-000000.md"
        anchor: "section-1"
        note: "Source"
    confidence: "medium"
    date: "2026-01-15"
competitive_landscape:
  - id: "COMP-001"
    name: "Competitor"
    positioning: "Position"
    strengths:
      - "Strength"
    weaknesses:
      - "Weakness"
    risk: "Medium"
    evidence_refs:
      - path: ".praude/research/PRD-001-20260115-000000.md"
        anchor: "section-2"
        note: "Source"
`
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
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "Completeness:") {
		t.Fatalf("expected completeness line")
	}
}

func TestViewShowsCUJAndResearchDetails(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `id: "PRD-001"
title: "Alpha"
summary: "First"
requirements:
  - "REQ-001: R"
critical_user_journeys:
  - id: "CUJ-001"
    title: "Journey"
    priority: "high"
    steps:
      - "Step"
    success_criteria:
      - "Outcome"
    linked_requirements:
      - "REQ-001"
market_research:
  - id: "MR-001"
    claim: "Market"
    evidence_refs:
      - path: ".praude/research/PRD-001-20260115-000000.md"
        anchor: "section-1"
        note: "Source"
    confidence: "medium"
    date: "2026-01-15"
competitive_landscape:
  - id: "COMP-001"
    name: "Competitor"
    positioning: "Position"
    strengths:
      - "Strength"
    weaknesses:
      - "Weakness"
    risk: "Medium"
    evidence_refs:
      - path: ".praude/research/PRD-001-20260115-000000.md"
        anchor: "section-2"
        note: "Source"
`
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
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "CUJ:") || !strings.Contains(out, "Market:") || !strings.Contains(out, "Competitive:") {
		t.Fatalf("expected section details")
	}
}

func TestViewShowsValidationWarnings(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `id: "PRD-001"
title: "Alpha"
summary: "First"
metadata:
  validation_warnings:
    - "Missing market research"
`
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
	m := NewModel()
	out := stripANSI(m.View())
	if !strings.Contains(out, "Validation Warnings") || !strings.Contains(out, "Missing market research") {
		t.Fatalf("expected validation warnings in view")
	}
}

func TestFocusIndicatorShown(t *testing.T) {
	m := NewModel()
	out := stripANSI(m.View())
	if !strings.Contains(out, "[LIST]") {
		t.Fatalf("expected focus indicator")
	}
	m = pressKey(m, "tab")
	out = stripANSI(m.View())
	if !strings.Contains(out, "[DETAIL]") {
		t.Fatalf("expected detail focus indicator")
	}
}

func TestKeyLaunchesResearchAndSetsStatus(t *testing.T) {
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
	spec := "id: \"PRD-001\"\ntitle: \"Alpha\"\nsummary: \"First\"\n"
	if err := os.WriteFile(filepath.Join(root, ".praude", "specs", "PRD-001.yaml"), []byte(spec), 0o644); err != nil {
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
	m := NewModel()
	m = pressKey(m, "r")
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
	if !strings.Contains(m.View(), "Last action:") {
		t.Fatalf("expected last action in view")
	}
}

func TestKeyLaunchesSuggestionsAndSetsStatus(t *testing.T) {
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
	spec := "id: \"PRD-001\"\ntitle: \"Alpha\"\nsummary: \"First\"\n"
	if err := os.WriteFile(filepath.Join(root, ".praude", "specs", "PRD-001.yaml"), []byte(spec), 0o644); err != nil {
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
	m := NewModel()
	m = pressKey(m, "p")
	entries, err := os.ReadDir(filepath.Join(root, ".praude", "suggestions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected suggestion file created")
	}
	if !called {
		t.Fatalf("expected agent launch")
	}
	if !strings.Contains(m.View(), "Last action:") {
		t.Fatalf("expected last action in view")
	}
}

func stripANSI(input string) string {
	re := regexp.MustCompile(`\x1b\\[[0-9;]*m`)
	return re.ReplaceAllString(input, "")
}
