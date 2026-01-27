package review

import (
	"context"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// ScopeCreepDetector identifies overly broad requirements and scope issues.
type ScopeCreepDetector struct {
	// MaxRequirements is the threshold above which we warn about scope.
	MaxRequirements int
	// MaxCUJs is the threshold for too many user journeys.
	MaxCUJs int
}

// NewScopeCreepDetector creates a new scope creep detector with defaults.
func NewScopeCreepDetector() *ScopeCreepDetector {
	return &ScopeCreepDetector{
		MaxRequirements: 15,
		MaxCUJs:         8,
	}
}

// Name returns the reviewer's identifier.
func (r *ScopeCreepDetector) Name() string {
	return "scope-creep"
}

// Review analyzes the PRD for signs of scope creep.
func (r *ScopeCreepDetector) Review(ctx context.Context, spec *specs.Spec) (*ReviewResult, error) {
	result := &ReviewResult{
		Reviewer: r.Name(),
		Score:    1.0,
	}

	// Check: Too many requirements
	if len(spec.Requirements) > r.MaxRequirements {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityWarning,
			Category:    "too-many-requirements",
			Description: "PRD has many requirements - consider splitting into multiple PRDs",
			Location:    "requirements",
		})
		result.Score -= 0.15
	}

	// Check: Too many CUJs
	if len(spec.CriticalUserJourneys) > r.MaxCUJs {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityWarning,
			Category:    "too-many-cujs",
			Description: "PRD has many CUJs - may indicate scope is too broad",
			Location:    "critical_user_journeys",
		})
		result.Score -= 0.1
	}

	// Check: Goals vs Non-Goals alignment
	if len(spec.Goals) > 0 && len(spec.NonGoals) > 0 {
		conflicts := r.findGoalNonGoalConflicts(spec.Goals, spec.NonGoals)
		for _, conflict := range conflicts {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityError,
				Category:    "goal-nongoal-conflict",
				Description: conflict,
				Location:    "goals/non_goals",
			})
			result.Score -= 0.2
		}
	}

	// Check: Requirements alignment with goals
	if len(spec.Goals) > 0 && len(spec.Requirements) > 0 {
		unalignedReqs := r.findUnalignedRequirements(spec.Requirements, spec.Goals)
		if len(unalignedReqs) > len(spec.Requirements)/3 {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityWarning,
				Category:    "unaligned-requirements",
				Description: "Several requirements don't clearly support stated goals",
				Location:    "requirements",
			})
			result.Score -= 0.1
		}
	}

	// Check: Requirements that match non-goals
	if len(spec.NonGoals) > 0 && len(spec.Requirements) > 0 {
		conflictingReqs := r.findRequirementsMatchingNonGoals(spec.Requirements, spec.NonGoals)
		for _, conflict := range conflictingReqs {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityError,
				Category:    "requirement-matches-nongoal",
				Description: conflict,
				Location:    "requirements",
			})
			result.Score -= 0.15
		}
	}

	// Check: Scope creep indicators in requirements
	scopeCreepIndicators := r.countScopeCreepIndicators(spec.Requirements)
	if scopeCreepIndicators > len(spec.Requirements)/4 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityWarning,
			Category:    "scope-creep-language",
			Description: "Requirements contain language suggesting scope expansion (also, additionally, furthermore)",
			Location:    "requirements",
		})
		result.Score -= 0.1
	}

	// Check: Complexity vs requirements count
	if spec.Complexity != "" && len(spec.Requirements) > 0 {
		if (spec.Complexity == "low" || spec.Complexity == "simple") && len(spec.Requirements) > 8 {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityInfo,
				Category:    "complexity-mismatch",
				Description: "PRD marked as simple but has many requirements",
				Location:    "complexity",
			})
			result.Score -= 0.05
		}
	}

	// Check: MVP scope
	mvpIncluded := false
	for _, cuj := range spec.CriticalUserJourneys {
		if cuj.Priority == "p0" || cuj.Priority == "critical" {
			mvpIncluded = true
			break
		}
	}
	if spec.StrategicContext.MVPIncluded && !mvpIncluded {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityInfo,
			Category:    "unclear-mvp",
			Description: "PRD is marked as MVP but no critical-priority CUJs are defined",
			Location:    "strategic_context",
		})
		result.Score -= 0.05
	}

	// Suggestions for scope management
	if len(spec.NonGoals) == 0 && len(spec.Requirements) > 5 {
		result.Suggestions = append(result.Suggestions,
			"Add non-goals to explicitly define what's out of scope")
	}

	if len(spec.Goals) == 0 && len(spec.Requirements) > 5 {
		result.Suggestions = append(result.Suggestions,
			"Add measurable goals to help prioritize requirements")
	}

	// Clamp score
	if result.Score < 0 {
		result.Score = 0
	}

	return result, nil
}

// findGoalNonGoalConflicts looks for goals that contradict non-goals.
func (r *ScopeCreepDetector) findGoalNonGoalConflicts(goals []specs.Goal, nonGoals []specs.NonGoal) []string {
	var conflicts []string

	for _, goal := range goals {
		goalWords := extractKeywords(goal.Description)
		for _, nonGoal := range nonGoals {
			nonGoalWords := extractKeywords(nonGoal.Description)
			overlap := countOverlap(goalWords, nonGoalWords)
			if overlap >= 3 {
				conflicts = append(conflicts,
					"Goal '"+truncateString(goal.Description, 30)+"' may conflict with non-goal '"+
						truncateString(nonGoal.Description, 30)+"'")
			}
		}
	}

	return conflicts
}

// findUnalignedRequirements finds requirements that don't support any goal.
func (r *ScopeCreepDetector) findUnalignedRequirements(reqs []string, goals []specs.Goal) []int {
	var unaligned []int

	// Build goal keywords
	goalKeywords := make(map[string]bool)
	for _, goal := range goals {
		for _, kw := range extractKeywords(goal.Description) {
			goalKeywords[kw] = true
		}
		for _, kw := range extractKeywords(goal.Metric) {
			goalKeywords[kw] = true
		}
	}

	for i, req := range reqs {
		reqWords := extractKeywords(req)
		matchCount := 0
		for _, word := range reqWords {
			if goalKeywords[word] {
				matchCount++
			}
		}
		if matchCount < 2 {
			unaligned = append(unaligned, i)
		}
	}

	return unaligned
}

// findRequirementsMatchingNonGoals finds requirements that match non-goals.
func (r *ScopeCreepDetector) findRequirementsMatchingNonGoals(reqs []string, nonGoals []specs.NonGoal) []string {
	var conflicts []string

	for _, nonGoal := range nonGoals {
		nonGoalWords := extractKeywords(nonGoal.Description)
		for _, req := range reqs {
			reqWords := extractKeywords(req)
			overlap := countOverlap(nonGoalWords, reqWords)
			if overlap >= 3 {
				conflicts = append(conflicts,
					"Requirement '"+truncateString(req, 30)+"' may contradict non-goal '"+
						truncateString(nonGoal.Description, 30)+"'")
			}
		}
	}

	return conflicts
}

// countScopeCreepIndicators counts requirements with scope expansion language.
func (r *ScopeCreepDetector) countScopeCreepIndicators(reqs []string) int {
	indicators := []string{
		"also", "additionally", "furthermore", "moreover",
		"as well as", "in addition", "plus", "along with",
		"and also", "not only", "but also",
		"while we're at it", "might as well",
	}

	count := 0
	for _, req := range reqs {
		lower := strings.ToLower(req)
		for _, indicator := range indicators {
			if strings.Contains(lower, indicator) {
				count++
				break
			}
		}
	}

	return count
}

// extractKeywords extracts significant words from text.
func extractKeywords(text string) []string {
	// Simple stopword removal
	stopwords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "must": true,
		"that": true, "this": true, "these": true, "those": true,
		"it": true, "its": true, "as": true, "if": true, "when": true,
	}

	words := strings.Fields(strings.ToLower(text))
	var keywords []string

	for _, word := range words {
		// Clean punctuation
		word = strings.Trim(word, ".,;:!?\"'()[]{}+-*/")
		if len(word) > 2 && !stopwords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// countOverlap counts how many words appear in both slices.
func countOverlap(a, b []string) int {
	aSet := make(map[string]bool)
	for _, word := range a {
		aSet[word] = true
	}

	overlap := 0
	for _, word := range b {
		if aSet[word] {
			overlap++
		}
	}

	return overlap
}
