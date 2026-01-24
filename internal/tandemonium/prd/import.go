// Package prd provides functionality to import Praude PRDs into Tandemonium epics.
package prd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/epics"
	praudeSpecs "github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

// ImportOptions configures how PRD import behaves.
type ImportOptions struct {
	// Root is the project root directory.
	Root string
	// PRDID is the PRD identifier (e.g., "PRD-001", "mvp", "v1").
	PRDID string
}

// ImportResult contains the result of a PRD import.
type ImportResult struct {
	Epics     []epics.Epic
	SourcePRD string
	Warnings  []string
}

// ImportFromPRD reads a Praude PRD and generates Tandemonium epics.
// It supports both the new PRD format (version-based) and legacy Spec format.
func ImportFromPRD(opts ImportOptions) (*ImportResult, error) {
	specsDir := filepath.Join(opts.Root, ".praude", "specs")

	// First try to load as new PRD format (version-based)
	prds, err := praudeSpecs.LoadAllPRDs(opts.Root)
	if err == nil && len(prds) > 0 {
		for _, prd := range prds {
			if prd.ID == opts.PRDID || prd.Version == opts.PRDID {
				return importFromNewPRD(prd)
			}
		}
	}

	// Fall back to legacy Spec format
	specPath := filepath.Join(specsDir, opts.PRDID+".yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		// Try without extension
		specPath = findSpecFile(specsDir, opts.PRDID)
		if specPath == "" {
			return nil, fmt.Errorf("PRD %q not found in %s", opts.PRDID, specsDir)
		}
	}

	spec, err := praudeSpecs.LoadSpec(specPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load PRD: %w", err)
	}

	return importFromLegacySpec(spec)
}

// importFromNewPRD converts a new-format PRD with Features into epics.
func importFromNewPRD(prd *praudeSpecs.PRD) (*ImportResult, error) {
	result := &ImportResult{
		SourcePRD: prd.ID,
		Epics:     make([]epics.Epic, 0, len(prd.Features)),
	}

	for i, feature := range prd.Features {
		epicID := fmt.Sprintf("EPIC-%03d", i+1)

		// Map feature priority to Tandemonium priority
		priority := mapPriority(feature.Priority)

		// Map feature status
		status := mapFeatureStatus(string(feature.Status))

		// Build acceptance criteria
		criteria := make([]string, 0, len(feature.AcceptanceCriteria))
		for _, ac := range feature.AcceptanceCriteria {
			criteria = append(criteria, ac.Description)
		}

		// Build stories from requirements
		stories := buildStoriesFromRequirements(epicID, feature.Requirements)

		epic := epics.Epic{
			ID:                 epicID,
			Title:              feature.Title,
			Summary:            feature.Summary,
			Status:             status,
			Priority:           priority,
			AcceptanceCriteria: criteria,
			Estimates:          mapComplexityToEstimate(feature.Complexity),
			Stories:            stories,
		}

		result.Epics = append(result.Epics, epic)
	}

	if len(result.Epics) == 0 {
		result.Warnings = append(result.Warnings, "PRD has no features to convert")
	}

	return result, nil
}

// importFromLegacySpec converts a legacy Spec format into epics.
func importFromLegacySpec(spec praudeSpecs.Spec) (*ImportResult, error) {
	result := &ImportResult{
		SourcePRD: spec.ID,
		Epics:     make([]epics.Epic, 0),
	}

	// Create one epic from the spec
	epic := epics.Epic{
		ID:       "EPIC-001",
		Title:    spec.Title,
		Summary:  spec.Summary,
		Status:   mapStatus(spec.Status),
		Priority: mapPriority(spec.Priority),
	}

	// Map acceptance criteria
	for _, ac := range spec.Acceptance {
		epic.AcceptanceCriteria = append(epic.AcceptanceCriteria, ac.Description)
	}

	// Map complexity to estimates
	epic.Estimates = mapComplexityToEstimate(spec.Complexity)

	// Build stories from requirements
	epic.Stories = buildStoriesFromRequirements(epic.ID, spec.Requirements)

	result.Epics = append(result.Epics, epic)
	return result, nil
}

// buildStoriesFromRequirements converts requirements to stories.
func buildStoriesFromRequirements(epicID string, requirements []string) []epics.Story {
	stories := make([]epics.Story, 0, len(requirements))
	for j, req := range requirements {
		storyID := fmt.Sprintf("%s-S%02d", epicID, j+1)
		stories = append(stories, epics.Story{
			ID:       storyID,
			Title:    extractRequirementTitle(req),
			Summary:  req,
			Status:   epics.StatusTodo,
			Priority: epics.PriorityP2,
		})
	}
	return stories
}

// mapPriority maps a numeric priority (0-4) to Tandemonium priority.
func mapPriority(priority int) epics.Priority {
	switch {
	case priority == 0:
		return epics.PriorityP0
	case priority == 1:
		return epics.PriorityP1
	case priority <= 3:
		return epics.PriorityP2
	default:
		return epics.PriorityP3
	}
}

// mapStatus maps a Praude status string to Tandemonium status.
func mapStatus(status string) epics.Status {
	switch status {
	case "draft":
		return epics.StatusTodo
	case "approved", "active":
		return epics.StatusTodo
	case "in_progress":
		return epics.StatusInProgress
	case "review":
		return epics.StatusReview
	case "done", "complete":
		return epics.StatusDone
	case "blocked":
		return epics.StatusBlocked
	default:
		return epics.StatusTodo
	}
}

// mapFeatureStatus maps a feature status to Tandemonium status.
func mapFeatureStatus(status string) epics.Status {
	switch status {
	case "draft":
		return epics.StatusTodo
	case "approved":
		return epics.StatusTodo
	case "in_progress":
		return epics.StatusInProgress
	case "done":
		return epics.StatusDone
	default:
		return epics.StatusTodo
	}
}

// mapComplexityToEstimate converts complexity to t-shirt size estimates.
func mapComplexityToEstimate(complexity string) string {
	switch complexity {
	case "low":
		return "S"
	case "medium":
		return "M"
	case "high":
		return "L"
	default:
		return "M"
	}
}

// extractRequirementTitle extracts a short title from a requirement string.
// Requirements are often in the format "REQ-001: Description"
func extractRequirementTitle(req string) string {
	// If it contains a colon, use the part after it
	for i, c := range req {
		if c == ':' && i < len(req)-1 {
			title := req[i+1:]
			if len(title) > 0 && title[0] == ' ' {
				title = title[1:]
			}
			// Truncate if too long
			if len(title) > 80 {
				return title[:77] + "..."
			}
			return title
		}
	}
	// Otherwise use the whole thing (truncated if needed)
	if len(req) > 80 {
		return req[:77] + "..."
	}
	return req
}

// findSpecFile searches for a spec file by ID in the specs directory.
func findSpecFile(specsDir, prdID string) string {
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Check for exact match without extension
		base := name
		if ext := filepath.Ext(name); ext == ".yaml" || ext == ".yml" {
			base = name[:len(name)-len(ext)]
		}
		if base == prdID {
			return filepath.Join(specsDir, name)
		}
	}
	return ""
}
