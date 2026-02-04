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

func TestValidateAcceptanceCriteriaDuplicateID(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
acceptance_criteria:
  - id: "AC-001"
    description: "First"
  - id: "AC-001"
    description: "Duplicate"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "duplicate acceptance criterion id: AC-001") {
		t.Fatalf("expected duplicate AC error, got %v", res.Errors)
	}
}

func TestValidateAcceptanceCriteriaMissingID(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
acceptance_criteria:
  - description: "No ID"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "acceptance criterion id is required") {
		t.Fatalf("expected missing AC id error, got %v", res.Errors)
	}
}

func TestValidateFilesToModifyInvalidAction(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
files_to_modify:
  - action: "rename"
    path: "foo.go"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "invalid file action: rename") {
		t.Fatalf("expected invalid action error, got %v", res.Errors)
	}
}

func TestValidateFilesToModifyEmptyPath(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
files_to_modify:
  - action: "create"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "file change path is required") {
		t.Fatalf("expected missing path error, got %v", res.Errors)
	}
}

func TestValidateFilesToModifyValid(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
files_to_modify:
  - action: "create"
    path: "foo.go"
  - action: "modify"
    path: "bar.go"
  - action: "delete"
    path: "baz.go"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Errors {
		if e == "invalid file action" || e == "file change path is required" {
			t.Fatalf("unexpected error: %s", e)
		}
	}
}

func TestValidateComplexityInvalid(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `complexity: "extreme"`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "invalid complexity: extreme") {
		t.Fatalf("expected invalid complexity error, got %v", res.Errors)
	}
}

func TestValidateComplexityValid(t *testing.T) {
	for _, c := range []string{"trivial", "low", "medium", "high", "critical"} {
		raw := []byte(string(baseSpecYAML()) + `complexity: "` + c + `"`)
		res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
		if err != nil {
			t.Fatal(err)
		}
		if containsError(res.Errors, "invalid complexity") {
			t.Fatalf("unexpected complexity error for %s", c)
		}
	}
}

func TestValidatePriorityOutOfRange(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `priority: 5`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "priority out of range 0-4") {
		t.Fatalf("expected priority error, got %v", res.Errors)
	}
}

func TestValidatePriorityOutOfRangeSoft(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `priority: 5`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationSoft})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("expected no errors in soft mode, got %v", res.Errors)
	}
	if !containsWarning(res.Warnings, "priority out of range 0-4") {
		t.Fatalf("expected priority warning, got %v", res.Warnings)
	}
}

func TestValidateGoalsDuplicateID(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
goals:
  - id: "GOAL-001"
    description: "First"
  - id: "GOAL-001"
    description: "Dup"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "duplicate goal id: GOAL-001") {
		t.Fatalf("expected duplicate goal error, got %v", res.Errors)
	}
}

func TestValidateGoalsMissingDescriptionSoft(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
goals:
  - id: "GOAL-001"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationSoft})
	if err != nil {
		t.Fatal(err)
	}
	if !containsWarning(res.Warnings, "goal missing description: GOAL-001") {
		t.Fatalf("expected description warning, got %v", res.Warnings)
	}
}

func TestValidateGoalsMissingDescriptionHard(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
goals:
  - id: "GOAL-001"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "goal missing description: GOAL-001") {
		t.Fatalf("expected description error in hard mode, got %v", res.Errors)
	}
}

func TestValidateNonGoalsDuplicateID(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
non_goals:
  - id: "NG-001"
    description: "First"
  - id: "NG-001"
    description: "Dup"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "duplicate non-goal id: NG-001") {
		t.Fatalf("expected duplicate non-goal error, got %v", res.Errors)
	}
}

func TestValidateStructuredRequirementsInvalidType(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
structured_requirements:
  - id: "REQ-S01"
    type: "usability"
    given: "a user"
    when: "action"
    then: "result"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "invalid requirement type: usability") {
		t.Fatalf("expected type error, got %v", res.Errors)
	}
}

func TestValidateStructuredRequirementsEmptyGWT(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
structured_requirements:
  - id: "REQ-S01"
    type: "functional"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "structured requirement has empty GWT: REQ-S01") {
		t.Fatalf("expected GWT error, got %v", res.Errors)
	}
}

func TestValidateStructuredRequirementsInvalidStatus(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
structured_requirements:
  - id: "REQ-S01"
    type: "functional"
    given: "a user"
    when: "action"
    then: "result"
    status: "rejected"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "invalid requirement status: rejected") {
		t.Fatalf("expected status error, got %v", res.Errors)
	}
}

func TestValidateStructuredRequirementsValid(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
structured_requirements:
  - id: "REQ-S01"
    type: "functional"
    given: "a user"
    when: "action"
    then: "result"
    status: "draft"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Errors {
		if e == "invalid requirement type" || e == "structured requirement has empty GWT" {
			t.Fatalf("unexpected error: %s", e)
		}
	}
}

func TestValidateHypothesesInvalidStatus(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
hypotheses:
  - id: "HYP-001"
    statement: "If X then Y"
    status: "pending"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "invalid hypothesis status: pending") {
		t.Fatalf("expected status error, got %v", res.Errors)
	}
}

func TestValidateHypothesesMissingStatementSoft(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
hypotheses:
  - id: "HYP-001"
    status: "untested"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationSoft})
	if err != nil {
		t.Fatal(err)
	}
	if !containsWarning(res.Warnings, "hypothesis missing statement: HYP-001") {
		t.Fatalf("expected statement warning, got %v", res.Warnings)
	}
}

func TestValidateHypothesesDuplicateID(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
hypotheses:
  - id: "HYP-001"
    statement: "First"
    status: "untested"
  - id: "HYP-001"
    statement: "Dup"
    status: "untested"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(res.Errors, "duplicate hypothesis id: HYP-001") {
		t.Fatalf("expected duplicate error, got %v", res.Errors)
	}
}

func TestValidateUserStoryHashMissing(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
user_story:
  text: "As a user I want..."
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationSoft})
	if err != nil {
		t.Fatal(err)
	}
	if !containsWarning(res.Warnings, "user story has text but no hash") {
		t.Fatalf("expected hash warning, got %v", res.Warnings)
	}
}

func TestValidateUserStoryHashPresent(t *testing.T) {
	raw := []byte(string(baseSpecYAML()) + `
user_story:
  text: "As a user I want..."
  hash: "abc123"
`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard})
	if err != nil {
		t.Fatal(err)
	}
	if containsError(res.Errors, "user story has text but no hash") {
		t.Fatal("unexpected hash error when hash present")
	}
}

func TestValidateVisionRefNotFound(t *testing.T) {
	root := t.TempDir()
	raw := []byte(string(baseSpecYAML()) + `vision_ref: "VISION-001"`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if !containsWarning(res.Warnings, "vision_ref not found: VISION-001") {
		t.Fatalf("expected vision_ref warning, got warnings=%v errors=%v", res.Warnings, res.Errors)
	}
}

func TestValidateVisionRefFound(t *testing.T) {
	root := t.TempDir()
	specsDir := filepath.Join(root, ".gurgeh", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specsDir, "VISION-001.yaml"), []byte("id: VISION-001"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := []byte(string(baseSpecYAML()) + `vision_ref: "VISION-001"`)
	res, err := Validate(raw, ValidationOptions{Mode: ValidationHard, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if containsWarning(res.Warnings, "vision_ref not found: VISION-001") {
		t.Fatal("unexpected vision_ref warning when file exists")
	}
}

func containsError(errors []string, needle string) bool {
	for _, e := range errors {
		if e == needle {
			return true
		}
	}
	return false
}

func containsWarning(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if warning == needle {
			return true
		}
	}
	return false
}
