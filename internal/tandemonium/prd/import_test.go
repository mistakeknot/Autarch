package prd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportFromPRD_LegacySpec(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	specsDir := filepath.Join(tmpDir, ".praude", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a test PRD file
	prdContent := `id: "PRD-001"
title: "Test PRD"
status: "draft"
summary: "This is a test PRD"
requirements:
  - "REQ-001: First requirement"
  - "REQ-002: Second requirement"
acceptance_criteria:
  - id: "AC-1"
    description: "First criterion"
  - id: "AC-2"
    description: "Second criterion"
complexity: "medium"
priority: 1
`
	if err := os.WriteFile(filepath.Join(specsDir, "PRD-001.yaml"), []byte(prdContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test import
	result, err := ImportFromPRD(ImportOptions{
		Root:  tmpDir,
		PRDID: "PRD-001",
	})
	if err != nil {
		t.Fatalf("ImportFromPRD failed: %v", err)
	}

	// Verify result
	if len(result.Epics) != 1 {
		t.Fatalf("Expected 1 epic, got %d", len(result.Epics))
	}

	epic := result.Epics[0]
	if epic.ID != "EPIC-001" {
		t.Errorf("Expected epic ID EPIC-001, got %s", epic.ID)
	}
	if epic.Title != "Test PRD" {
		t.Errorf("Expected epic title 'Test PRD', got %s", epic.Title)
	}
	if len(epic.Stories) != 2 {
		t.Errorf("Expected 2 stories, got %d", len(epic.Stories))
	}
	if len(epic.AcceptanceCriteria) != 2 {
		t.Errorf("Expected 2 acceptance criteria, got %d", len(epic.AcceptanceCriteria))
	}
	if epic.Estimates != "M" {
		t.Errorf("Expected estimates 'M' (from medium complexity), got %s", epic.Estimates)
	}
	if epic.Priority != "p1" {
		t.Errorf("Expected priority 'p1', got %s", epic.Priority)
	}
}

func TestImportFromPRD_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	specsDir := filepath.Join(tmpDir, ".praude", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := ImportFromPRD(ImportOptions{
		Root:  tmpDir,
		PRDID: "NONEXISTENT",
	})
	if err == nil {
		t.Error("Expected error for nonexistent PRD, got nil")
	}
}

func TestMapPriority(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "p0"},
		{1, "p1"},
		{2, "p2"},
		{3, "p2"},
		{4, "p3"},
		{10, "p3"},
	}

	for _, tc := range tests {
		result := mapPriority(tc.input)
		if string(result) != tc.expected {
			t.Errorf("mapPriority(%d) = %s, expected %s", tc.input, result, tc.expected)
		}
	}
}

func TestMapComplexityToEstimate(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"low", "S"},
		{"medium", "M"},
		{"high", "L"},
		{"unknown", "M"},
		{"", "M"},
	}

	for _, tc := range tests {
		result := mapComplexityToEstimate(tc.input)
		if result != tc.expected {
			t.Errorf("mapComplexityToEstimate(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestExtractRequirementTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"REQ-001: First requirement", "First requirement"},
		{"REQ-002:Second requirement", "Second requirement"},
		{"Just a requirement without ID", "Just a requirement without ID"},
		{"Very long requirement that should be truncated because it exceeds eighty characters in total length and needs to fit in a title field", "Very long requirement that should be truncated because it exceeds eighty char..."},
	}

	for _, tc := range tests {
		result := extractRequirementTitle(tc.input)
		if result != tc.expected {
			t.Errorf("extractRequirementTitle(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}
