package arbiter_test

import (
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestExportToSpec_RoundTrip(t *testing.T) {
	original := &specs.Spec{
		Title:   "AI-Powered Search",
		Summary: "Users can't find relevant research papers quickly",
		UserStory: specs.UserStory{
			Text: "As a researcher, I want to find papers by concept",
		},
		Goals: []specs.Goal{
			{Description: "Reduce search time by 50%"},
		},
		NonGoals: []specs.NonGoal{
			{Description: "Full-text indexing of PDFs"},
		},
		Assumptions: []specs.Assumption{
			{Description: "Users have stable internet"},
		},
		Requirements: []string{
			"Must support boolean queries",
			"Must return results in under 2 seconds",
		},
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				Title:    "First Search",
				Priority: "high",
				Steps:    []string{"Open app", "Type query", "View results"},
			},
		},
		Acceptance: []specs.AcceptanceCriterion{
			{ID: "AC-001", Description: "Search returns results"},
		},
	}

	// Spec → SprintState
	state := arbiter.MigrateFromSpec(original, "/tmp/test")

	// SprintState → Spec
	exported, err := arbiter.ExportToSpec(state)
	if err != nil {
		t.Fatalf("ExportToSpec failed: %v", err)
	}

	// Verify round-trip preserves key fields
	if exported.Title != original.Title {
		t.Errorf("Title: got %q, want %q", exported.Title, original.Title)
	}
	if exported.Summary != original.Summary {
		t.Errorf("Summary: got %q, want %q", exported.Summary, original.Summary)
	}
	if exported.UserStory.Text != original.UserStory.Text {
		t.Errorf("UserStory: got %q, want %q", exported.UserStory.Text, original.UserStory.Text)
	}
	if len(exported.Requirements) != len(original.Requirements) {
		t.Errorf("Requirements count: got %d, want %d", len(exported.Requirements), len(original.Requirements))
	} else {
		for i, r := range exported.Requirements {
			if r != original.Requirements[i] {
				t.Errorf("Requirement[%d]: got %q, want %q", i, r, original.Requirements[i])
			}
		}
	}
	if len(exported.Acceptance) == 0 {
		t.Error("expected acceptance criteria in export")
	}
	if len(exported.CriticalUserJourneys) == 0 {
		t.Error("expected CUJs in export")
	} else if exported.CriticalUserJourneys[0].Title != "First Search" {
		t.Errorf("CUJ title: got %q, want %q", exported.CriticalUserJourneys[0].Title, "First Search")
	}
}

func TestExportToSpec_NilSections(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	exported, err := arbiter.ExportToSpec(state)
	if err != nil {
		t.Fatalf("ExportToSpec failed on empty state: %v", err)
	}
	if exported.Title != "" {
		t.Errorf("expected empty title, got %q", exported.Title)
	}
}

func TestExportToSpec_WithFindings(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Findings = []arbiter.ResearchFinding{
		{ID: "f1", Summary: "Competitor does X", Relevance: 0.9},
	}
	exported, err := arbiter.ExportToSpec(state)
	if err != nil {
		t.Fatalf("ExportToSpec failed: %v", err)
	}
	if len(exported.MarketResearch) != 1 {
		t.Fatalf("expected 1 market research item, got %d", len(exported.MarketResearch))
	}
	if exported.MarketResearch[0].Confidence != "high" {
		t.Errorf("expected high confidence for 0.9 relevance, got %q", exported.MarketResearch[0].Confidence)
	}
}
