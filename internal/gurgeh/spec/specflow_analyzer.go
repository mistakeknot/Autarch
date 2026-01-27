// Package spec provides PRD specification analysis tools.
package spec

import (
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// SpecFlowAnalyzer identifies gaps in PRD specifications.
// It examines the spec from a user flow perspective, looking for
// missing edge cases, unclear criteria, and incomplete journeys.
type SpecFlowAnalyzer struct{}

// NewSpecFlowAnalyzer creates a new analyzer.
func NewSpecFlowAnalyzer() *SpecFlowAnalyzer {
	return &SpecFlowAnalyzer{}
}

// Gap represents a specification deficiency.
type Gap struct {
	// Category classifies the type of gap.
	Category GapCategory

	// Description explains what's missing or unclear.
	Description string

	// Severity indicates how critical this gap is.
	Severity GapSeverity

	// Location points to where in the PRD the gap was found.
	Location string

	// Suggestion provides guidance on how to address the gap.
	Suggestion string
}

// GapCategory classifies types of specification gaps.
type GapCategory string

const (
	CategoryMissingFlow      GapCategory = "missing_flow"
	CategoryUnclearCriteria  GapCategory = "unclear_criteria"
	CategoryEdgeCase         GapCategory = "edge_case"
	CategoryErrorHandling    GapCategory = "error_handling"
	CategoryStateTransition  GapCategory = "state_transition"
	CategoryDataValidation   GapCategory = "data_validation"
	CategoryIntegrationPoint GapCategory = "integration_point"
)

// GapSeverity indicates how critical a gap is.
type GapSeverity string

const (
	SeverityBlocker    GapSeverity = "blocker"    // Must fix before implementation
	SeverityWarning    GapSeverity = "warning"    // Should fix
	SeveritySuggestion GapSeverity = "suggestion" // Nice to have
)

// AnalysisResult contains all gaps found during analysis.
type AnalysisResult struct {
	// SpecID identifies the analyzed PRD.
	SpecID string

	// Gaps found during analysis.
	Gaps []Gap

	// Coverage is an estimated completeness score (0.0-1.0).
	Coverage float64

	// Blockers counts gaps with blocker severity.
	Blockers int

	// Warnings counts gaps with warning severity.
	Warnings int

	// ReadyForImplementation is true if no blockers exist.
	ReadyForImplementation bool
}

// Analyze examines a PRD for completeness gaps.
func (a *SpecFlowAnalyzer) Analyze(spec *specs.Spec) *AnalysisResult {
	result := &AnalysisResult{
		SpecID:   spec.ID,
		Gaps:     make([]Gap, 0),
		Coverage: 1.0,
	}

	// Run all gap detection checks
	a.checkRequirementCoverage(spec, result)
	a.checkCUJCompleteness(spec, result)
	a.checkAcceptanceCriteriaCoverage(spec, result)
	a.checkEdgeCases(spec, result)
	a.checkErrorHandling(spec, result)
	a.checkStateTransitions(spec, result)
	a.checkDataValidation(spec, result)
	a.checkIntegrationPoints(spec, result)

	// Count severities
	for _, gap := range result.Gaps {
		switch gap.Severity {
		case SeverityBlocker:
			result.Blockers++
			result.Coverage -= 0.15
		case SeverityWarning:
			result.Warnings++
			result.Coverage -= 0.05
		case SeveritySuggestion:
			result.Coverage -= 0.02
		}
	}

	// Clamp coverage
	if result.Coverage < 0 {
		result.Coverage = 0
	}

	result.ReadyForImplementation = result.Blockers == 0

	return result
}

// checkRequirementCoverage verifies each requirement has acceptance criteria.
func (a *SpecFlowAnalyzer) checkRequirementCoverage(spec *specs.Spec, result *AnalysisResult) {
	if len(spec.Requirements) == 0 {
		return
	}

	// Build acceptance criteria keyword index
	acKeywords := make(map[string]bool)
	for _, ac := range spec.Acceptance {
		for _, word := range extractSignificantWords(ac.Description) {
			acKeywords[word] = true
		}
	}

	// Check each requirement has related acceptance criteria
	for i, req := range spec.Requirements {
		reqWords := extractSignificantWords(req)
		matchCount := 0
		for _, word := range reqWords {
			if acKeywords[word] {
				matchCount++
			}
		}

		// If less than 20% of significant words match, flag as uncovered
		if len(reqWords) > 0 && float64(matchCount)/float64(len(reqWords)) < 0.2 {
			result.Gaps = append(result.Gaps, Gap{
				Category:    CategoryUnclearCriteria,
				Description: "Requirement may not have acceptance criteria: " + truncate(req, 50),
				Severity:    SeverityWarning,
				Location:    formatReqLocation(i),
				Suggestion:  "Add acceptance criteria that verify this requirement is met",
			})
		}
	}
}

// checkCUJCompleteness verifies CUJs have complete flows.
func (a *SpecFlowAnalyzer) checkCUJCompleteness(spec *specs.Spec, result *AnalysisResult) {
	for i, cuj := range spec.CriticalUserJourneys {
		location := formatCUJLocation(i)

		// Check: Has enough steps
		if len(cuj.Steps) > 0 && len(cuj.Steps) < 3 {
			result.Gaps = append(result.Gaps, Gap{
				Category:    CategoryMissingFlow,
				Description: "CUJ '" + cuj.Title + "' has very few steps - may be incomplete",
				Severity:    SeverityWarning,
				Location:    location,
				Suggestion:  "Break down the journey into more granular user actions",
			})
		}

		// Check: Steps have clear actor
		for j, step := range cuj.Steps {
			if !hasActor(step) {
				result.Gaps = append(result.Gaps, Gap{
					Category:    CategoryMissingFlow,
					Description: "Step lacks clear actor (who performs this action?)",
					Severity:    SeveritySuggestion,
					Location:    location + ".steps[" + itoa(j) + "]",
					Suggestion:  "Start step with 'User...' or 'System...' to clarify actor",
				})
			}
		}

		// Check: Success criteria are measurable
		for j, criterion := range cuj.SuccessCriteria {
			if !isMeasurableCriterion(criterion) {
				result.Gaps = append(result.Gaps, Gap{
					Category:    CategoryUnclearCriteria,
					Description: "Success criterion is not measurable: " + truncate(criterion, 40),
					Severity:    SeverityWarning,
					Location:    location + ".success_criteria[" + itoa(j) + "]",
					Suggestion:  "Add specific observable outcomes or metrics",
				})
			}
		}
	}
}

// checkAcceptanceCriteriaCoverage checks AC coverage vs requirements.
func (a *SpecFlowAnalyzer) checkAcceptanceCriteriaCoverage(spec *specs.Spec, result *AnalysisResult) {
	if len(spec.Requirements) > 0 && len(spec.Acceptance) == 0 {
		result.Gaps = append(result.Gaps, Gap{
			Category:    CategoryUnclearCriteria,
			Description: "PRD has requirements but no acceptance criteria",
			Severity:    SeverityBlocker,
			Location:    "acceptance_criteria",
			Suggestion:  "Add at least one acceptance criterion per requirement",
		})
		return
	}

	// Check ratio
	if len(spec.Requirements) > 0 && float64(len(spec.Acceptance))/float64(len(spec.Requirements)) < 0.5 {
		result.Gaps = append(result.Gaps, Gap{
			Category:    CategoryUnclearCriteria,
			Description: "Low acceptance criteria to requirements ratio",
			Severity:    SeverityWarning,
			Location:    "acceptance_criteria",
			Suggestion:  "Consider adding more acceptance criteria to cover all requirements",
		})
	}
}

// checkEdgeCases looks for missing edge case handling.
func (a *SpecFlowAnalyzer) checkEdgeCases(spec *specs.Spec, result *AnalysisResult) {
	// Common edge cases to check for
	edgeCasePatterns := []struct {
		trigger    string
		edgeCase   string
		suggestion string
	}{
		{"list", "empty list", "Specify behavior when list is empty"},
		{"search", "no results", "Specify behavior when search returns no results"},
		{"form", "invalid input", "Specify validation rules and error messages"},
		{"upload", "file too large", "Specify maximum file size and error handling"},
		{"login", "invalid credentials", "Specify behavior for failed authentication"},
		{"delete", "confirmation", "Specify confirmation flow for destructive actions"},
		{"timeout", "connection loss", "Specify behavior when network is unavailable"},
		{"concurrent", "race condition", "Specify behavior for simultaneous updates"},
	}

	// Combine all text for searching
	allText := strings.ToLower(spec.Title + " " + spec.Summary)
	for _, req := range spec.Requirements {
		allText += " " + strings.ToLower(req)
	}
	for _, cuj := range spec.CriticalUserJourneys {
		allText += " " + strings.ToLower(cuj.Title)
		for _, step := range cuj.Steps {
			allText += " " + strings.ToLower(step)
		}
	}

	// Check for edge case coverage
	for _, pattern := range edgeCasePatterns {
		if strings.Contains(allText, pattern.trigger) {
			// Check if edge case is addressed
			if !strings.Contains(allText, pattern.edgeCase) &&
				!strings.Contains(allText, "error") &&
				!strings.Contains(allText, "fail") {
				result.Gaps = append(result.Gaps, Gap{
					Category:    CategoryEdgeCase,
					Description: "Feature involves '" + pattern.trigger + "' but doesn't address: " + pattern.edgeCase,
					Severity:    SeveritySuggestion,
					Location:    "requirements",
					Suggestion:  pattern.suggestion,
				})
			}
		}
	}
}

// checkErrorHandling looks for missing error handling specifications.
func (a *SpecFlowAnalyzer) checkErrorHandling(spec *specs.Spec, result *AnalysisResult) {
	// Check if any CUJ or requirement mentions error handling
	hasErrorHandling := false

	allText := ""
	for _, cuj := range spec.CriticalUserJourneys {
		for _, step := range cuj.Steps {
			allText += " " + strings.ToLower(step)
		}
		for _, sc := range cuj.SuccessCriteria {
			allText += " " + strings.ToLower(sc)
		}
	}
	for _, ac := range spec.Acceptance {
		allText += " " + strings.ToLower(ac.Description)
	}

	errorIndicators := []string{"error", "fail", "invalid", "exception", "reject", "deny"}
	for _, indicator := range errorIndicators {
		if strings.Contains(allText, indicator) {
			hasErrorHandling = true
			break
		}
	}

	// If no error handling mentioned and we have multiple CUJs, flag it
	if !hasErrorHandling && len(spec.CriticalUserJourneys) > 0 {
		result.Gaps = append(result.Gaps, Gap{
			Category:    CategoryErrorHandling,
			Description: "No error handling scenarios specified in CUJs or acceptance criteria",
			Severity:    SeverityWarning,
			Location:    "critical_user_journeys",
			Suggestion:  "Add CUJs or acceptance criteria for error scenarios",
		})
	}
}

// checkStateTransitions looks for unclear state transitions.
func (a *SpecFlowAnalyzer) checkStateTransitions(spec *specs.Spec, result *AnalysisResult) {
	// Look for state-related words
	stateWords := []string{"status", "state", "phase", "stage", "mode", "pending", "active", "complete", "draft", "published"}

	allText := strings.ToLower(spec.Summary)
	for _, req := range spec.Requirements {
		allText += " " + strings.ToLower(req)
	}

	hasStateReferences := false
	for _, word := range stateWords {
		if strings.Contains(allText, word) {
			hasStateReferences = true
			break
		}
	}

	if hasStateReferences {
		// Check if transitions are defined
		transitionWords := []string{"transition", "change to", "becomes", "moves to", "from", "->", "→"}
		hasTransitions := false
		for _, word := range transitionWords {
			if strings.Contains(allText, word) {
				hasTransitions = true
				break
			}
		}

		if !hasTransitions {
			result.Gaps = append(result.Gaps, Gap{
				Category:    CategoryStateTransition,
				Description: "PRD references states but doesn't define valid transitions",
				Severity:    SeveritySuggestion,
				Location:    "requirements",
				Suggestion:  "Add a state diagram or list of valid state transitions",
			})
		}
	}
}

// checkDataValidation looks for missing validation rules.
func (a *SpecFlowAnalyzer) checkDataValidation(spec *specs.Spec, result *AnalysisResult) {
	// Look for data-related words that typically need validation
	dataPatterns := []struct {
		pattern  string
		question string
	}{
		{"email", "What format is valid for email?"},
		{"phone", "What format is valid for phone numbers?"},
		{"password", "What are the password requirements?"},
		{"date", "What date format is expected?"},
		{"amount", "What are the min/max amounts?"},
		{"quantity", "What are valid quantity ranges?"},
		{"url", "What URL formats are accepted?"},
		{"file", "What file types and sizes are allowed?"},
	}

	allText := strings.ToLower(spec.Summary)
	for _, req := range spec.Requirements {
		allText += " " + strings.ToLower(req)
	}
	for _, ac := range spec.Acceptance {
		allText += " " + strings.ToLower(ac.Description)
	}

	for _, pattern := range dataPatterns {
		if strings.Contains(allText, pattern.pattern) {
			// Check if validation is specified
			validationIndicators := []string{"valid", "format", "must be", "required", "minimum", "maximum", "between", "at least", "no more than"}
			hasValidation := false
			for _, indicator := range validationIndicators {
				if strings.Contains(allText, indicator) {
					hasValidation = true
					break
				}
			}

			if !hasValidation {
				result.Gaps = append(result.Gaps, Gap{
					Category:    CategoryDataValidation,
					Description: "PRD mentions '" + pattern.pattern + "' but lacks validation rules",
					Severity:    SeveritySuggestion,
					Location:    "requirements",
					Suggestion:  pattern.question,
				})
			}
		}
	}
}

// checkIntegrationPoints looks for external dependencies that need specification.
func (a *SpecFlowAnalyzer) checkIntegrationPoints(spec *specs.Spec, result *AnalysisResult) {
	integrationIndicators := []string{
		"api", "webhook", "callback", "integration", "third-party", "external",
		"oauth", "sso", "sync", "import", "export", "connect",
	}

	allText := strings.ToLower(spec.Summary)
	for _, req := range spec.Requirements {
		allText += " " + strings.ToLower(req)
	}

	for _, indicator := range integrationIndicators {
		if strings.Contains(allText, indicator) {
			// Check if integration details are specified
			detailIndicators := []string{"endpoint", "format", "protocol", "authentication", "rate limit", "retry", "timeout"}
			hasDetails := false
			for _, detail := range detailIndicators {
				if strings.Contains(allText, detail) {
					hasDetails = true
					break
				}
			}

			if !hasDetails {
				result.Gaps = append(result.Gaps, Gap{
					Category:    CategoryIntegrationPoint,
					Description: "PRD mentions '" + indicator + "' but lacks integration details",
					Severity:    SeverityWarning,
					Location:    "requirements",
					Suggestion:  "Specify endpoint, format, authentication, and error handling for integrations",
				})
				break // Only flag once
			}
		}
	}
}

// Helper functions

func extractSignificantWords(text string) []string {
	stopwords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"should": true, "must": true, "will": true, "can": true, "could": true,
		"that": true, "this": true, "it": true, "as": true, "if": true,
	}

	words := strings.Fields(strings.ToLower(text))
	var significant []string
	for _, word := range words {
		word = strings.Trim(word, ".,;:!?\"'()[]{}+-*/")
		if len(word) > 2 && !stopwords[word] {
			significant = append(significant, word)
		}
	}
	return significant
}

func hasActor(step string) bool {
	lower := strings.ToLower(step)
	actors := []string{"user", "system", "admin", "customer", "visitor", "agent", "service", "api"}
	for _, actor := range actors {
		if strings.Contains(lower, actor) {
			return true
		}
	}
	return false
}

func isMeasurableCriterion(criterion string) bool {
	lower := strings.ToLower(criterion)
	measurable := []string{
		"displays", "shows", "returns", "contains", "equals",
		"within", "less than", "more than", "at least",
		"seconds", "minutes", "percent", "%",
		"visible", "hidden", "enabled", "disabled",
		"success", "error", "complete", "fail",
	}
	for _, m := range measurable {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatReqLocation(index int) string {
	return "requirements[" + itoa(index) + "]"
}

func formatCUJLocation(index int) string {
	return "critical_user_journeys[" + itoa(index) + "]"
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
