package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAgentSelectorToggleAndSelect(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "codex"}, {Name: "claude"}})

	// F2 opens
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

func TestAgentSelectorQuickPick(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "codex"}, {Name: "claude"}})
	_, _ = s.Update(tea.KeyMsg{Type: tea.KeyF2})

	msg, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if sel, ok := msg.(AgentSelectedMsg); !ok || sel.Name != "claude" {
		t.Fatalf("expected selection of claude, got %#v", msg)
	}
}

func TestAgentSelectorRendersAsModelControl(t *testing.T) {
	sel := NewAgentSelector([]AgentOption{{Name: "codex"}})
	view := sel.View()
	if !strings.Contains(view, "Model") {
		t.Fatalf("expected model label")
	}
}
