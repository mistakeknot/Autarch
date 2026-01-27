package review

import (
	"context"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// CUJConsistencyReviewer validates Critical User Journeys for completeness and consistency.
type CUJConsistencyReviewer struct{}

// NewCUJConsistencyReviewer creates a new CUJ consistency reviewer.
func NewCUJConsistencyReviewer() *CUJConsistencyReviewer {
	return &CUJConsistencyReviewer{}
}

// Name returns the reviewer's identifier.
func (r *CUJConsistencyReviewer) Name() string {
	return "cuj-consistency"
}

// Review validates CUJ structure and cross-references.
func (r *CUJConsistencyReviewer) Review(ctx context.Context, spec *specs.Spec) (*ReviewResult, error) {
	result := &ReviewResult{
		Reviewer: r.Name(),
		Score:    1.0,
	}

	// Check: Has CUJs
	if len(spec.CriticalUserJourneys) == 0 {
		// Already caught by completeness reviewer, just add suggestion
		result.Suggestions = append(result.Suggestions,
			"Add Critical User Journeys to define key user paths through the feature")
		return result, nil
	}

	// Build requirement ID set for cross-reference validation
	reqIDs := make(map[string]bool)
	for i, req := range spec.Requirements {
		// Requirements are strings, generate implicit IDs
		reqIDs[formatReqID(i)] = true
		// Also allow matching by content substring
		reqIDs[strings.ToLower(req)] = true
	}

	for i, cuj := range spec.CriticalUserJourneys {
		location := formatCUJLocation(i)

		// Check: CUJ has title
		if strings.TrimSpace(cuj.Title) == "" {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityWarning,
				Category:    "missing-cuj-title",
				Description: "CUJ has no title",
				Location:    location,
			})
			result.Score -= 0.1
		}

		// Check: CUJ has steps
		if len(cuj.Steps) == 0 {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityError,
				Category:    "missing-cuj-steps",
				Description: "CUJ has no steps defined",
				Location:    location,
			})
			result.Score -= 0.15
		} else {
			// Check for vague steps
			vagueSteps := r.countVagueSteps(cuj.Steps)
			if vagueSteps > len(cuj.Steps)/2 {
				result.Issues = append(result.Issues, Issue{
					Severity:    SeverityWarning,
					Category:    "vague-cuj-steps",
					Description: "Most CUJ steps are too vague to be actionable",
					Location:    location,
				})
				result.Score -= 0.1
			}
		}

		// Check: CUJ has success criteria
		if len(cuj.SuccessCriteria) == 0 {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityWarning,
				Category:    "missing-success-criteria",
				Description: "CUJ has no success criteria",
				Location:    location,
			})
			result.Score -= 0.1
		}

		// Check: Linked requirements exist
		for _, linkedReq := range cuj.LinkedRequirements {
			if !r.requirementExists(linkedReq, reqIDs) {
				result.Issues = append(result.Issues, Issue{
					Severity:    SeverityWarning,
					Category:    "broken-requirement-link",
					Description: "CUJ references non-existent requirement: " + linkedReq,
					Location:    location,
				})
				result.Score -= 0.05
			}
		}

		// Check: Priority is valid
		if cuj.Priority != "" && !isValidPriority(cuj.Priority) {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityInfo,
				Category:    "invalid-priority",
				Description: "CUJ has non-standard priority value: " + cuj.Priority,
				Location:    location,
			})
		}
	}

	// Check for duplicate CUJ titles
	titleCounts := make(map[string]int)
	for _, cuj := range spec.CriticalUserJourneys {
		lower := strings.ToLower(strings.TrimSpace(cuj.Title))
		if lower != "" {
			titleCounts[lower]++
		}
	}
	for title, count := range titleCounts {
		if count > 1 {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityWarning,
				Category:    "duplicate-cuj-title",
				Description: "Multiple CUJs have the same title: " + title,
				Location:    "critical_user_journeys",
			})
			result.Score -= 0.1
		}
	}

	// Suggestions
	if len(spec.CriticalUserJourneys) < 2 {
		result.Suggestions = append(result.Suggestions,
			"Consider adding more CUJs to cover alternative paths and edge cases")
	}

	// Clamp score
	if result.Score < 0 {
		result.Score = 0
	}

	return result, nil
}

// countVagueSteps counts steps that are too short or vague to be actionable.
func (r *CUJConsistencyReviewer) countVagueSteps(steps []string) int {
	vagueCount := 0
	vaguePatterns := []string{
		"do something", "click", "enter", "proceed", "continue",
		"user does", "system does", "etc",
	}

	for _, step := range steps {
		lower := strings.ToLower(step)

		// Too short
		if len(step) < 10 {
			vagueCount++
			continue
		}

		// Contains vague patterns
		for _, pattern := range vaguePatterns {
			if lower == pattern || (len(lower) < 20 && strings.Contains(lower, pattern)) {
				vagueCount++
				break
			}
		}
	}

	return vagueCount
}

// requirementExists checks if a requirement reference is valid.
func (r *CUJConsistencyReviewer) requirementExists(ref string, reqIDs map[string]bool) bool {
	// Check direct ID match
	if reqIDs[ref] {
		return true
	}

	// Check lowercase content match
	lower := strings.ToLower(ref)
	for id := range reqIDs {
		if strings.Contains(id, lower) || strings.Contains(lower, id) {
			return true
		}
	}

	return false
}

func formatReqID(index int) string {
	return "REQ-" + string(rune('0'+index))
}

func formatCUJLocation(index int) string {
	return "critical_user_journeys[" + string(rune('0'+index)) + "]"
}

func isValidPriority(priority string) bool {
	valid := []string{"p0", "p1", "p2", "p3", "critical", "high", "medium", "low"}
	lower := strings.ToLower(priority)
	for _, v := range valid {
		if lower == v {
			return true
		}
	}
	return false
}
