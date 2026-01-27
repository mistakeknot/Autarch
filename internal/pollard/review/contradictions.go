package review

import (
	"context"
	"strings"

	"github.com/mistakeknot/autarch/internal/pollard/insights"
)

// ContradictionDetector identifies conflicting findings within or across insights.
type ContradictionDetector struct {
	// OtherInsights to compare against for cross-insight contradictions.
	OtherInsights []*insights.Insight
}

// NewContradictionDetector creates a new contradiction detector.
func NewContradictionDetector() *ContradictionDetector {
	return &ContradictionDetector{}
}

// WithOtherInsights sets other insights to check for cross-insight contradictions.
func (r *ContradictionDetector) WithOtherInsights(others []*insights.Insight) *ContradictionDetector {
	r.OtherInsights = others
	return r
}

// Name returns the reviewer's identifier.
func (r *ContradictionDetector) Name() string {
	return "contradiction-detector"
}

// Review looks for internal and external contradictions.
func (r *ContradictionDetector) Review(ctx context.Context, insight *insights.Insight) (*ReviewResult, error) {
	result := &ReviewResult{
		Reviewer: r.Name(),
		Score:    1.0,
	}

	// Check for internal contradictions
	internalContradictions := r.findInternalContradictions(insight)
	for _, contradiction := range internalContradictions {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityWarning,
			Category:    "internal-contradiction",
			Description: contradiction,
			Location:    "findings",
		})
		result.Score -= 0.15
	}

	// Check for external contradictions (if other insights provided)
	if len(r.OtherInsights) > 0 {
		externalContradictions := r.findExternalContradictions(insight)
		for _, contradiction := range externalContradictions {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityInfo,
				Category:    "external-contradiction",
				Description: contradiction,
				Location:    "findings",
			})
			result.Score -= 0.1
		}
	}

	// Check for confidence inconsistencies
	if r.hasConfidenceInconsistency(insight) {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityInfo,
			Category:    "confidence-inconsistency",
			Description: "High-relevance finding lacks supporting evidence while low-relevance finding has extensive evidence",
			Location:    "findings",
		})
		result.Score -= 0.05
	}

	// Suggestions
	if len(r.OtherInsights) == 0 && len(insight.Findings) > 3 {
		result.Suggestions = append(result.Suggestions,
			"Consider cross-checking findings against other insights in the same research brief")
	}

	// Clamp score
	if result.Score < 0 {
		result.Score = 0
	}

	return result, nil
}

// findInternalContradictions looks for conflicting statements within an insight.
func (r *ContradictionDetector) findInternalContradictions(insight *insights.Insight) []string {
	var contradictions []string

	// Extract key claims from findings
	claims := make([]findingClaim, 0, len(insight.Findings))
	for i, finding := range insight.Findings {
		claims = append(claims, extractClaims(finding, i)...)
	}

	// Look for contradictory claims
	for i := 0; i < len(claims); i++ {
		for j := i + 1; j < len(claims); j++ {
			if areContradictory(claims[i], claims[j]) {
				contradictions = append(contradictions,
					"Finding "+claims[i].source+" may contradict finding "+claims[j].source)
			}
		}
	}

	return contradictions
}

// findExternalContradictions looks for conflicts with other insights.
func (r *ContradictionDetector) findExternalContradictions(insight *insights.Insight) []string {
	var contradictions []string

	for _, other := range r.OtherInsights {
		if other.ID == insight.ID {
			continue // Skip self
		}

		// Compare key claims
		for _, finding := range insight.Findings {
			for _, otherFinding := range other.Findings {
				if claimsContradict(finding, otherFinding) {
					contradictions = append(contradictions,
						"Finding '"+truncate(finding.Title, 30)+"' may contradict '"+
							truncate(otherFinding.Title, 30)+"' in insight "+other.ID)
				}
			}
		}
	}

	return contradictions
}

// hasConfidenceInconsistency checks if evidence doesn't match relevance levels.
func (r *ContradictionDetector) hasConfidenceInconsistency(insight *insights.Insight) bool {
	var highRelevanceWithNoEvidence, lowRelevanceWithEvidence bool

	for _, finding := range insight.Findings {
		hasEvidence := len(finding.Evidence) > 0
		if finding.Relevance == insights.RelevanceHigh && !hasEvidence {
			highRelevanceWithNoEvidence = true
		}
		if finding.Relevance == insights.RelevanceLow && hasEvidence && len(finding.Evidence) > 2 {
			lowRelevanceWithEvidence = true
		}
	}

	return highRelevanceWithNoEvidence && lowRelevanceWithEvidence
}

// findingClaim represents an extracted claim from a finding.
type findingClaim struct {
	subject   string // What the claim is about
	predicate string // What is claimed (positive/negative/comparison)
	source    string // Which finding it came from
}

// extractClaims pulls out key claims from a finding.
func extractClaims(finding insights.Finding, index int) []findingClaim {
	var claims []findingClaim
	source := "findings[" + string(rune('0'+index)) + "]"

	// Extract claims from description
	desc := strings.ToLower(finding.Description)

	// Look for comparative claims
	comparatives := []string{"better than", "worse than", "faster than", "slower than", "more than", "less than"}
	for _, comp := range comparatives {
		if strings.Contains(desc, comp) {
			claims = append(claims, findingClaim{
				subject:   finding.Title,
				predicate: comp,
				source:    source,
			})
		}
	}

	// Look for definitive claims
	definitives := []string{"always", "never", "must", "cannot", "impossible", "guaranteed"}
	for _, def := range definitives {
		if strings.Contains(desc, def) {
			claims = append(claims, findingClaim{
				subject:   finding.Title,
				predicate: def,
				source:    source,
			})
		}
	}

	return claims
}

// areContradictory checks if two claims contradict each other.
func areContradictory(a, b findingClaim) bool {
	// Same subject with opposite predicates
	if !strings.Contains(strings.ToLower(a.subject), strings.ToLower(b.subject)) &&
		!strings.Contains(strings.ToLower(b.subject), strings.ToLower(a.subject)) {
		return false // Different subjects
	}

	// Check for opposite comparatives
	opposites := map[string]string{
		"better than":  "worse than",
		"faster than":  "slower than",
		"more than":    "less than",
		"always":       "never",
		"must":         "cannot",
		"possible":     "impossible",
		"guaranteed":   "never",
	}

	if opposite, ok := opposites[a.predicate]; ok && opposite == b.predicate {
		return true
	}
	if opposite, ok := opposites[b.predicate]; ok && opposite == a.predicate {
		return true
	}

	return false
}

// claimsContradict checks if two findings make contradictory claims.
func claimsContradict(a, b insights.Finding) bool {
	// Simple heuristic: same topic with opposite sentiment
	aTitle := strings.ToLower(a.Title)
	bTitle := strings.ToLower(b.Title)

	// Check if they're about similar topics
	if !hasSimilarTopic(aTitle, bTitle) {
		return false
	}

	// Check for opposite sentiment indicators
	aPositive := hasPositiveSentiment(a.Description)
	bPositive := hasPositiveSentiment(b.Description)
	aNegative := hasNegativeSentiment(a.Description)
	bNegative := hasNegativeSentiment(b.Description)

	return (aPositive && bNegative) || (aNegative && bPositive)
}

func hasSimilarTopic(a, b string) bool {
	// Extract key words and compare
	aWords := strings.Fields(a)
	bWords := strings.Fields(b)

	matches := 0
	for _, aw := range aWords {
		if len(aw) < 4 {
			continue // Skip short words
		}
		for _, bw := range bWords {
			if aw == bw {
				matches++
			}
		}
	}

	return matches >= 2
}

func hasPositiveSentiment(text string) bool {
	lower := strings.ToLower(text)
	positives := []string{"good", "great", "excellent", "fast", "efficient", "improved", "better", "success"}
	for _, p := range positives {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func hasNegativeSentiment(text string) bool {
	lower := strings.ToLower(text)
	negatives := []string{"bad", "poor", "slow", "inefficient", "worse", "failed", "problem", "issue"}
	for _, n := range negatives {
		if strings.Contains(lower, n) {
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
