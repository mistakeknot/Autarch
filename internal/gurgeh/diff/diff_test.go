package diff

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestDiffSpecs_AddedRequirements(t *testing.T) {
	old := &specs.Spec{
		ID:           "SPEC-001",
		Requirements: []string{"REQ-001"},
	}
	new := &specs.Spec{
		ID:           "SPEC-001",
		Requirements: []string{"REQ-001", "REQ-002", "REQ-003"},
	}

	diff := DiffSpecs(old, new)

	if len(diff.Added) != 2 {
		t.Errorf("Added = %d, want 2", len(diff.Added))
	}
}

func TestDiffSpecs_RemovedRequirements(t *testing.T) {
	old := &specs.Spec{
		ID:           "SPEC-001",
		Requirements: []string{"REQ-001", "REQ-002", "REQ-003"},
	}
	new := &specs.Spec{
		ID:           "SPEC-001",
		Requirements: []string{"REQ-001"},
	}

	diff := DiffSpecs(old, new)

	if len(diff.Removed) != 2 {
		t.Errorf("Removed = %d, want 2", len(diff.Removed))
	}
}

func TestDiffSpecs_FieldChanges(t *testing.T) {
	old := &specs.Spec{
		ID:      "SPEC-001",
		Title:   "Old Title",
		Summary: "Old Summary",
		Status:  "draft",
	}
	new := &specs.Spec{
		ID:      "SPEC-001",
		Title:   "New Title",
		Summary: "New Summary",
		Status:  "approved",
	}

	diff := DiffSpecs(old, new)

	if len(diff.FieldDiffs) != 3 {
		t.Errorf("FieldDiffs = %d, want 3", len(diff.FieldDiffs))
	}

	// Check specific field
	foundTitle := false
	for _, fd := range diff.FieldDiffs {
		if fd.Field == "title" {
			foundTitle = true
			if fd.OldValue != "Old Title" || fd.NewValue != "New Title" {
				t.Errorf("title diff incorrect: %v", fd)
			}
		}
	}
	if !foundTitle {
		t.Error("expected title field diff")
	}
}

func TestDiffSpecs_NoChanges(t *testing.T) {
	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test",
		Requirements: []string{"REQ-001"},
	}

	diff := DiffSpecs(spec, spec)

	if diff.HasChanges() {
		t.Error("expected no changes for identical specs")
	}
}

func TestDiffSpecs_AcceptanceCriteria(t *testing.T) {
	old := &specs.Spec{
		ID: "SPEC-001",
		Acceptance: []specs.AcceptanceCriterion{
			{ID: "AC-001", Description: "Old description"},
		},
	}
	new := &specs.Spec{
		ID: "SPEC-001",
		Acceptance: []specs.AcceptanceCriterion{
			{ID: "AC-001", Description: "New description"},
		},
	}

	diff := DiffSpecs(old, new)

	found := false
	for _, c := range diff.Changed {
		if c.Type == "acceptance_criteria" && c.ID == "AC-001" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected acceptance criteria change")
	}
}

func TestAssessScopeImpact_Larger(t *testing.T) {
	diff := &SpecDiff{
		Added:   []string{"1", "2", "3", "4", "5", "6"}, // 6 items triggers warning
		Removed: []string{},
	}

	assessment := assessScopeImpact(diff)

	if assessment.Direction != "larger" {
		t.Errorf("Direction = %s, want larger", assessment.Direction)
	}
	if len(assessment.Warnings) == 0 {
		t.Error("expected warning for significant scope increase")
	}
}

func TestAssessScopeImpact_Smaller(t *testing.T) {
	diff := &SpecDiff{
		Added:   []string{},
		Removed: []string{"1", "2", "3", "4"},
	}

	assessment := assessScopeImpact(diff)

	if assessment.Direction != "smaller" {
		t.Errorf("Direction = %s, want smaller", assessment.Direction)
	}
}

func TestAssessScopeImpact_Similar(t *testing.T) {
	diff := &SpecDiff{
		Added:   []string{"1"},
		Removed: []string{"2"},
	}

	assessment := assessScopeImpact(diff)

	if assessment.Direction != "similar" {
		t.Errorf("Direction = %s, want similar", assessment.Direction)
	}
}

func TestIsScopeCreep(t *testing.T) {
	tests := []struct {
		name     string
		diff     *SpecDiff
		expected bool
	}{
		{
			name: "scope creep",
			diff: &SpecDiff{
				Added:   []string{"1", "2", "3", "4", "5", "6"},
				Removed: []string{},
			},
			expected: true,
		},
		{
			name: "no creep - balanced",
			diff: &SpecDiff{
				Added:   []string{"1", "2"},
				Removed: []string{"3", "4"},
			},
			expected: false,
		},
		{
			name: "no creep - shrinking",
			diff: &SpecDiff{
				Added:   []string{},
				Removed: []string{"1", "2", "3"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.diff.ScopeImpact = assessScopeImpact(tt.diff)
			if tt.diff.IsScopeCreep() != tt.expected {
				t.Errorf("IsScopeCreep() = %v, want %v", tt.diff.IsScopeCreep(), tt.expected)
			}
		})
	}
}

func TestSummary(t *testing.T) {
	diff := &SpecDiff{
		Added:   []string{"1", "2"},
		Removed: []string{"3"},
		Changed: []ChangedItem{{Type: "test"}},
	}

	summary := diff.Summary()

	if !strings.Contains(summary, "+2 added") {
		t.Error("summary should contain added count")
	}
	if !strings.Contains(summary, "-1 removed") {
		t.Error("summary should contain removed count")
	}
	if !strings.Contains(summary, "~1 modified") {
		t.Error("summary should contain modified count")
	}
}

func TestFormatDiff(t *testing.T) {
	diff := &SpecDiff{
		OldID:   "v1",
		NewID:   "v2",
		Added:   []string{"New requirement"},
		Removed: []string{"Old requirement"},
		Changed: []ChangedItem{
			{ID: "REQ-001", OldValue: "old", NewValue: "new", Type: "requirement"},
		},
		FieldDiffs: []FieldDiff{
			{Field: "title", OldValue: "Old Title", NewValue: "New Title"},
		},
		ScopeImpact: ScopeAssessment{
			Direction:  "similar",
			Confidence: 0.8,
		},
	}

	output := FormatDiff(diff)

	if !strings.Contains(output, "Spec Changes") {
		t.Error("output should contain header")
	}
	if !strings.Contains(output, "New requirement") {
		t.Error("output should contain added items")
	}
	if !strings.Contains(output, "Old requirement") {
		t.Error("output should contain removed items")
	}
	if !strings.Contains(output, "Field Changes") {
		t.Error("output should contain field changes")
	}
}

func TestFormatDiff_NoChanges(t *testing.T) {
	diff := &SpecDiff{}

	output := FormatDiff(diff)

	if output != "No changes detected." {
		t.Errorf("expected 'No changes detected.', got: %s", output)
	}
}

func TestDiffPRDs(t *testing.T) {
	old := &specs.PRD{
		ID:    "MVP",
		Title: "MVP",
		Features: []specs.Feature{
			{ID: "FEAT-001", Title: "Feature 1", Requirements: []string{"REQ-001"}},
		},
	}
	new := &specs.PRD{
		ID:    "MVP",
		Title: "MVP Updated",
		Features: []specs.Feature{
			{ID: "FEAT-001", Title: "Feature 1 Updated", Requirements: []string{"REQ-001", "REQ-002"}},
			{ID: "FEAT-002", Title: "Feature 2"},
		},
	}

	diff := DiffPRDs(old, new)

	// Should detect new feature
	foundNewFeature := false
	for _, a := range diff.Added {
		if strings.Contains(a, "FEAT-002") {
			foundNewFeature = true
			break
		}
	}
	if !foundNewFeature {
		t.Error("expected to detect new feature")
	}

	// Should detect title change
	foundTitleDiff := false
	for _, fd := range diff.FieldDiffs {
		if fd.Field == "title" {
			foundTitleDiff = true
			break
		}
	}
	if !foundTitleDiff {
		t.Error("expected to detect title change")
	}

	// Should detect feature title change
	foundFeatureTitleChange := false
	for _, c := range diff.Changed {
		if c.Type == "feature_title" && c.ID == "FEAT-001" {
			foundFeatureTitleChange = true
			break
		}
	}
	if !foundFeatureTitleChange {
		t.Error("expected to detect feature title change")
	}
}
