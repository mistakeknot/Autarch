// Package intermute provides Intermute integration for Pollard research intelligence.
// It publishes research findings as Intermute Insights for cross-tool visibility.
package intermute

import (
	"context"
	"strings"

	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

// InsightCreator defines the interface for creating insights in Intermute.
type InsightCreator interface {
	CreateInsight(ctx context.Context, insight intermute.Insight) (intermute.Insight, error)
}

// Publisher publishes Pollard research findings as Intermute Insights.
type Publisher struct {
	client  InsightCreator
	project string
	specID  string // Optional: link insights to a spec
}

// NewPublisher creates a new research findings publisher.
// If client is nil, publish operations become no-ops (graceful degradation).
func NewPublisher(client InsightCreator, project string) *Publisher {
	return &Publisher{
		client:  client,
		project: project,
	}
}

// WithSpecID sets a spec ID to link all published insights to.
func (p *Publisher) WithSpecID(specID string) *Publisher {
	p.specID = specID
	return p
}

// PublishFinding publishes a single research finding as an Intermute Insight.
func (p *Publisher) PublishFinding(ctx context.Context, finding research.Finding) (intermute.Insight, error) {
	if p.client == nil {
		return intermute.Insight{}, nil
	}

	insight := mapFindingToInsight(finding, p.project, p.specID)
	return p.client.CreateInsight(ctx, insight)
}

// PublishFindings publishes multiple findings and returns the created insights.
// Errors from individual creations are logged but don't stop processing.
func (p *Publisher) PublishFindings(ctx context.Context, findings []research.Finding) ([]intermute.Insight, error) {
	if p.client == nil {
		return nil, nil
	}

	var results []intermute.Insight
	for _, finding := range findings {
		insight, err := p.PublishFinding(ctx, finding)
		if err != nil {
			// Log but continue with other findings
			continue
		}
		results = append(results, insight)
	}
	return results, nil
}

// mapFindingToInsight converts a research Finding to an Intermute Insight.
func mapFindingToInsight(finding research.Finding, project, specID string) intermute.Insight {
	return intermute.Insight{
		// ID is assigned by Intermute
		Project:   project,
		SpecID:    specID,
		Source:    finding.SourceType,
		Category:  mapCategoryFromTags(finding.Tags),
		Title:     finding.Title,
		Body:      finding.Summary,
		URL:       finding.Source,
		Score:     finding.Relevance,
		CreatedAt: finding.CollectedAt,
	}
}

// mapCategoryFromTags determines the insight category based on finding tags.
// Returns "competitive", "trends", "user", or "research" (default).
func mapCategoryFromTags(tags []string) string {
	for _, tag := range tags {
		lower := strings.ToLower(tag)
		switch {
		case strings.Contains(lower, "competitive"):
			return "competitive"
		case strings.Contains(lower, "trend"):
			return "trends"
		case strings.Contains(lower, "user"):
			return "user"
		}
	}
	return "research"
}
