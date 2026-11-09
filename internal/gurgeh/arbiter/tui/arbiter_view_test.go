package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestArbiterViewCtrlCQuits(t *testing.T) {
	view := NewArbiterView("/tmp/test", nil)
	view.state = arbiter.NewSprintState("/tmp/test")

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg")
	}
}

func TestArbiterViewQDoesNotQuit(t *testing.T) {
	view := NewArbiterView("/tmp/test", nil)
	view.state = arbiter.NewSprintState("/tmp/test")

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatalf("did not expect quit command")
		}
	}
}

func TestArbiterSidebarUsesInterviewSteps(t *testing.T) {
	view := NewArbiterView("/tmp/test", nil)
	items := view.SidebarItems()

	if len(items) != 8 {
		t.Fatalf("expected 8 sidebar items, got %d", len(items))
	}
	if items[0].Label != "Vision" ||
		items[1].Label != "Problem" ||
		items[2].Label != "Users" ||
		items[3].Label != "Features + Goals" ||
		items[4].Label != "Requirements" ||
		items[5].Label != "Scope + Assumptions" ||
		items[6].Label != "Critical User Journeys" ||
		items[7].Label != "Acceptance Criteria" {
		t.Fatalf("unexpected sidebar labels: %#v", items)
	}
}

func TestDeriveResearchQueryPrefersVision(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Sections[arbiter.PhaseVision].Content = "vision text"
	state.Sections[arbiter.PhaseProblem].Content = "problem text"

	got := deriveResearchQuery(state)
	if got != "vision text" {
		t.Fatalf("expected vision query, got %q", got)
	}
}

func TestDeriveResearchQueryFallsBackToProblem(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Sections[arbiter.PhaseProblem].Content = "problem text"

	got := deriveResearchQuery(state)
	if got != "problem text" {
		t.Fatalf("expected problem query, got %q", got)
	}
}

func TestPersistExportedSpecWritesSpecFile(t *testing.T) {
	root := t.TempDir()
	spec := &specs.Spec{
		ID:    "PRD-TEST",
		Title: "Test",
	}

	if err := persistExportedSpec(root, spec); err != nil {
		t.Fatalf("persistExportedSpec failed: %v", err)
	}

	path := filepath.Join(root, ".gurgeh", "specs", "PRD-TEST.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected persisted spec file: %v", err)
	}
}

func TestArbiterViewBlurCancelsContext(t *testing.T) {
	view := NewArbiterView("/tmp/test", nil)
	if view.ctx == nil {
		t.Fatalf("expected view context to be initialized")
	}
	done := view.ctx.Done()

	view.Blur()

	select {
	case <-done:
	default:
		t.Fatalf("expected context to be canceled on blur")
	}

	// Repeated cleanup should be safe.
	view.Blur()
}
