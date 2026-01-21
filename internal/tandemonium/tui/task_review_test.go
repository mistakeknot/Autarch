package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestReviewActionEntersReviewView(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewFleet
	m.TaskList = []TaskItem{{ID: "T1", Title: "One", Status: "review"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updated.(Model)
	if m.ViewMode != ViewReview {
		t.Fatalf("expected review view")
	}
}
