package arbiter

import (
	"fmt"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// MigrateFromSpec converts a legacy Spec to SprintState
func MigrateFromSpec(spec *specs.Spec, projectPath string) *SprintState {
	state := NewSprintState(projectPath)

	if spec == nil {
		return state
	}

	// Map Title → Vision section
	if spec.Title != "" {
		state.Sections[PhaseVision].Content = fmt.Sprintf("## Vision\n\n%s", spec.Title)
		state.Sections[PhaseVision].Status = DraftAccepted
	}

	// Map Summary → Problem section
	if spec.Summary != "" {
		state.Sections[PhaseProblem].Content = spec.Summary
		state.Sections[PhaseProblem].Status = DraftAccepted
	}

	// Map UserStory → Users section
	if spec.UserStory.Text != "" {
		state.Sections[PhaseUsers].Content = spec.UserStory.Text
		state.Sections[PhaseUsers].Status = DraftAccepted
	}

	// Map Requirements + Title → Features+Goals section
	featuresGoalsContent := buildFeaturesGoalsContent(spec)
	if featuresGoalsContent != "" {
		state.Sections[PhaseFeaturesGoals].Content = featuresGoalsContent
		state.Sections[PhaseFeaturesGoals].Status = DraftAccepted
	}

	// Map Requirements → Requirements section
	if len(spec.Requirements) > 0 {
		var reqParts []string
		reqParts = append(reqParts, "## Requirements")
		for _, req := range spec.Requirements {
			reqParts = append(reqParts, fmt.Sprintf("- %s", req))
		}
		state.Sections[PhaseRequirements].Content = strings.Join(reqParts, "\n")
		state.Sections[PhaseRequirements].Status = DraftAccepted
	}

	// Map Goals + NonGoals → Scope+Assumptions
	scopeAssumptionsContent := buildScopeAssumptionsContent(spec)
	if scopeAssumptionsContent != "" {
		state.Sections[PhaseScopeAssumptions].Content = scopeAssumptionsContent
		state.Sections[PhaseScopeAssumptions].Status = DraftAccepted
	}

	// Map CriticalUserJourneys → CUJs section
	cujContent := buildCUJContent(spec)
	if cujContent != "" {
		state.Sections[PhaseCUJs].Content = cujContent
		state.Sections[PhaseCUJs].Status = DraftAccepted
	}

	// Map AcceptanceCriteria → Acceptance Criteria section
	acceptanceContent := buildAcceptanceContent(spec)
	if acceptanceContent != "" {
		state.Sections[PhaseAcceptanceCriteria].Content = acceptanceContent
		state.Sections[PhaseAcceptanceCriteria].Status = DraftAccepted
	}

	return state
}

// buildFeaturesGoalsContent constructs the Features + Goals section from Spec fields
func buildFeaturesGoalsContent(spec *specs.Spec) string {
	var parts []string

	if spec.Title != "" {
		parts = append(parts, fmt.Sprintf("**Title**: %s", spec.Title))
	}

	if len(spec.Goals) > 0 {
		parts = append(parts, "**Goals**:")
		for _, goal := range spec.Goals {
			line := fmt.Sprintf("- %s", goal.Description)
			if goal.Metric != "" {
				line += fmt.Sprintf(" (Metric: %s", goal.Metric)
				if goal.Target != "" {
					line += fmt.Sprintf(", Target: %s", goal.Target)
				}
				line += ")"
			}
			parts = append(parts, line)
		}
	}

	if len(spec.Requirements) > 0 {
		parts = append(parts, "**Requirements**:")
		for _, req := range spec.Requirements {
			parts = append(parts, fmt.Sprintf("- %s", req))
		}
	}

	return strings.Join(parts, "\n")
}

// buildScopeAssumptionsContent constructs the Scope + Assumptions section
func buildScopeAssumptionsContent(spec *specs.Spec) string {
	var parts []string

	if len(spec.NonGoals) > 0 {
		parts = append(parts, "**Scope (Non-Goals)**:")
		for _, ng := range spec.NonGoals {
			line := fmt.Sprintf("- %s", ng.Description)
			if ng.Rationale != "" {
				line += fmt.Sprintf(" (Rationale: %s)", ng.Rationale)
			}
			parts = append(parts, line)
		}
	}

	if len(spec.Assumptions) > 0 {
		parts = append(parts, "**Assumptions**:")
		for _, assumption := range spec.Assumptions {
			line := fmt.Sprintf("- %s", assumption.Description)
			if assumption.ImpactIfFalse != "" {
				line += fmt.Sprintf(" (Impact if false: %s)", assumption.ImpactIfFalse)
			}
			if assumption.Confidence != "" {
				line += fmt.Sprintf(" [%s confidence]", assumption.Confidence)
			}
			parts = append(parts, line)
		}
	}

	return strings.Join(parts, "\n")
}

// buildCUJContent constructs the Critical User Journeys section
func buildCUJContent(spec *specs.Spec) string {
	if len(spec.CriticalUserJourneys) == 0 {
		return ""
	}

	var parts []string
	for _, cuj := range spec.CriticalUserJourneys {
		parts = append(parts, fmt.Sprintf("**%s** (Priority: %s)", cuj.Title, cuj.Priority))

		if len(cuj.Steps) > 0 {
			parts = append(parts, "Steps:")
			for i, step := range cuj.Steps {
				parts = append(parts, fmt.Sprintf("%d. %s", i+1, step))
			}
		}

		if len(cuj.SuccessCriteria) > 0 {
			parts = append(parts, "Success Criteria:")
			for _, sc := range cuj.SuccessCriteria {
				parts = append(parts, fmt.Sprintf("- %s", sc))
			}
		}
		parts = append(parts, "")
	}

	return strings.Join(parts, "\n")
}

// buildAcceptanceContent constructs the Acceptance Criteria section
func buildAcceptanceContent(spec *specs.Spec) string {
	if len(spec.Acceptance) == 0 {
		return ""
	}

	var parts []string
	for _, ac := range spec.Acceptance {
		parts = append(parts, fmt.Sprintf("- [%s] %s", ac.ID, ac.Description))
	}

	return strings.Join(parts, "\n")
}
