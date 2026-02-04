package arbiter

import (
	"context"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/scan"
)

func TestGenerateDraftFromContext(t *testing.T) {
	gen := NewGenerator()
	ctx := context.Background()
	projectCtx := &ProjectContext{
		HasReadme:      true,
		ReadmeSnippet:  "A CLI tool for managing reading lists",
		HasPackageJSON: false,
	}
	draft, err := gen.GenerateDraft(ctx, PhaseProblem, projectCtx, "", nil)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if draft.Content == "" {
		t.Error("expected non-empty draft content")
	}
	if len(draft.Options) < 2 {
		t.Errorf("expected at least 2 options, got %d", len(draft.Options))
	}
	if draft.Status != DraftProposed {
		t.Errorf("expected DraftProposed status, got %d", draft.Status)
	}
	if !strings.Contains(draft.Content, "reading lists") {
		t.Error("expected draft to reference project context")
	}
}

func TestGenerateDraftFromUserInput(t *testing.T) {
	gen := NewGenerator()
	ctx := context.Background()
	draft, err := gen.GenerateDraft(ctx, PhaseProblem, nil, "I want to build a habit tracker for developers", nil)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if draft.Content == "" {
		t.Error("expected non-empty draft content")
	}
	if draft.Status != DraftProposed {
		t.Errorf("expected DraftProposed status, got %d", draft.Status)
	}
}

func TestGenerateDraftFromInputWithBecause(t *testing.T) {
	gen := NewGenerator()
	ctx := context.Background()
	draft, err := gen.GenerateDraft(ctx, PhaseProblem, nil, "I want to build a task manager because existing tools are too complex", nil)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if !strings.Contains(draft.Content, "existing tools are too complex") {
		t.Error("expected draft to extract reason after 'because'")
	}
}

func TestGenerateDraftFallback(t *testing.T) {
	gen := NewGenerator()
	ctx := context.Background()
	draft, err := gen.GenerateDraft(ctx, PhaseProblem, nil, "", nil)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if draft.Content == "" {
		t.Error("expected non-empty fallback draft")
	}
}

func TestGenerateAllPhases(t *testing.T) {
	gen := NewGenerator()
	ctx := context.Background()
	for _, phase := range AllPhases() {
		draft, err := gen.GenerateDraft(ctx, phase, nil, "test input", nil)
		if err != nil {
			t.Fatalf("phase %s: generate failed: %v", phase, err)
		}
		if draft.Content == "" {
			t.Errorf("phase %s: expected non-empty content", phase)
		}
		if len(draft.Options) < 2 {
			t.Errorf("phase %s: expected at least 2 options, got %d", phase, len(draft.Options))
		}
		if draft.Status != DraftProposed {
			t.Errorf("phase %s: expected DraftProposed", phase)
		}
	}
}

func TestGenerateUnknownPhase(t *testing.T) {
	gen := NewGenerator()
	ctx := context.Background()
	_, err := gen.GenerateDraft(ctx, Phase(99), nil, "test", nil)
	if err == nil {
		t.Error("expected error for unknown phase")
	}
}

func TestGenerateDraftWithScanEvidence(t *testing.T) {
	gen := NewGenerator()
	ctx := context.Background()
	pd := &scan.PhaseData{
		Summary: "Project manages reading lists",
		Evidence: []scan.EvidenceItem{
			{Type: "readme", FilePath: "README.md", Quote: "A CLI for curating reading lists", Confidence: 0.9},
		},
		ResolvedQuestions: []scan.ResolvedQuestion{
			{Question: "Who is the target user?", Answer: "Developers who read technical content"},
		},
	}
	draft, err := gen.GenerateDraft(ctx, PhaseVision, nil, "reading list tool", nil, pd)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if !strings.Contains(draft.Preamble, "<evidence>") {
		t.Error("expected evidence to be wrapped in <evidence> delimiters in Preamble")
	}
	if !strings.Contains(draft.Preamble, "README.md") {
		t.Error("expected evidence file path in Preamble")
	}
	if !strings.Contains(draft.Preamble, "Resolved Questions") {
		t.Error("expected resolved questions section in Preamble")
	}
}

func TestGenerateDraftWithNilScanData(t *testing.T) {
	gen := NewGenerator()
	ctx := context.Background()
	// Passing nil scan data should produce same output as no scan data
	draft, err := gen.GenerateDraft(ctx, PhaseVision, nil, "test", nil, (*scan.PhaseData)(nil))
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if strings.Contains(draft.Content, "<evidence>") {
		t.Error("nil scan data should not produce evidence block")
	}
}

func TestGenerateDraftFallbackNoContext(t *testing.T) {
	gen := NewGenerator()
	ctx := context.Background()
	draft, err := gen.GenerateDraft(ctx, PhaseProblem, nil, "", nil)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if draft.Content == "" {
		t.Error("expected fallback content")
	}
	if len(draft.Options) < 2 {
		t.Errorf("expected options, got %d", len(draft.Options))
	}
}

func TestGenerateDraftWithResearchFindings(t *testing.T) {
	gen := NewGenerator()
	ctx := context.Background()
	findings := []ResearchFinding{
		{Title: "Competing Tool X", Summary: "Tool X solves reading lists with AI", SourceType: "github", Relevance: 0.85},
		{Title: "Academic Paper Y", Summary: "Study on reading habit formation", SourceType: "arxiv", Relevance: 0.7},
	}
	draft, err := gen.GenerateDraft(ctx, PhaseVision, nil, "reading list tool", findings)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if !strings.Contains(draft.Preamble, "<research>") {
		t.Error("expected research findings wrapped in <research> delimiters")
	}
	if !strings.Contains(draft.Preamble, "Competing Tool X") {
		t.Error("expected finding title in preamble")
	}
	if !strings.Contains(draft.Preamble, "85%") {
		t.Error("expected relevance percentage in preamble")
	}
	if !strings.Contains(draft.Preamble, "arxiv") {
		t.Error("expected source type in preamble")
	}
}

func TestFormatResearchContextEmpty(t *testing.T) {
	gen := NewGenerator()
	result := gen.formatResearchContext(nil)
	if result != "" {
		t.Error("expected empty string for nil findings")
	}
	result = gen.formatResearchContext([]ResearchFinding{})
	if result != "" {
		t.Error("expected empty string for empty findings")
	}
}
