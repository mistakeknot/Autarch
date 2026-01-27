package arbiter_test

import (
	"context"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
)

// TestIntegration_FullSprintWithResearch exercises the complete sprint lifecycle
// with a research provider, verifying Intermute spec creation, insight publishing,
// quick scan integration, confidence scoring, and phase advancement.
func TestIntegration_FullSprintWithResearch(t *testing.T) {
	provider := &testResearchProvider{
		specID: "spec-integration-1",
		findings: []arbiter.ResearchFinding{
			{Title: "Prior Art A", SourceType: "github", Relevance: 0.8, Tags: []string{"competitive"}},
			{Title: "Prior Art B", SourceType: "hackernews", Relevance: 0.6, Tags: []string{"trends"}},
		},
	}
	o := arbiter.NewOrchestratorWithResearch("/tmp/integration-test", provider)
	ctx := context.Background()

	// Start sprint — should create Intermute Spec
	state, err := o.Start(ctx, "Build an AI-powered code review tool")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if state.SpecID != "spec-integration-1" {
		t.Errorf("expected SpecID=spec-integration-1, got %q", state.SpecID)
	}
	if state.ID == "" {
		t.Error("expected non-empty sprint ID")
	}
	if state.Phase != arbiter.PhaseVision {
		t.Errorf("expected PhaseVision, got %v", state.Phase)
	}

	// Walk through all phases
	phases := arbiter.AllPhases()
	for i := 1; i < len(phases); i++ {
		state = o.AcceptDraft(state)
		state, err = o.Advance(ctx, state)
		if err != nil {
			t.Fatalf("Advance to phase %d (%v) failed: %v", i, phases[i], err)
		}
		if state.Phase != phases[i] {
			t.Errorf("expected phase %v, got %v", phases[i], state.Phase)
		}
		// Every phase should have a section with content
		section := state.Sections[state.Phase]
		if section == nil {
			t.Fatalf("section nil for phase %v", state.Phase)
		}
		if section.Content == "" {
			t.Errorf("empty content for phase %v", state.Phase)
		}
	}

	// Quick scan should have published an insight (at FeaturesGoals transition)
	if len(provider.published) == 0 {
		t.Error("expected at least one published insight from quick scan")
	}

	// Confidence should be non-zero (we have accepted sections + research findings)
	total := state.Confidence.Total()
	if total <= 0 {
		t.Errorf("expected positive confidence, got %f", total)
	}

	// Research score should reflect the mock findings
	if state.Confidence.Research <= 0 {
		t.Errorf("expected positive research confidence, got %f", state.Confidence.Research)
	}

	// Handoff options should include research and tasks
	options := o.GetHandoffOptions(state)
	if len(options) == 0 {
		t.Error("expected handoff options")
	}
	var hasResearch, hasTasks bool
	for _, opt := range options {
		if opt.ID == "research" {
			hasResearch = true
		}
		if opt.ID == "tasks" {
			hasTasks = true
		}
	}
	if !hasResearch {
		t.Error("expected 'research' handoff option")
	}
	if !hasTasks {
		t.Error("expected 'tasks' handoff option")
	}
}

// TestIntegration_StartWithResearch_ImportsFindings verifies that
// StartWithResearch publishes Pollard findings as Intermute Insights
// and populates sprint state correctly.
func TestIntegration_StartWithResearch_ImportsFindings(t *testing.T) {
	provider := &testResearchProvider{specID: "spec-import-1"}
	o := arbiter.NewOrchestratorWithResearch("/tmp/integration-test", provider)

	pollardFindings := []arbiter.ResearchFinding{
		{Title: "Elasticsearch", SourceType: "github", Relevance: 0.9, Tags: []string{"search"}},
		{Title: "Typesense", SourceType: "github", Relevance: 0.85, Tags: []string{"search"}},
		{Title: "Search is hard", SourceType: "hackernews", Relevance: 0.7, Tags: []string{"commentary"}},
	}

	state, err := o.StartWithResearch(context.Background(), "search tool", pollardFindings)
	if err != nil {
		t.Fatalf("StartWithResearch failed: %v", err)
	}

	// All 3 findings should have been published
	if len(provider.published) != 3 {
		t.Errorf("expected 3 published insights, got %d", len(provider.published))
	}

	// State should have findings (from FetchLinkedInsights)
	if len(state.Findings) == 0 {
		t.Error("expected findings on state after import")
	}
}

// TestIntegration_NoResearchProvider_StillWorks verifies the sprint
// works correctly without any research provider (nil = no-research mode).
func TestIntegration_NoResearchProvider_StillWorks(t *testing.T) {
	o := arbiter.NewOrchestrator("/tmp/no-research-test")
	ctx := context.Background()

	state, err := o.Start(ctx, "Simple project with no research")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if state.SpecID != "" {
		t.Errorf("expected empty SpecID, got %q", state.SpecID)
	}

	// Should still advance through phases without error
	state = o.AcceptDraft(state)
	state, err = o.Advance(ctx, state)
	if err != nil {
		t.Fatalf("Advance failed without research: %v", err)
	}
	if state.Phase != arbiter.PhaseProblem {
		t.Errorf("expected PhaseProblem, got %v", state.Phase)
	}

	// Confidence.Research should be 0 with no findings
	if state.Confidence.Research != 0 {
		t.Errorf("expected 0 research confidence without provider, got %f", state.Confidence.Research)
	}
}

// TestIntegration_ConfidenceReflectsResearchQuality verifies that
// the confidence scoring formula correctly weights findings.
func TestIntegration_ConfidenceReflectsResearchQuality(t *testing.T) {
	// Provider with diverse, high-relevance findings
	richProvider := &testResearchProvider{
		specID: "spec-rich",
		findings: []arbiter.ResearchFinding{
			{Title: "F1", SourceType: "github", Relevance: 0.9, Tags: []string{"a"}},
			{Title: "F2", SourceType: "hackernews", Relevance: 0.8, Tags: []string{"b"}},
			{Title: "F3", SourceType: "arxiv", Relevance: 0.7, Tags: []string{"c"}},
		},
	}
	// Provider with no findings
	emptyProvider := &testResearchProvider{
		specID:   "spec-empty",
		findings: []arbiter.ResearchFinding{},
	}

	ctx := context.Background()

	// Advance both sprints through Vision → Problem → Users → FeaturesGoals
	// (quick scan + confidence update happen during Advance)
	advanceN := func(o *arbiter.Orchestrator, input string, n int) *arbiter.SprintState {
		s, err := o.Start(ctx, input)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		for i := 0; i < n; i++ {
			s = o.AcceptDraft(s)
			s, err = o.Advance(ctx, s)
			if err != nil {
				t.Fatalf("Advance %d failed: %v", i, err)
			}
		}
		return s
	}

	richOrch := arbiter.NewOrchestratorWithResearch("/tmp/rich", richProvider)
	richState := advanceN(richOrch, "rich research project", 4) // past FeaturesGoals so confidence sees findings

	emptyOrch := arbiter.NewOrchestratorWithResearch("/tmp/empty", emptyProvider)
	emptyState := advanceN(emptyOrch, "empty research project", 4)

	// Rich findings should produce higher research confidence
	if richState.Confidence.Research <= emptyState.Confidence.Research {
		t.Errorf("expected rich research (%f) > empty research (%f)",
			richState.Confidence.Research, emptyState.Confidence.Research)
	}
}
