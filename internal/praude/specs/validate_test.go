package specs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateMissingTitle(t *testing.T) {
	raw := []byte("id: \"PRD-001\"\n")
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Errors) == 0 {
		t.Fatalf("expected error")
	}
}

func TestValidateStatus(t *testing.T) {
	raw := []byte("id: \"PRD-001\"\ntitle: \"A\"\nsummary: \"S\"\nstatus: \"bogus\"\n")
	res, err := Validate(raw, ValidationOptions{Mode: ValidationSoft})
	if err != nil {
		t.Fatal(err)
	}
	if !containsWarning(res.Warnings, "invalid status: bogus") {
		t.Fatalf("expected status warning")
	}
}

func TestValidateHardErrorsOnMissingLinkedRequirement(t *testing.T) {
	root := t.TempDir()
	raw := baseSpecYAML()
	raw = []byte(string(raw) + `
critical_user_journeys:
  - id: "CUJ-001"
    title: "Signup"
    priority: "high"
    steps:
      - "Open page"
    success_criteria:
      - "Account created"
    linked_requirements:
      - "REQ-999"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard, Root: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Errors) == 0 {
		t.Fatalf("expected error for missing requirement link")
	}
}

func TestValidateSoftWarnsOnMissingLinkedRequirement(t *testing.T) {
	root := t.TempDir()
	raw := baseSpecYAML()
	raw = []byte(string(raw) + `
critical_user_journeys:
  - id: "CUJ-001"
    title: "Signup"
    priority: "high"
    steps:
      - "Open page"
    success_criteria:
      - "Account created"
    linked_requirements:
      - "REQ-999"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationSoft, Root: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("expected no errors")
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected warning for missing requirement link")
	}
}

func TestValidateHardErrorsOnMissingEvidenceFile(t *testing.T) {
	root := t.TempDir()
	raw := baseSpecYAML()
	raw = []byte(string(raw) + `
market_research:
  - id: "MR-001"
    claim: "Market is growing"
    evidence_refs:
      - path: ".praude/research/PRD-001-20260115-000000.md"
        anchor: "section-1"
        note: "Source quote"
    confidence: "medium"
    date: "2026-01-15"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard, Root: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Errors) == 0 {
		t.Fatalf("expected error for missing evidence file")
	}
}

func TestValidateSoftWarnsOnMissingEvidenceFile(t *testing.T) {
	root := t.TempDir()
	raw := baseSpecYAML()
	raw = []byte(string(raw) + `
market_research:
  - id: "MR-001"
    claim: "Market is growing"
    evidence_refs:
      - path: ".praude/research/PRD-001-20260115-000000.md"
        anchor: "section-1"
        note: "Source quote"
    confidence: "medium"
    date: "2026-01-15"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationSoft, Root: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("expected no errors")
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected warning for missing evidence file")
	}
}

func TestValidateHardWarnsWhenMarketCompetitiveMissing(t *testing.T) {
	root := t.TempDir()
	raw := baseSpecYAML()
	raw = []byte(string(raw) + `
critical_user_journeys:
  - id: "CUJ-001"
    title: "Signup"
    priority: "high"
    steps:
      - "Open page"
    success_criteria:
      - "Account created"
    linked_requirements:
      - "REQ-001"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard, Root: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("expected no errors")
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected warning for missing optional sections")
	}
}

func TestValidateRejectsDuplicateCUJIDs(t *testing.T) {
	root := t.TempDir()
	raw := baseSpecYAML()
	raw = []byte(string(raw) + `
critical_user_journeys:
  - id: "CUJ-001"
    title: "Signup"
    priority: "high"
    steps:
      - "Open page"
    success_criteria:
      - "Account created"
    linked_requirements:
      - "REQ-001"
  - id: "CUJ-001"
    title: "Duplicate"
    priority: "low"
    steps:
      - "Step"
    success_criteria:
      - "Outcome"
    linked_requirements:
      - "REQ-001"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard, Root: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Errors) == 0 {
		t.Fatalf("expected error for duplicate CUJ IDs")
	}
}

func TestValidateRejectsInvalidCUJPriority(t *testing.T) {
	root := t.TempDir()
	raw := baseSpecYAML()
	raw = []byte(string(raw) + `
critical_user_journeys:
  - id: "CUJ-001"
    title: "Signup"
    priority: "urgent"
    steps:
      - "Open page"
    success_criteria:
      - "Account created"
    linked_requirements:
      - "REQ-001"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard, Root: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Errors) == 0 {
		t.Fatalf("expected error for invalid CUJ priority")
	}
}

func TestValidateAcceptsEvidenceFileInResearchDir(t *testing.T) {
	root := t.TempDir()
	researchDir := filepath.Join(root, ".praude", "research")
	if err := os.MkdirAll(researchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(researchDir, "PRD-001-20260115-000000.md"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := baseSpecYAML()
	raw = []byte(string(raw) + `
market_research:
  - id: "MR-001"
    claim: "Market is growing"
    evidence_refs:
      - path: ".praude/research/PRD-001-20260115-000000.md"
        anchor: "section-1"
        note: "Source quote"
    confidence: "medium"
    date: "2026-01-15"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard, Root: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("expected no errors")
	}
}

func baseSpecYAML() []byte {
	return []byte(`id: "PRD-001"
title: "Example"
summary: "Summary"
requirements:
  - "REQ-001: Requirement one"
`)
}

func containsWarning(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if warning == needle {
			return true
		}
	}
	return false
}
