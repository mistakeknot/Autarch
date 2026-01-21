package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestCoordSelectionMoves(t *testing.T) {
	m := NewModel()
	m.RightTab = RightTabCoord
	m.FocusPane = FocusDetail
	m.CoordRecipient = "alice"
	m.CoordInbox = []storage.MessageDelivery{
		{Message: storage.Message{Sender: "bob", Subject: "One"}},
		{Message: storage.Message{Sender: "carol", Subject: "Two"}},
	}
	m.CoordLocks = []storage.Reservation{}
	m.CoordSelected = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	next := updated.(Model)
	if next.CoordSelected != 1 {
		t.Fatalf("expected selection 1, got %d", next.CoordSelected)
	}
}
