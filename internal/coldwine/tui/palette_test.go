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
	if !strings.Contains(stripANSI(got.View()), "Command Palette") {
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
	if !strings.Contains(stripANSI(got.View()), "Settings") {
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
	if !strings.Contains(stripANSI(got.View()), "Help") {
		t.Fatal("expected help view")
	}
}
