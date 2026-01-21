package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShowCommandSummarizesSpec(t *testing.T) {
	root := t.TempDir()
	specsDir := filepath.Join(root, ".praude", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `id: "PRD-001"
title: "Alpha"
summary: "Summary"
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
	if err := os.WriteFile(filepath.Join(specsDir, "PRD-001.yaml"), []byte(spec), 0o644); err != nil {
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
	cmd := ShowCmd()
	cmd.SetArgs([]string{"PRD-001"})
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "CUJ") || !strings.Contains(out, "Market") || !strings.Contains(out, "Competitive") {
		t.Fatalf("expected summary output")
	}
}

func TestShowCommandIncludesValidationWarnings(t *testing.T) {
	root := t.TempDir()
	specsDir := filepath.Join(root, ".praude", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `id: "PRD-001"
title: "Alpha"
summary: "Summary"
metadata:
  validation_warnings:
    - "Missing market research"
`
	if err := os.WriteFile(filepath.Join(specsDir, "PRD-001.yaml"), []byte(spec), 0o644); err != nil {
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
	cmd := ShowCmd()
	cmd.SetArgs([]string{"PRD-001"})
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Validation warnings") || !strings.Contains(out, "Missing market research") {
		t.Fatalf("expected validation warnings in output")
	}
}
