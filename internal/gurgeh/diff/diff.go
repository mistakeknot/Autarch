// Package diff provides spec change tracking and diffing capabilities.
// This enables tracking what changed between spec versions, detecting scope
// creep, and supporting approval workflows for spec changes.
package diff

import (
	"fmt"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// SpecDiff represents the differences between two spec versions
type SpecDiff struct {
	OldID       string        `json:"old_id"`
	NewID       string        `json:"new_id"`
	Added       []string      `json:"added"`        // New requirements
	Removed     []string      `json:"removed"`      // Dropped requirements
	Changed     []ChangedItem `json:"changed"`      // Modified requirements
	FieldDiffs  []FieldDiff   `json:"field_diffs"`  // Changes to other fields
	ScopeImpact ScopeAssessment `json:"scope_impact"`
}

// ChangedItem represents a modified requirement
type ChangedItem struct {
	ID       string `json:"id,omitempty"`
	OldValue string `json:"old_value"`
	NewValue string `json:"new_value"`
	Type     string `json:"type"` // requirement, acceptance_criteria, cuj_step
}

// FieldDiff represents a change to a spec field
type FieldDiff struct {
	Field    string `json:"field"`
	OldValue string `json:"old_value"`
	NewValue string `json:"new_value"`
}

// ScopeAssessment indicates the impact on scope
type ScopeAssessment struct {
	Direction   string   `json:"direction"`    // larger, smaller, similar
	Confidence  float64  `json:"confidence"`   // 0-1
	AddedCount  int      `json:"added_count"`
	RemovedCount int     `json:"removed_count"`
	ChangedCount int     `json:"changed_count"`
	Warnings    []string `json:"warnings,omitempty"`
}

// DiffSpecs compares two specs and returns the differences
func DiffSpecs(old, new *specs.Spec) *SpecDiff {
	diff := &SpecDiff{
		OldID: old.ID,
		NewID: new.ID,
	}

	// Diff requirements
	diff.Added, diff.Removed, diff.Changed = diffRequirements(old.Requirements, new.Requirements)

	// Diff other fields
	diff.FieldDiffs = diffFields(old, new)

	// Diff acceptance criteria
	acChanges := diffAcceptanceCriteria(old.Acceptance, new.Acceptance)
	diff.Changed = append(diff.Changed, acChanges...)

	// Diff CUJs
	cujChanges := diffCUJs(old.CriticalUserJourneys, new.CriticalUserJourneys)
	diff.Changed = append(diff.Changed, cujChanges...)

	// Assess scope impact
	diff.ScopeImpact = assessScopeImpact(diff)

	return diff
}

// DiffPRDs compares two PRDs and returns the differences
func DiffPRDs(old, new *specs.PRD) *SpecDiff {
	diff := &SpecDiff{
		OldID: old.ID,
		NewID: new.ID,
	}

	// Track feature changes
	oldFeatures := make(map[string]specs.Feature)
	newFeatures := make(map[string]specs.Feature)

	for _, f := range old.Features {
		oldFeatures[f.ID] = f
	}
	for _, f := range new.Features {
		newFeatures[f.ID] = f
	}

	// Added features
	for id := range newFeatures {
		if _, ok := oldFeatures[id]; !ok {
			diff.Added = append(diff.Added, fmt.Sprintf("Feature %s: %s", id, newFeatures[id].Title))
		}
	}

	// Removed features
	for id := range oldFeatures {
		if _, ok := newFeatures[id]; !ok {
			diff.Removed = append(diff.Removed, fmt.Sprintf("Feature %s: %s", id, oldFeatures[id].Title))
		}
	}

	// Changed features
	for id, oldF := range oldFeatures {
		if newF, ok := newFeatures[id]; ok {
			if oldF.Title != newF.Title {
				diff.Changed = append(diff.Changed, ChangedItem{
					ID:       id,
					OldValue: oldF.Title,
					NewValue: newF.Title,
					Type:     "feature_title",
				})
			}
			if oldF.Status != newF.Status {
				diff.Changed = append(diff.Changed, ChangedItem{
					ID:       id,
					OldValue: string(oldF.Status),
					NewValue: string(newF.Status),
					Type:     "feature_status",
				})
			}
			// Diff requirements within feature
			added, removed, changed := diffRequirements(oldF.Requirements, newF.Requirements)
			for _, r := range added {
				diff.Added = append(diff.Added, fmt.Sprintf("[%s] %s", id, r))
			}
			for _, r := range removed {
				diff.Removed = append(diff.Removed, fmt.Sprintf("[%s] %s", id, r))
			}
			diff.Changed = append(diff.Changed, changed...)
		}
	}

	// PRD-level field diffs
	if old.Title != new.Title {
		diff.FieldDiffs = append(diff.FieldDiffs, FieldDiff{
			Field:    "title",
			OldValue: old.Title,
			NewValue: new.Title,
		})
	}
	if old.Status != new.Status {
		diff.FieldDiffs = append(diff.FieldDiffs, FieldDiff{
			Field:    "status",
			OldValue: string(old.Status),
			NewValue: string(new.Status),
		})
	}

	diff.ScopeImpact = assessScopeImpact(diff)
	return diff
}

// HasChanges returns true if there are any differences
func (d *SpecDiff) HasChanges() bool {
	return len(d.Added) > 0 || len(d.Removed) > 0 || len(d.Changed) > 0 || len(d.FieldDiffs) > 0
}

// IsScopeCreep returns true if scope has grown significantly
func (d *SpecDiff) IsScopeCreep() bool {
	return d.ScopeImpact.Direction == "larger" && d.ScopeImpact.Confidence > 0.7
}

// Summary returns a brief summary of changes
func (d *SpecDiff) Summary() string {
	if !d.HasChanges() {
		return "No changes"
	}

	var parts []string
	if len(d.Added) > 0 {
		parts = append(parts, fmt.Sprintf("+%d added", len(d.Added)))
	}
	if len(d.Removed) > 0 {
		parts = append(parts, fmt.Sprintf("-%d removed", len(d.Removed)))
	}
	if len(d.Changed) > 0 {
		parts = append(parts, fmt.Sprintf("~%d modified", len(d.Changed)))
	}

	summary := strings.Join(parts, ", ")
	if d.IsScopeCreep() {
		summary += " ⚠️ SCOPE CREEP DETECTED"
	}
	return summary
}

// FormatDiff formats the diff as markdown
func FormatDiff(d *SpecDiff) string {
	if !d.HasChanges() {
		return "No changes detected."
	}

	var sb strings.Builder

	sb.WriteString("# Spec Changes\n\n")
	sb.WriteString(fmt.Sprintf("**Summary:** %s\n\n", d.Summary()))

	// Scope assessment
	sb.WriteString("## Scope Impact\n")
	sb.WriteString(fmt.Sprintf("- **Direction:** %s\n", d.ScopeImpact.Direction))
	sb.WriteString(fmt.Sprintf("- **Confidence:** %.0f%%\n", d.ScopeImpact.Confidence*100))
	if len(d.ScopeImpact.Warnings) > 0 {
		sb.WriteString("- **Warnings:**\n")
		for _, w := range d.ScopeImpact.Warnings {
			sb.WriteString(fmt.Sprintf("  - ⚠️ %s\n", w))
		}
	}
	sb.WriteString("\n")

	// Added items
	if len(d.Added) > 0 {
		sb.WriteString("## Added\n")
		for _, item := range d.Added {
			sb.WriteString(fmt.Sprintf("+ %s\n", item))
		}
		sb.WriteString("\n")
	}

	// Removed items
	if len(d.Removed) > 0 {
		sb.WriteString("## Removed\n")
		for _, item := range d.Removed {
			sb.WriteString(fmt.Sprintf("- %s\n", item))
		}
		sb.WriteString("\n")
	}

	// Changed items
	if len(d.Changed) > 0 {
		sb.WriteString("## Modified\n")
		for _, item := range d.Changed {
			sb.WriteString(fmt.Sprintf("### %s", item.Type))
			if item.ID != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", item.ID))
			}
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf("- **Before:** %s\n", truncate(item.OldValue, 100)))
			sb.WriteString(fmt.Sprintf("- **After:** %s\n", truncate(item.NewValue, 100)))
			sb.WriteString("\n")
		}
	}

	// Field diffs
	if len(d.FieldDiffs) > 0 {
		sb.WriteString("## Field Changes\n")
		for _, fd := range d.FieldDiffs {
			sb.WriteString(fmt.Sprintf("- **%s:** `%s` → `%s`\n", fd.Field, fd.OldValue, fd.NewValue))
		}
	}

	return sb.String()
}

// --- Helper functions ---

func diffRequirements(old, new []string) (added, removed []string, changed []ChangedItem) {
	oldSet := make(map[string]bool)
	newSet := make(map[string]bool)

	for _, r := range old {
		oldSet[normalizeReq(r)] = true
	}
	for _, r := range new {
		newSet[normalizeReq(r)] = true
	}

	// Find added
	for _, r := range new {
		if !oldSet[normalizeReq(r)] {
			added = append(added, r)
		}
	}

	// Find removed
	for _, r := range old {
		if !newSet[normalizeReq(r)] {
			removed = append(removed, r)
		}
	}

	return
}

func diffFields(old, new *specs.Spec) []FieldDiff {
	var diffs []FieldDiff

	if old.Title != new.Title {
		diffs = append(diffs, FieldDiff{Field: "title", OldValue: old.Title, NewValue: new.Title})
	}
	if old.Summary != new.Summary {
		diffs = append(diffs, FieldDiff{Field: "summary", OldValue: old.Summary, NewValue: new.Summary})
	}
	if old.Status != new.Status {
		diffs = append(diffs, FieldDiff{Field: "status", OldValue: old.Status, NewValue: new.Status})
	}
	if old.Complexity != new.Complexity {
		diffs = append(diffs, FieldDiff{Field: "complexity", OldValue: old.Complexity, NewValue: new.Complexity})
	}
	if old.Priority != new.Priority {
		diffs = append(diffs, FieldDiff{
			Field:    "priority",
			OldValue: fmt.Sprintf("%d", old.Priority),
			NewValue: fmt.Sprintf("%d", new.Priority),
		})
	}

	return diffs
}

func diffAcceptanceCriteria(old, new []specs.AcceptanceCriterion) []ChangedItem {
	var changes []ChangedItem

	oldMap := make(map[string]specs.AcceptanceCriterion)
	for _, ac := range old {
		oldMap[ac.ID] = ac
	}

	for _, newAC := range new {
		if oldAC, ok := oldMap[newAC.ID]; ok {
			if oldAC.Description != newAC.Description {
				changes = append(changes, ChangedItem{
					ID:       newAC.ID,
					OldValue: oldAC.Description,
					NewValue: newAC.Description,
					Type:     "acceptance_criteria",
				})
			}
		}
	}

	return changes
}

func diffCUJs(old, new []specs.CriticalUserJourney) []ChangedItem {
	var changes []ChangedItem

	oldMap := make(map[string]specs.CriticalUserJourney)
	for _, cuj := range old {
		oldMap[cuj.ID] = cuj
	}

	for _, newCUJ := range new {
		if oldCUJ, ok := oldMap[newCUJ.ID]; ok {
			if oldCUJ.Title != newCUJ.Title {
				changes = append(changes, ChangedItem{
					ID:       newCUJ.ID,
					OldValue: oldCUJ.Title,
					NewValue: newCUJ.Title,
					Type:     "cuj_title",
				})
			}
			if oldCUJ.Priority != newCUJ.Priority {
				changes = append(changes, ChangedItem{
					ID:       newCUJ.ID,
					OldValue: oldCUJ.Priority,
					NewValue: newCUJ.Priority,
					Type:     "cuj_priority",
				})
			}
		}
	}

	return changes
}

func assessScopeImpact(diff *SpecDiff) ScopeAssessment {
	assessment := ScopeAssessment{
		AddedCount:   len(diff.Added),
		RemovedCount: len(diff.Removed),
		ChangedCount: len(diff.Changed),
	}

	// Calculate net change
	netChange := assessment.AddedCount - assessment.RemovedCount

	if netChange > 2 {
		assessment.Direction = "larger"
		assessment.Confidence = min(1.0, float64(netChange)/5.0)
		if netChange > 5 {
			assessment.Warnings = append(assessment.Warnings, "Significant scope increase detected")
		}
	} else if netChange < -2 {
		assessment.Direction = "smaller"
		assessment.Confidence = min(1.0, float64(-netChange)/5.0)
	} else {
		assessment.Direction = "similar"
		assessment.Confidence = 0.8
	}

	// Warn about large number of changes
	if assessment.ChangedCount > 5 {
		assessment.Warnings = append(assessment.Warnings, "Many items modified - review carefully")
	}

	return assessment
}

func normalizeReq(r string) string {
	return strings.TrimSpace(strings.ToLower(r))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
