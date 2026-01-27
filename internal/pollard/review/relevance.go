package review

import (
	"context"
	"strings"

	"github.com/mistakeknot/autarch/internal/pollard/insights"
)

// RelevanceReviewer scores how relevant findings are to the research brief.
type RelevanceReviewer struct {
	// ResearchContext contains keywords/topics from the research brief.
	// If empty, reviewer skips context-based relevance checks.
	ResearchContext []string
}

// NewRelevanceReviewer creates a new relevance reviewer.
func NewRelevanceReviewer() *RelevanceReviewer {
	return &RelevanceReviewer{}
}

// WithContext sets the research context for relevance scoring.
func (r *RelevanceReviewer) WithContext(keywords []string) *RelevanceReviewer {
	r.ResearchContext = keywords
	return r
}

// Name returns the reviewer's identifier.
func (r *RelevanceReviewer) Name() string {
	return "relevance"
}

// Review analyzes insight relevance and actionability.
func (r *RelevanceReviewer) Review(ctx context.Context, insight *insights.Insight) (*ReviewResult, error) {
	result := &ReviewResult{
		Reviewer: r.Name(),
		Score:    1.0,
	}

	// Check: Has findings
	if len(insight.Findings) == 0 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityError,
			Category:    "no-findings",
			Description: "Insight has no findings - nothing actionable",
			Location:    "findings",
		})
		result.Score -= 0.5
		return result, nil
	}

	// Check: Finding quality
	lowRelevanceCount := 0
	vagueFindingsCount := 0
	for i, finding := range insight.Findings {
		// Check relevance level
		if finding.Relevance == insights.RelevanceLow {
			lowRelevanceCount++
		}

		// Check for vague descriptions
		if isVagueDescription(finding.Description) {
			vagueFindingsCount++
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityWarning,
				Category:    "vague-finding",
				Description: "Finding description is too vague to be actionable",
				Location:    formatFindingLocation(i),
			})
			result.Score -= 0.1
		}

		// Check for missing title
		if strings.TrimSpace(finding.Title) == "" {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityInfo,
				Category:    "missing-title",
				Description: "Finding has no title",
				Location:    formatFindingLocation(i),
			})
			result.Score -= 0.05
		}
	}

	// Penalize if most findings are low relevance
	if len(insight.Findings) > 0 && float64(lowRelevanceCount)/float64(len(insight.Findings)) > 0.5 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityWarning,
			Category:    "low-relevance",
			Description: "More than half of findings are marked low relevance",
			Location:    "findings",
		})
		result.Score -= 0.2
	}

	// Check: Has recommendations
	if len(insight.Recommendations) == 0 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityInfo,
			Category:    "no-recommendations",
			Description: "No recommendations - findings may not be actionable",
			Location:    "recommendations",
		})
		result.Score -= 0.1
	} else {
		// Check recommendation quality
		for i, rec := range insight.Recommendations {
			if strings.TrimSpace(rec.FeatureHint) == "" {
				result.Issues = append(result.Issues, Issue{
					Severity:    SeverityWarning,
					Category:    "vague-recommendation",
					Description: "Recommendation has no feature hint",
					Location:    formatRecLocation(i),
				})
				result.Score -= 0.1
			}
		}
	}

	// Check: Context relevance (if context provided)
	if len(r.ResearchContext) > 0 {
		contextScore := r.scoreContextRelevance(insight)
		if contextScore < 0.3 {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityWarning,
				Category:    "off-topic",
				Description: "Insight appears unrelated to research context",
				Location:    "title",
			})
			result.Score -= 0.2
		}
	}

	// Suggestions
	if len(insight.LinkedFeatures) == 0 {
		result.Suggestions = append(result.Suggestions,
			"Consider linking findings to specific Gurgeh feature specs")
	}

	// Clamp score
	if result.Score < 0 {
		result.Score = 0
	}

	return result, nil
}

// isVagueDescription checks if a description lacks specificity.
func isVagueDescription(desc string) bool {
	if len(desc) < 20 {
		return true
	}

	vaguePatterns := []string{
		"interesting", "good", "bad", "nice", "cool",
		"something", "stuff", "things", "etc",
		"might be", "could be", "maybe",
	}

	lower := strings.ToLower(desc)
	vagueCount := 0
	for _, pattern := range vaguePatterns {
		if strings.Contains(lower, pattern) {
			vagueCount++
		}
	}

	// More than 2 vague words in a short description is concerning
	return vagueCount > 2 || (len(desc) < 50 && vagueCount > 0)
}

// scoreContextRelevance checks how well the insight matches research context.
func (r *RelevanceReviewer) scoreContextRelevance(insight *insights.Insight) float64 {
	if len(r.ResearchContext) == 0 {
		return 1.0 // No context to check against
	}

	// Build searchable text from insight
	text := strings.ToLower(insight.Title)
	for _, f := range insight.Findings {
		text += " " + strings.ToLower(f.Title)
		text += " " + strings.ToLower(f.Description)
	}

	// Count keyword matches
	matches := 0
	for _, keyword := range r.ResearchContext {
		if strings.Contains(text, strings.ToLower(keyword)) {
			matches++
		}
	}

	return float64(matches) / float64(len(r.ResearchContext))
}

func formatFindingLocation(index int) string {
	return "findings[" + string(rune('0'+index)) + "]"
}

func formatRecLocation(index int) string {
	return "recommendations[" + string(rune('0'+index)) + "]"
}
