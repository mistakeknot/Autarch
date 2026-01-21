package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCtrlCDoubleQuit(t *testing.T) {
	m := NewModel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	current := now
	m.Now = func() time.Time { return current }
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatalf("did not expect quit on first ctrl+c")
		}
	}
	current = now.Add(ctrlCWindow / 2)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected quit command on second ctrl+c")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg")
	}
}

func TestCtrlCOutsideWindowDoesNotQuit(t *testing.T) {
	m := NewModel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	current := now
	m.Now = func() time.Time { return current }
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)
	current = now.Add(ctrlCWindow * 2)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatalf("did not expect quit when outside window")
		}
	}
}
