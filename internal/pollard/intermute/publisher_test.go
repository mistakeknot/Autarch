package intermute

import (
	"context"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

// mockInsightClient implements InsightCreator for testing
type mockInsightClient struct {
	insights []intermute.Insight
	createErr error
}

func (m *mockInsightClient) CreateInsight(ctx context.Context, insight intermute.Insight) (intermute.Insight, error) {
	if m.createErr != nil {
		return intermute.Insight{}, m.createErr
	}
	insight.ID = "insight-" + insight.Title[:8]
	m.insights = append(m.insights, insight)
	return insight, nil
}

func TestPublisher_PublishFinding(t *testing.T) {
	mock := &mockInsightClient{insights: make([]intermute.Insight, 0)}
	pub := NewPublisher(mock, "autarch")

	finding := research.Finding{
		ID:          "finding-001",
		Title:       "Competitor launched new API",
		Summary:     "Company X released a REST API for their core product",
		Source:      "https://example.com/api-announcement",
		SourceType:  "github-scout",
		Relevance:   0.85,
		Tags:        []string{"competitive", "api"},
		CollectedAt: time.Now(),
	}

	insight, err := pub.PublishFinding(context.Background(), finding)
	if err != nil {
		t.Fatalf("PublishFinding failed: %v", err)
	}

	if insight.ID == "" {
		t.Error("expected non-empty insight ID")
	}
	if len(mock.insights) != 1 {
		t.Fatalf("expected 1 insight, got %d", len(mock.insights))
	}

	created := mock.insights[0]
	if created.Title != "Competitor launched new API" {
		t.Errorf("expected title 'Competitor launched new API', got %s", created.Title)
	}
	if created.Source != "github-scout" {
		t.Errorf("expected source 'github-scout', got %s", created.Source)
	}
	if created.Score != 0.85 {
		t.Errorf("expected score 0.85, got %f", created.Score)
	}
	if created.URL != "https://example.com/api-announcement" {
		t.Errorf("expected URL to be set, got %s", created.URL)
	}
}

func TestPublisher_PublishFindings(t *testing.T) {
	mock := &mockInsightClient{insights: make([]intermute.Insight, 0)}
	pub := NewPublisher(mock, "autarch")

	findings := []research.Finding{
		{
			ID:         "f1",
			Title:      "Finding One",
			Summary:    "Summary one",
			SourceType: "arxiv",
			Relevance:  0.9,
		},
		{
			ID:         "f2",
			Title:      "Finding Two",
			Summary:    "Summary two",
			SourceType: "hackernews",
			Relevance:  0.7,
		},
	}

	results, err := pub.PublishFindings(context.Background(), findings)
	if err != nil {
		t.Fatalf("PublishFindings failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	if len(mock.insights) != 2 {
		t.Errorf("expected 2 insights created, got %d", len(mock.insights))
	}
}

func TestPublisher_MapFindingCategoryFromTags(t *testing.T) {
	testCases := []struct {
		tags     []string
		expected string
	}{
		{[]string{"competitive", "api"}, "competitive"},
		{[]string{"trends", "emerging"}, "trends"},
		{[]string{"user", "research"}, "user"},
		{[]string{"market", "analysis"}, "research"}, // default
		{[]string{}, "research"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := mapCategoryFromTags(tc.tags)
			if result != tc.expected {
				t.Errorf("mapCategoryFromTags(%v) = %s, want %s", tc.tags, result, tc.expected)
			}
		})
	}
}

func TestPublisher_NilClientGracefulDegradation(t *testing.T) {
	pub := NewPublisher(nil, "autarch")

	finding := research.Finding{
		ID:    "f1",
		Title: "Test",
	}

	// Should return empty insight but no error
	insight, err := pub.PublishFinding(context.Background(), finding)
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if insight.ID != "" {
		t.Error("expected empty insight for nil client")
	}
}

func TestPublisher_WithSpecID(t *testing.T) {
	mock := &mockInsightClient{insights: make([]intermute.Insight, 0)}
	pub := NewPublisher(mock, "autarch").WithSpecID("spec-123")

	finding := research.Finding{
		ID:    "f1",
		Title: "Test Finding",
	}

	_, err := pub.PublishFinding(context.Background(), finding)
	if err != nil {
		t.Fatalf("PublishFinding failed: %v", err)
	}

	if mock.insights[0].SpecID != "spec-123" {
		t.Errorf("expected SpecID 'spec-123', got %s", mock.insights[0].SpecID)
	}
}
