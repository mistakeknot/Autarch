package review

import (
	"context"
	"net/url"
	"strings"

	"github.com/mistakeknot/autarch/internal/pollard/insights"
)

// SourceCredibilityReviewer validates the quality and trustworthiness of sources.
type SourceCredibilityReviewer struct{}

// NewSourceCredibilityReviewer creates a new source credibility reviewer.
func NewSourceCredibilityReviewer() *SourceCredibilityReviewer {
	return &SourceCredibilityReviewer{}
}

// Name returns the reviewer's identifier.
func (r *SourceCredibilityReviewer) Name() string {
	return "source-credibility"
}

// Review analyzes source quality and trustworthiness.
func (r *SourceCredibilityReviewer) Review(ctx context.Context, insight *insights.Insight) (*ReviewResult, error) {
	result := &ReviewResult{
		Reviewer: r.Name(),
		Score:    1.0,
	}

	// Check: Has sources
	if len(insight.Sources) == 0 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityError,
			Category:    "missing-sources",
			Description: "Insight has no sources - findings cannot be verified",
			Location:    "sources",
		})
		result.Score -= 0.4
	}

	// Check each source
	for i, source := range insight.Sources {
		// Validate URL format
		if source.URL != "" {
			if _, err := url.Parse(source.URL); err != nil {
				result.Issues = append(result.Issues, Issue{
					Severity:    SeverityWarning,
					Category:    "invalid-url",
					Description: "Source URL is malformed",
					Location:    formatSourceLocation(i),
				})
				result.Score -= 0.1
			}
		}

		// Check source type is specified
		if source.Type == "" {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityInfo,
				Category:    "missing-type",
				Description: "Source type not specified (product, github, article, docs)",
				Location:    formatSourceLocation(i),
			})
			result.Score -= 0.05
		}

		// Evaluate source credibility based on domain
		credibility := evaluateSourceCredibility(source)
		if credibility < 0.5 {
			result.Issues = append(result.Issues, Issue{
				Severity:    SeverityWarning,
				Category:    "low-credibility",
				Description: "Source may have low credibility - consider additional verification",
				Location:    formatSourceLocation(i),
			})
			result.Score -= 0.1
		}
	}

	// Check: Findings have evidence
	evidenceCount := 0
	for _, finding := range insight.Findings {
		if len(finding.Evidence) > 0 {
			evidenceCount++
		}
	}
	if len(insight.Findings) > 0 && evidenceCount == 0 {
		result.Issues = append(result.Issues, Issue{
			Severity:    SeverityWarning,
			Category:    "missing-evidence",
			Description: "No findings have supporting evidence",
			Location:    "findings",
		})
		result.Score -= 0.15
	}

	// Suggestions
	if len(insight.Sources) == 1 {
		result.Suggestions = append(result.Suggestions,
			"Consider adding additional sources to corroborate findings")
	}

	// Clamp score
	if result.Score < 0 {
		result.Score = 0
	}

	return result, nil
}

// evaluateSourceCredibility scores a source's trustworthiness (0.0-1.0).
func evaluateSourceCredibility(source insights.Source) float64 {
	if source.URL == "" {
		return 0.3 // No URL is suspicious
	}

	u, err := url.Parse(source.URL)
	if err != nil {
		return 0.2
	}

	domain := strings.ToLower(u.Host)

	// High credibility domains
	highCredibility := []string{
		"github.com", "arxiv.org", "doi.org", "nature.com", "science.org",
		"acm.org", "ieee.org", "springer.com", "nih.gov", "gov",
		"edu", "ac.uk", "wikipedia.org",
	}
	for _, hc := range highCredibility {
		if strings.Contains(domain, hc) {
			return 0.9
		}
	}

	// Medium credibility (established tech/news)
	mediumCredibility := []string{
		"medium.com", "dev.to", "hackernews", "reddit.com", "stackoverflow.com",
		"techcrunch.com", "theverge.com", "wired.com", "arstechnica.com",
	}
	for _, mc := range mediumCredibility {
		if strings.Contains(domain, mc) {
			return 0.7
		}
	}

	// Product/company sites (variable credibility)
	if source.Type == "product" || source.Type == "docs" {
		return 0.6 // Primary sources are okay for product info
	}

	return 0.5 // Unknown - neutral
}

func formatSourceLocation(index int) string {
	return "sources[" + string(rune('0'+index)) + "]"
}
