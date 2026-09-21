package specs

import (
	"fmt"
	"strings"
)

// SpecDiff represents the differences between two spec versions.
type SpecDiff struct {
	SpecID      string
	FromVersion int
	ToVersion   int
	Changes     []DiffEntry
}

// DiffEntry is a single field-level difference.
type DiffEntry struct {
	Field  string
	Before string
	After  string
}

// DiffSpecs compares two specs and returns structured differences.
func DiffSpecs(a, b *Spec) []DiffEntry {
	var entries []DiffEntry

	if a.Title != b.Title {
		entries = append(entries, DiffEntry{"title", a.Title, b.Title})
	}
	if a.Summary != b.Summary {
		entries = append(entries, DiffEntry{"summary", a.Summary, b.Summary})
	}
	if a.Status != b.Status {
		entries = append(entries, DiffEntry{"status", a.Status, b.Status})
	}
	if a.Complexity != b.Complexity {
		entries = append(entries, DiffEntry{"complexity", a.Complexity, b.Complexity})
	}

	// Goals
	if len(a.Goals) != len(b.Goals) {
		entries = append(entries, DiffEntry{"goals_count", itoa(len(a.Goals)), itoa(len(b.Goals))})
	}

	// Assumptions
	if len(a.Assumptions) != len(b.Assumptions) {
		entries = append(entries, DiffEntry{"assumptions_count", itoa(len(a.Assumptions)), itoa(len(b.Assumptions))})
	}
	// Check confidence changes
	aMap := make(map[string]string)
	for _, as := range a.Assumptions {
		aMap[as.ID] = as.Confidence
	}
	for _, bs := range b.Assumptions {
		if ac, ok := aMap[bs.ID]; ok && ac != bs.Confidence {
			entries = append(entries, DiffEntry{
				fmt.Sprintf("assumption[%s].confidence", bs.ID),
				ac, bs.Confidence,
			})
		}
	}

	// Hypotheses
	if len(a.Hypotheses) != len(b.Hypotheses) {
		entries = append(entries, DiffEntry{"hypotheses_count", itoa(len(a.Hypotheses)), itoa(len(b.Hypotheses))})
	}

	// Structured Requirements
	if len(a.StructuredRequirements) != len(b.StructuredRequirements) {
		entries = append(entries, DiffEntry{"structured_requirements_count", itoa(len(a.StructuredRequirements)), itoa(len(b.StructuredRequirements))})
	}

	// Requirements (legacy string list)
	if strings.Join(a.Requirements, "\n") != strings.Join(b.Requirements, "\n") {
		entries = append(entries, DiffEntry{"requirements", strings.Join(a.Requirements, "; "), strings.Join(b.Requirements, "; ")})
	}

	return entries
}

// FormatDiff returns a human-readable diff summary.
func FormatDiff(diff []DiffEntry) string {
	if len(diff) == 0 {
		return "No changes"
	}
	var lines []string
	for _, d := range diff {
		lines = append(lines, fmt.Sprintf("  %s: %q → %q", d.Field, d.Before, d.After))
	}
	return strings.Join(lines, "\n")
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
