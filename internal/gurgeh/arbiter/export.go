package arbiter

import (
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// ExportToSpec converts a SprintState back into a specs.Spec.
// This is the inverse of MigrateFromSpec—same markdown conventions.
func ExportToSpec(state *SprintState) (*specs.Spec, error) {
	spec := &specs.Spec{
		ID:     state.ID,
		Status: "draft",
	}

	// Vision → Title
	if s, ok := state.Sections[PhaseVision]; ok && s.Content != "" {
		spec.Title = stripHeading(s.Content)
	}

	// Problem → Summary
	if s, ok := state.Sections[PhaseProblem]; ok && s.Content != "" {
		spec.Summary = stripHeading(s.Content)
	}

	// Users → UserStory
	if s, ok := state.Sections[PhaseUsers]; ok && s.Content != "" {
		spec.UserStory = specs.UserStory{Text: stripHeading(s.Content)}
	}

	// FeaturesGoals → Goals + Requirements (partial)
	if s, ok := state.Sections[PhaseFeaturesGoals]; ok && s.Content != "" {
		spec.Goals = parseGoals(s.Content)
	}

	// Requirements → Requirements
	if s, ok := state.Sections[PhaseRequirements]; ok && s.Content != "" {
		spec.Requirements = parseBulletItems(s.Content)
	}

	// ScopeAssumptions → NonGoals + Assumptions
	if s, ok := state.Sections[PhaseScopeAssumptions]; ok && s.Content != "" {
		spec.NonGoals, spec.Assumptions = parseScopeAssumptions(s.Content)
	}

	// CUJs → CriticalUserJourneys
	if s, ok := state.Sections[PhaseCUJs]; ok && s.Content != "" {
		spec.CriticalUserJourneys = parseCUJs(s.Content)
	}

	// AcceptanceCriteria → Acceptance
	if s, ok := state.Sections[PhaseAcceptanceCriteria]; ok && s.Content != "" {
		spec.Acceptance = parseAcceptanceCriteria(s.Content)
	}

	// Research findings → MarketResearch
	for _, f := range state.Findings {
		spec.MarketResearch = append(spec.MarketResearch, specs.MarketResearchItem{
			ID:         f.ID,
			Claim:      f.Summary,
			Confidence: relevanceToConfidence(f.Relevance),
		})
	}

	return spec, nil
}

// stripHeading removes a leading markdown heading (e.g. "## Vision\n\n") from content.
func stripHeading(content string) string {
	lines := strings.SplitN(content, "\n", 3)
	if len(lines) == 0 {
		return content
	}
	// Skip heading line and optional blank line
	start := 0
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "#") {
		start = 1
	}
	if start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start >= len(lines) {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[start:], "\n"))
}

// parseBulletItems extracts "- item" lines from markdown content.
func parseBulletItems(content string) []string {
	var items []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			items = append(items, strings.TrimPrefix(line, "- "))
		}
	}
	return items
}

// parseGoals extracts Goal structs from features+goals content.
func parseGoals(content string) []specs.Goal {
	var goals []specs.Goal
	inGoals := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(strings.ToLower(trimmed), "**goals**") || strings.HasPrefix(trimmed, "## Goals") {
			inGoals = true
			continue
		}
		if strings.HasPrefix(trimmed, "**") || strings.HasPrefix(trimmed, "## ") {
			inGoals = false
			continue
		}
		if inGoals && strings.HasPrefix(trimmed, "- ") {
			desc := strings.TrimPrefix(trimmed, "- ")
			goals = append(goals, specs.Goal{Description: desc})
		}
	}
	return goals
}

// parseScopeAssumptions splits scope+assumptions content into NonGoals and Assumptions.
func parseScopeAssumptions(content string) ([]specs.NonGoal, []specs.Assumption) {
	var nonGoals []specs.NonGoal
	var assumptions []specs.Assumption

	section := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "scope") || strings.Contains(lower, "non-goal") {
			section = "scope"
			continue
		}
		if strings.Contains(lower, "assumption") {
			section = "assumptions"
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimPrefix(trimmed, "- ")
			switch section {
			case "scope":
				nonGoals = append(nonGoals, specs.NonGoal{Description: item})
			case "assumptions":
				assumptions = append(assumptions, specs.Assumption{Description: item})
			}
		}
	}
	return nonGoals, assumptions
}

// parseCUJs extracts CriticalUserJourney structs from CUJ content.
func parseCUJs(content string) []specs.CriticalUserJourney {
	var cujs []specs.CriticalUserJourney
	var current *specs.CriticalUserJourney

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "**") && strings.Contains(trimmed, "(Priority:") {
			if current != nil {
				cujs = append(cujs, *current)
			}
			title := strings.TrimPrefix(trimmed, "### ")
			title = strings.TrimPrefix(title, "**")
			title = strings.TrimSuffix(title, "**")
			// Extract priority if present
			if idx := strings.Index(title, "(Priority:"); idx != -1 {
				prio := strings.TrimSpace(title[idx+len("(Priority:"):])
				prio = strings.TrimSuffix(prio, ")")
				title = strings.TrimSpace(title[:idx])
				current = &specs.CriticalUserJourney{Title: title, Priority: strings.TrimSpace(prio)}
			} else {
				current = &specs.CriticalUserJourney{Title: title, Priority: "medium"}
			}
			continue
		}
		if current == nil {
			continue
		}
		// Numbered steps
		if len(trimmed) > 2 && trimmed[0] >= '1' && trimmed[0] <= '9' && trimmed[1] == '.' {
			step := strings.TrimSpace(trimmed[2:])
			current.Steps = append(current.Steps, step)
		}
		// Success criteria
		if strings.HasPrefix(trimmed, "- ") {
			current.SuccessCriteria = append(current.SuccessCriteria, strings.TrimPrefix(trimmed, "- "))
		}
	}
	if current != nil {
		cujs = append(cujs, *current)
	}
	return cujs
}

// parseAcceptanceCriteria extracts AcceptanceCriterion structs.
func parseAcceptanceCriteria(content string) []specs.AcceptanceCriterion {
	var criteria []specs.AcceptanceCriterion
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [") {
			// Format: "- [AC-001] description" or "- [ ] description"
			rest := strings.TrimPrefix(trimmed, "- [")
			if idx := strings.Index(rest, "]"); idx != -1 {
				id := rest[:idx]
				desc := strings.TrimSpace(rest[idx+1:])
				if id == " " || id == "x" || id == "X" {
					id = ""
				}
				criteria = append(criteria, specs.AcceptanceCriterion{ID: id, Description: desc})
			}
		}
	}
	return criteria
}

func relevanceToConfidence(relevance float64) string {
	switch {
	case relevance >= 0.8:
		return "high"
	case relevance >= 0.5:
		return "medium"
	default:
		return "low"
	}
}
