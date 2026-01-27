package review

import (
	"context"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// CompletenessReviewer checks that all required PRD sections are filled in.
type CompletenessReviewer struct{}

// NewCompletenessReviewer creates a new completeness reviewer.
func NewCompletenessReviewer() *CompletenessReviewer {
	return &CompletenessReviewer{}
}

// Name returns the reviewer's identifier.
func (r *CompletenessReviewer) Name() string {
	return "completeness"
}

// Review checks that required fields are present and substantive.
func (r *CompletenessReviewer) Review(ctx context.Context, spec *specs.Spec) (*ReviewResult, error) {
	result := &ReviewResult{
		Reviewer: r.Name(),
		Score:    1.0,
	}

	// Required fields
	if strings.TrimSpace(spec.Title) == "" {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityError,
			Category:    "missing-title",
			Description: "PRD has no title",
			Location:    "title",
		})
		result.Score -= 0.2
	}

	if strings.TrimSpace(spec.Summary) == "" {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityError,
			Category:    "missing-summary",
			Description: "PRD has no summary/problem statement",
			Location:    "summary",
		})
		result.Score -= 0.2
	}

	if len(spec.Requirements) == 0 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityError,
			Category:    "missing-requirements",
			Description: "PRD has no requirements",
			Location:    "requirements",
		})
		result.Score -= 0.3
	}

	// Check user story
	if strings.TrimSpace(spec.UserStory.Text) == "" {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityWarning,
			Category:    "missing-user-story",
			Description: "PRD has no user story",
			Location:    "user_story",
		})
		result.Score -= 0.1
	}

	// Check acceptance criteria
	if len(spec.Acceptance) == 0 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityWarning,
			Category:    "missing-acceptance",
			Description: "PRD has no acceptance criteria",
			Location:    "acceptance_criteria",
		})
		result.Score -= 0.1
	}

	// Check CUJs
	if len(spec.CriticalUserJourneys) == 0 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityWarning,
			Category:    "missing-cujs",
			Description: "PRD has no Critical User Journeys defined",
			Location:    "critical_user_journeys",
		})
		result.Score -= 0.1
	}

	// New fields: Goals, Non-Goals, Assumptions
	if len(spec.Goals) == 0 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityInfo,
			Category:    "missing-goals",
			Description: "PRD has no measurable goals - success criteria unclear",
			Location:    "goals",
		})
		result.Score -= 0.05
	}

	if len(spec.NonGoals) == 0 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityInfo,
			Category:    "missing-non-goals",
			Description: "PRD has no non-goals - scope boundaries unclear",
			Location:    "non_goals",
		})
		result.Score -= 0.05
	}

	if len(spec.Assumptions) == 0 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityInfo,
			Category:    "missing-assumptions",
			Description: "PRD has no documented assumptions",
			Location:    "assumptions",
		})
		result.Score -= 0.05
	}

	// Suggestions
	if len(spec.Research) == 0 {
		result.Suggestions = append(result.Suggestions,
			"Consider running Pollard research to inform the PRD")
	}

	if len(spec.MarketResearch) == 0 && len(spec.CompetitiveLandscape) == 0 {
		result.Suggestions = append(result.Suggestions,
			"Consider adding market research or competitive analysis")
	}

	// Clamp score
	if result.Score < 0 {
		result.Score = 0
	}

	return result, nil
}
