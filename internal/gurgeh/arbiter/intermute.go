package arbiter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/pollard/insights"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

// PollardToFindings converts Pollard insights to arbiter ResearchFindings.
func PollardToFindings(pollardInsights []*insights.Insight) []ResearchFinding {
	var findings []ResearchFinding
	for _, pi := range pollardInsights {
		for _, f := range pi.Findings {
			sourceURL := ""
			sourceType := ""
			if len(pi.Sources) > 0 {
				sourceURL = pi.Sources[0].URL
				sourceType = pi.Sources[0].Type
			}
			findings = append(findings, ResearchFinding{
				Title:      f.Title,
				Summary:    f.Description,
				Source:     sourceURL,
				SourceType: sourceType,
				Relevance:  relevanceToFloat(f.Relevance),
				Tags:       []string{string(pi.Category)},
			})
		}
	}
	return findings
}

func relevanceToFloat(r insights.Relevance) float64 {
	switch r {
	case insights.RelevanceHigh:
		return 0.9
	case insights.RelevanceMedium:
		return 0.6
	case insights.RelevanceLow:
		return 0.3
	default:
		return 0.5
	}
}

// ResearchProvider provides research integration for the Arbiter sprint.
// When nil, the orchestrator operates in no-research mode.
type ResearchProvider interface {
	CreateSpec(ctx context.Context, id, title string) (string, error)
	PublishInsight(ctx context.Context, specID string, finding ResearchFinding) (string, error)
	LinkInsight(ctx context.Context, insightID, specID string) error
	FetchLinkedInsights(ctx context.Context, specID string) ([]ResearchFinding, error)
	StartDeepScan(ctx context.Context, specID string) (string, error)     // returns scan job ID
	CheckDeepScan(ctx context.Context, scanID string) (bool, error)       // returns true when done
}

// ResearchBridge implements ResearchProvider by wrapping the Intermute client.
type ResearchBridge struct {
	client  *intermute.Client
	project string
}

// NewResearchBridge creates a new ResearchBridge.
func NewResearchBridge(intermuteURL, project string) (*ResearchBridge, error) {
	client, err := intermute.NewClient(&intermute.Config{
		URL:     intermuteURL,
		Project: project,
	})
	if err != nil {
		return nil, fmt.Errorf("creating intermute client: %w", err)
	}
	return &ResearchBridge{
		client:  client,
		project: project,
	}, nil
}

// sanitizeTitle strips newlines and truncates to 200 characters.
func sanitizeTitle(title string) string {
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", " ")
	title = strings.TrimSpace(title)
	if len(title) > 200 {
		title = title[:200]
	}
	return title
}

// CreateSpec creates an Intermute Spec from a sprint ID and title.
func (b *ResearchBridge) CreateSpec(ctx context.Context, id, title string) (string, error) {
	now := time.Now()
	spec, err := b.client.CreateSpec(ctx, intermute.Spec{
		Project:   b.project,
		Title:     sanitizeTitle(title),
		Status:    intermute.SpecStatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return "", fmt.Errorf("creating spec: %w", err)
	}
	return spec.ID, nil
}

// PublishInsight creates an Intermute Insight linked to a spec.
func (b *ResearchBridge) PublishInsight(ctx context.Context, specID string, finding ResearchFinding) (string, error) {
	insight, err := b.client.CreateInsight(ctx, intermute.Insight{
		Project:  b.project,
		SpecID:   specID,
		Source:   finding.SourceType,
		Category: strings.Join(finding.Tags, ","),
		Title:    sanitizeTitle(finding.Title),
		Body:     finding.Summary,
		URL:      finding.Source,
		Score:    finding.Relevance,
	})
	if err != nil {
		return "", fmt.Errorf("publishing insight: %w", err)
	}
	return insight.ID, nil
}

// LinkInsight links an insight to a spec via Intermute.
func (b *ResearchBridge) LinkInsight(ctx context.Context, insightID, specID string) error {
	if err := b.client.LinkInsightToSpec(ctx, insightID, specID); err != nil {
		return fmt.Errorf("linking insight to spec: %w", err)
	}
	return nil
}

// StartDeepScan kicks off an async deep scan via Intermute.
func (b *ResearchBridge) StartDeepScan(ctx context.Context, specID string) (string, error) {
	scanID, err := b.client.StartDeepScan(ctx, specID)
	if err != nil {
		return "", fmt.Errorf("starting deep scan: %w", err)
	}
	return scanID, nil
}

// CheckDeepScan checks whether a deep scan has completed.
func (b *ResearchBridge) CheckDeepScan(ctx context.Context, scanID string) (bool, error) {
	done, err := b.client.CheckDeepScan(ctx, scanID)
	if err != nil {
		return false, fmt.Errorf("checking deep scan: %w", err)
	}
	return done, nil
}

// FetchLinkedInsights retrieves insights linked to a spec and maps them to ResearchFindings.
func (b *ResearchBridge) FetchLinkedInsights(ctx context.Context, specID string) ([]ResearchFinding, error) {
	insights, err := b.client.ListInsights(ctx, specID, "")
	if err != nil {
		return nil, fmt.Errorf("fetching linked insights: %w", err)
	}
	findings := make([]ResearchFinding, len(insights))
	for i, in := range insights {
		findings[i] = ResearchFinding{
			ID:         in.ID,
			Title:      in.Title,
			Summary:    in.Body,
			Source:     in.URL,
			SourceType: in.Source,
			Relevance:  in.Score,
			Tags:       []string{in.Category},
		}
	}
	return findings, nil
}
