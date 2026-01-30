package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestChatPanelHidesSystemRoleLabel(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(60, 20)
	panel.AddMessage("system", "Welcome")

	view := panel.View()
	if strings.Contains(view, "System:") {
		t.Fatalf("expected System label to be hidden, got %q", view)
	}
	if !strings.Contains(view, "Welcome") {
		t.Fatalf("expected system content to be rendered")
	}
}

func TestChatPanelIgnoresMouseEscapeSequences(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(60, 20)
	panel.Focus()
	panel.SetValue("hello")

	_, _ = panel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[<64;150;16M")})

	if panel.Value() != "hello" {
		t.Fatalf("expected mouse escape sequence to be ignored")
	}
}
