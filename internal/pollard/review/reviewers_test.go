package review

import (
	"context"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/pollard/insights"
)

func TestRunParallelReview(t *testing.T) {
	insight := &insights.Insight{
		ID:          "TEST-001",
		Title:       "Test Insight",
		Category:    insights.CategoryCompetitive,
		CollectedAt: time.Now(),
		Sources: []insights.Source{
			{URL: "https://github.com/example/repo", Type: "github"},
		},
		Findings: []insights.Finding{
			{
				Title:       "Key Finding",
				Relevance:   insights.RelevanceHigh,
				Description: "This is a detailed finding about the competitive landscape that provides actionable insights.",
				Evidence:    []string{"screenshot.png"},
			},
		},
		Recommendations: []insights.Recommendation{
			{
				FeatureHint: "Add similar feature",
				Priority:    "p1",
				Rationale:   "Competitors have this, we should too",
			},
		},
	}

	reviewers := DefaultReviewers()
	result, err := RunParallelReview(context.Background(), insight, reviewers)
	if err != nil {
		t.Fatalf("RunParallelReview failed: %v", err)
	}

	if result.InsightID != "TEST-001" {
		t.Errorf("InsightID = %q, want %q", result.InsightID, "TEST-001")
	}

	if len(result.Results) != 3 {
		t.Errorf("got %d results, want 3 (one per reviewer)", len(result.Results))
	}

	// A well-formed insight should pass
	if !result.Passed {
		t.Errorf("expected insight to pass review, but it failed with score %.2f", result.OverallScore)
		for _, r := range result.Results {
			t.Logf("  %s: score=%.2f, issues=%d", r.Reviewer, r.Score, len(r.Issues))
			for _, issue := range r.Issues {
				t.Logf("    - [%s] %s: %s", issue.Severity, issue.Category, issue.Description)
			}
		}
	}
}

func TestSourceCredibilityReviewer_NoSources(t *testing.T) {
	insight := &insights.Insight{
		ID:       "TEST-002",
		Title:    "Insight Without Sources",
		Sources:  []insights.Source{},
		Findings: []insights.Finding{},
	}

	reviewer := NewSourceCredibilityReviewer()
	result, err := reviewer.Review(context.Background(), insight)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Should have an error about missing sources
	hasError := false
	for _, issue := range result.Issues {
		if issue.Severity == SeverityError && issue.Category == "missing-sources" {
			hasError = true
			break
		}
	}

	if !hasError {
		t.Error("expected error about missing sources")
	}

	if result.Score > 0.6 {
		t.Errorf("score %.2f should be <= 0.6 for insight without sources", result.Score)
	}
}

func TestRelevanceReviewer_VagueFindings(t *testing.T) {
	insight := &insights.Insight{
		ID:    "TEST-003",
		Title: "Insight With Vague Findings",
		Findings: []insights.Finding{
			{
				Title:       "Something",
				Relevance:   insights.RelevanceHigh,
				Description: "This is interesting stuff.",
			},
		},
	}

	reviewer := NewRelevanceReviewer()
	result, err := reviewer.Review(context.Background(), insight)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Should flag vague description
	hasWarning := false
	for _, issue := range result.Issues {
		if issue.Category == "vague-finding" {
			hasWarning = true
			break
		}
	}

	if !hasWarning {
		t.Error("expected warning about vague finding")
	}
}

func TestContradictionDetector_InternalContradiction(t *testing.T) {
	insight := &insights.Insight{
		ID:    "TEST-004",
		Title: "Insight With Contradictions",
		Findings: []insights.Finding{
			{
				Title:       "System Speed",
				Relevance:   insights.RelevanceHigh,
				Description: "The system speed is always better than competitors",
			},
			{
				Title:       "System Speed",
				Relevance:   insights.RelevanceHigh,
				Description: "The system speed is never better than competitors in benchmarks",
			},
		},
	}

	reviewer := NewContradictionDetector()
	result, err := reviewer.Review(context.Background(), insight)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Should detect contradiction (same subject with always vs never)
	hasContradiction := false
	for _, issue := range result.Issues {
		if issue.Category == "internal-contradiction" {
			hasContradiction = true
			break
		}
	}

	if !hasContradiction {
		t.Error("expected warning about internal contradiction")
	}
}
