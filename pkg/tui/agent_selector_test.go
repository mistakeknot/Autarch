package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestAgentSelectorToggleAndSelect(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "codex"}, {Name: "claude"}})

	// ctrl+g opens (MacBook-friendly alternative to F2)
	_, _ = s.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	if !s.Open {
		t.Fatal("expected selector open after ctrl+g")
	}

	// Close it and test F2 still works
	_, _ = s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_, _ = s.Update(tea.KeyMsg{Type: tea.KeyF2})
	if !s.Open {
		t.Fatal("expected selector open after F2")
	}

	// Down + enter selects second option
	_, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	msg, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel, ok := msg.(AgentSelectedMsg); !ok || sel.Name != "claude" {
		t.Fatalf("expected selection of claude, got %#v", msg)
	}
	if s.Open {
		t.Fatal("expected selector closed after selection")
	}
}

func TestAgentSelectorIgnoresNumericPick(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "codex"}, {Name: "claude"}})
	_, _ = s.Update(tea.KeyMsg{Type: tea.KeyF2})

	msg, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if msg != nil {
		t.Fatalf("expected no selection on numeric key, got %#v", msg)
	}
}

func TestAgentSelectorRendersAsModelControl(t *testing.T) {
	sel := NewAgentSelector([]AgentOption{{Name: "codex"}})
	view := sel.View()
	if !strings.Contains(view, "Model") {
		t.Fatalf("expected model label")
	}
}

func TestAgentSelectorClosedLabelFitsWidth(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "\x1b[31m模型\x1b[0m", Source: "project config"}})
	for _, width := range []int{0, 1, 12, 30} {
		if got := lipgloss.Width(s.View(width)); got > width {
			t.Fatalf("width %d exceeds allocation %d", got, width)
		}
	}
}

func TestChatPanelShowsAgentPreferenceSource(t *testing.T) {
	p := NewChatPanel()
	p.SetSize(60, 20)
	p.SetAgentSelector(NewAgentSelector([]AgentOption{{Name: "codex", Source: "project config"}}))
	if !strings.Contains(p.View(), "project config") {
		t.Fatal("closed composer lost preference source")
	}
}
