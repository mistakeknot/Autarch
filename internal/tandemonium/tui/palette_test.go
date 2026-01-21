package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPaletteOpensOnCtrlK(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	got := updated.(Model)
	if !got.PaletteOpen {
		t.Fatal("expected palette to open")
	}
	if !strings.Contains(got.View(), "COMMAND PALETTE") {
		t.Fatal("expected palette view")
	}
}

func TestSettingsOpensOnComma(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")})
	got := updated.(Model)
	if !got.SettingsOpen {
		t.Fatal("expected settings to open")
	}
	if !strings.Contains(got.View(), "SETTINGS") {
		t.Fatal("expected settings view")
	}
}

func TestHelpOpensOnQuestion(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	got := updated.(Model)
	if !got.HelpOpen {
		t.Fatal("expected help to open")
	}
	if !strings.Contains(got.View(), "HELP") {
		t.Fatal("expected help view")
	}
}
