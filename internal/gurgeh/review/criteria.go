package review

import (
	"context"
	"regexp"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// AcceptanceCriteriaReviewer validates that acceptance criteria are measurable and testable.
type AcceptanceCriteriaReviewer struct{}

// NewAcceptanceCriteriaReviewer creates a new acceptance criteria reviewer.
func NewAcceptanceCriteriaReviewer() *AcceptanceCriteriaReviewer {
	return &AcceptanceCriteriaReviewer{}
}

// Name returns the reviewer's identifier.
func (r *AcceptanceCriteriaReviewer) Name() string {
	return "acceptance-criteria"
}

// Review validates acceptance criteria quality.
func (r *AcceptanceCriteriaReviewer) Review(ctx context.Context, spec *specs.Spec) (*ReviewResult, error) {
	result := &ReviewResult{
		Reviewer: r.Name(),
		Score:    1.0,
	}

	// Check: Has acceptance criteria
	if len(spec.Acceptance) == 0 {
		// Already caught by completeness reviewer
		result.Suggestions = append(result.Suggestions,
			"Add acceptance criteria to define when the feature is complete")
		return result, nil
	}

	vagueCount := 0
	unmeasurableCount := 0
	duplicateCheck := make(map[string]bool)

	for i, criterion := range spec.Acceptance {
		location := formatACLocation(i)
		desc := criterion.Description

		// Check: Has description
		if strings.TrimSpace(desc) == "" {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityError,
				Category:    "empty-criterion",
				Description: "Acceptance criterion has no description",
				Location:    location,
			})
			result.Score -= 0.15
			continue
		}

		// Check: Not too short
		if len(desc) < 15 {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityWarning,
				Category:    "brief-criterion",
				Description: "Acceptance criterion is too brief to be testable",
				Location:    location,
			})
			result.Score -= 0.05
			vagueCount++
		}

		// Check: Not vague
		if r.isVagueCriterion(desc) {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityWarning,
				Category:    "vague-criterion",
				Description: "Acceptance criterion uses vague language",
				Location:    location,
			})
			result.Score -= 0.1
			vagueCount++
		}

		// Check: Is measurable
		if !r.isMeasurable(desc) {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityInfo,
				Category:    "unmeasurable-criterion",
				Description: "Acceptance criterion may be hard to verify objectively",
				Location:    location,
			})
			result.Score -= 0.05
			unmeasurableCount++
		}

		// Check: Not a duplicate
		normalized := strings.ToLower(strings.TrimSpace(desc))
		if duplicateCheck[normalized] {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityWarning,
				Category:    "duplicate-criterion",
				Description: "Duplicate acceptance criterion detected",
				Location:    location,
			})
			result.Score -= 0.1
		}
		duplicateCheck[normalized] = true

		// Check: Uses testable language
		if !r.usesTestableLanguage(desc) {
			result.Suggestions = append(result.Suggestions,
				"Consider using Given/When/Then format for criterion: "+truncateString(desc, 40))
		}
	}

	// Overall quality checks
	if vagueCount > len(spec.Acceptance)/2 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityWarning,
			Category:    "mostly-vague-criteria",
			Description: "Most acceptance criteria are vague - consider making them more specific",
			Location:    "acceptance_criteria",
		})
		result.Score -= 0.1
	}

	if unmeasurableCount > len(spec.Acceptance)/2 {
		result.Suggestions = append(result.Suggestions,
			"Consider adding measurable outcomes (numbers, states, behaviors) to acceptance criteria")
	}

	// Check coverage against requirements
	if len(spec.Requirements) > 0 && len(spec.Acceptance) < len(spec.Requirements)/2 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityInfo,
			Category:    "low-ac-coverage",
			Description: "Few acceptance criteria relative to requirements - some requirements may not be testable",
			Location:    "acceptance_criteria",
		})
		result.Score -= 0.05
	}

	// Clamp score
	if result.Score < 0 {
		result.Score = 0
	}

	return result, nil
}

// isVagueCriterion checks for vague, non-specific language.
func (r *AcceptanceCriteriaReviewer) isVagueCriterion(desc string) bool {
	vaguePatterns := []string{
		"should work", "should be good", "should be fast",
		"should be easy", "should be nice", "should be intuitive",
		"properly", "correctly", "appropriately", "efficiently",
		"reasonable", "acceptable", "adequate", "satisfactory",
		"as expected", "as needed", "as required", "as appropriate",
		"user-friendly", "performant", "scalable", "maintainable",
		"etc", "and so on", "and more",
	}

	lower := strings.ToLower(desc)
	vagueCount := 0

	for _, pattern := range vaguePatterns {
		if strings.Contains(lower, pattern) {
			vagueCount++
		}
	}

	return vagueCount >= 2 || (len(desc) < 50 && vagueCount >= 1)
}

// isMeasurable checks if the criterion contains measurable outcomes.
func (r *AcceptanceCriteriaReviewer) isMeasurable(desc string) bool {
	lower := strings.ToLower(desc)

	// Look for numbers
	numberPattern := regexp.MustCompile(`\d+`)
	if numberPattern.MatchString(desc) {
		return true
	}

	// Look for specific state transitions
	stateIndicators := []string{
		"returns", "displays", "shows", "contains", "includes",
		"equals", "matches", "is set to", "changes to",
		"enabled", "disabled", "visible", "hidden",
		"success", "failure", "error", "completed",
		"true", "false", "null", "empty", "non-empty",
	}

	for _, indicator := range stateIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}

	// Look for specific user actions
	actionIndicators := []string{
		"when user clicks", "when user submits", "when user enters",
		"after clicking", "after submitting", "upon completion",
		"given", "when", "then",
	}

	for _, indicator := range actionIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}

	return false
}

// usesTestableLanguage checks for structured test language (Given/When/Then, etc.)
func (r *AcceptanceCriteriaReviewer) usesTestableLanguage(desc string) bool {
	lower := strings.ToLower(desc)

	// Gherkin-style
	if strings.Contains(lower, "given") && strings.Contains(lower, "when") {
		return true
	}
	if strings.Contains(lower, "when") && strings.Contains(lower, "then") {
		return true
	}

	// Action-result style
	if strings.Contains(lower, "if") && (strings.Contains(lower, "then") || strings.Contains(lower, "should")) {
		return true
	}

	return false
}

func formatACLocation(index int) string {
	return "acceptance_criteria[" + string(rune('0'+index)) + "]"
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
