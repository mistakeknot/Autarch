package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

func TestSearchOverlayFilters(t *testing.T) {
	overlay := NewSearchOverlay()
	overlay.SetItems([]specs.Summary{{ID: "PRD-1", Title: "Alpha"}, {ID: "PRD-2", Title: "Beta"}})
	overlay.Show()
	overlay, _ = overlay.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if len(overlay.Results()) == 0 {
		t.Fatalf("expected results")
	}
}
