package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/suggestions"
)

func TestSuggestionsReviewOutputsCounts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "suggestions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := suggestions.Create(filepath.Join(root, ".praude", "suggestions"), "PRD-001", time.Now()); err != nil {
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
	cmd := SuggestionsCmd()
	cmd.SetArgs([]string{"review", "PRD-001"})
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Summary") || !strings.Contains(out, "Requirements") {
		t.Fatalf("expected summary output, got %q", out)
	}
	if !strings.Contains(out, "CUJ") || !strings.Contains(out, "Market") || !strings.Contains(out, "Competitive") {
		t.Fatalf("expected section output, got %q", out)
	}
}

func TestSuggestionsApplyAllUpdatesSpec(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude", "suggestions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `id: "PRD-001"
title: "Alpha"
summary: "Old"
requirements:
  - "REQ-001: Old"
`
	if err := os.WriteFile(filepath.Join(root, ".praude", "specs", "PRD-001.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `# Suggestions for PRD-001

## Summary
- status: pending
- suggestion: "New summary"

## Requirements
- status: pending
- suggestion:
  - "REQ-001: New requirement"

## Critical User Journeys
- status: pending
- suggestion:
  - id: "CUJ-001"
    title: "Primary Journey"
    priority: "high"
    steps:
      - "Step"
    success_criteria:
      - "Outcome"
    linked_requirements:
      - "REQ-001"

## Market Research
- status: pending
- suggestion:
  - id: "MR-001"
    claim: "Market claim"
    evidence_refs:
      - path: ".praude/research/PRD-001-20260115-000000.md"
        anchor: "section-1"
        note: "Source"
    confidence: "medium"
    date: "2026-01-15"

## Competitive Landscape
- status: pending
- suggestion:
  - id: "COMP-001"
    name: "Competitor"
    positioning: "Positioning"
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
	if err := os.WriteFile(filepath.Join(root, ".praude", "suggestions", "PRD-001-20260115-000000.md"), []byte(body), 0o644); err != nil {
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
	cmd := SuggestionsCmd()
	cmd.SetArgs([]string{"apply", "PRD-001", "--all"})
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".praude", "specs", "PRD-001.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "New summary") {
		t.Fatalf("expected summary updated")
	}
	if !strings.Contains(string(raw), "REQ-001: New requirement") {
		t.Fatalf("expected requirements updated")
	}
}
