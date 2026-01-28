package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
)

func TestSprintViewRendersDraft(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Phase = arbiter.PhaseProblem
	state.Sections[arbiter.PhaseProblem].Content = "Test problem"
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftProposed
	view := NewSprintView(state)
	output := view.View()
	if output == "" {
		t.Error("expected non-empty view")
	}
	if !strings.Contains(output, "Test problem") {
		t.Error("expected view to contain draft content")
	}
	if !strings.Contains(output, "Accept") {
		t.Error("expected view to contain Accept option")
	}
}

func TestSprintViewHandlesAccept(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Phase = arbiter.PhaseProblem
	state.Sections[arbiter.PhaseProblem].Content = "Test problem"
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftProposed
	view := NewSprintView(state)
	newView, _ := view.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	sprintView := newView.(*SprintView)
	if sprintView.state.Sections[arbiter.PhaseProblem].Status != arbiter.DraftAccepted {
		t.Error("expected draft to be accepted")
	}
}

func TestSprintViewHandlesNavigation(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Phase = arbiter.PhaseProblem
	state.Sections[arbiter.PhaseProblem].Content = "Test problem"
	state.Sections[arbiter.PhaseProblem].Options = []string{"Option A", "Option B", "Option C"}
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftProposed
	view := NewSprintView(state)

	// Navigate down
	newView, _ := view.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	sv := newView.(*SprintView)
	if sv.optionIndex != 1 {
		t.Errorf("expected optionIndex 1 after j, got %d", sv.optionIndex)
	}

	// Navigate up
	newView, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	sv = newView.(*SprintView)
	if sv.optionIndex != 0 {
		t.Errorf("expected optionIndex 0 after k, got %d", sv.optionIndex)
	}
}

func TestSprintViewRendersConflicts(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Phase = arbiter.PhaseProblem
	state.Sections[arbiter.PhaseProblem].Content = "Test problem"
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftProposed
	state.Conflicts = []arbiter.Conflict{
		{
			Severity: arbiter.SeverityBlocker,
			Message:  "Feature contradicts scope",
			Sections: []arbiter.Phase{arbiter.PhaseFeaturesGoals, arbiter.PhaseScopeAssumptions},
		},
	}
	view := NewSprintView(state)
	output := view.View()
	if !strings.Contains(output, "Feature contradicts scope") {
		t.Error("expected view to contain conflict message")
	}
}

func TestSprintViewShowsConfidence(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Phase = arbiter.PhaseProblem
	state.Sections[arbiter.PhaseProblem].Content = "Test problem"
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftProposed
	state.Confidence = arbiter.ConfidenceScore{
		Completeness: 0.5,
		Consistency:  0.8,
		Specificity:  0.6,
		Research:     0.3,
		Assumptions:  0.7,
	}
	view := NewSprintView(state)
	output := view.View()
	// Confidence percentage should appear
	if !strings.Contains(output, "%") {
		t.Error("expected view to contain confidence percentage")
	}
}

func TestSprintViewSelectOption(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Phase = arbiter.PhaseProblem
	state.Sections[arbiter.PhaseProblem].Content = "Original"
	state.Sections[arbiter.PhaseProblem].Options = []string{"Alt A", "Alt B", "Alt C"}
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftProposed
	view := NewSprintView(state)

	newView, _ := view.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	sv := newView.(*SprintView)
	if sv.state.Sections[arbiter.PhaseProblem].Content != "Alt A" {
		t.Errorf("expected content 'Alt A', got %q", sv.state.Sections[arbiter.PhaseProblem].Content)
	}
}

func TestSprintViewResearchToggle(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Phase = arbiter.PhaseProblem
	state.Sections[arbiter.PhaseProblem].Content = "Test"
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftProposed
	state.Findings = []arbiter.ResearchFinding{
		{Title: "Competitor X", SourceType: "github", Relevance: 0.85, Tags: []string{"competitive"}},
	}
	view := NewSprintView(state)

	// Research panel hidden by default
	output := view.View()
	if strings.Contains(output, "Competitor X") {
		t.Error("research panel should be hidden by default")
	}

	// Toggle with 'r'
	newView, _ := view.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	sv := newView.(*SprintView)
	output = sv.View()
	if !strings.Contains(output, "Competitor X") {
		t.Error("expected research panel to show finding after toggle")
	}
	if !strings.Contains(output, "85%") {
		t.Error("expected relevance percentage")
	}

	// Toggle off
	newView, _ = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	sv = newView.(*SprintView)
	output = sv.View()
	if strings.Contains(output, "Competitor X") {
		t.Error("research panel should be hidden after second toggle")
	}
}

func TestSprintViewResearchDeepScanStatus(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Phase = arbiter.PhaseProblem
	state.Sections[arbiter.PhaseProblem].Content = "Test"
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftProposed
	state.DeepScan = arbiter.DeepScanState{Status: arbiter.DeepScanRunning, ScanID: "scan-1"}
	view := NewSprintView(state)
	view.showResearch = true

	output := view.View()
	if !strings.Contains(output, "Deep scan in progress") {
		t.Error("expected deep scan running status")
	}
}

func TestSprintViewShowsQuickScanResults(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Phase = arbiter.PhaseProblem
	state.Sections[arbiter.PhaseProblem].Content = "Test"
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftProposed
	state.ResearchCtx = &arbiter.QuickScanResult{
		Topic:   "search tools",
		Summary: "Found 3 relevant GitHub projects.",
		GitHubHits: []arbiter.GitHubFinding{
			{Name: "elastic/elasticsearch", Description: "Distributed search", Stars: 65000, URL: "https://github.com/elastic/elasticsearch"},
		},
		HNHits: []arbiter.HNFinding{
			{Title: "Why search is hard", Points: 200, Comments: 50, URL: "https://hn.example.com", Theme: "infrastructure"},
		},
	}
	view := NewSprintView(state)
	view.showResearch = true

	output := view.View()
	if !strings.Contains(output, "elasticsearch") {
		t.Error("expected GitHub finding in research panel")
	}
	if !strings.Contains(output, "65000") {
		t.Error("expected star count in research panel")
	}
	if !strings.Contains(output, "Why search is hard") {
		t.Error("expected HN finding in research panel")
	}
	if !strings.Contains(output, "Found 3 relevant") {
		t.Error("expected summary in research panel")
	}
}

func TestSprintViewQuit(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	view := NewSprintView(state)

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Error("expected quit command")
	}
}
