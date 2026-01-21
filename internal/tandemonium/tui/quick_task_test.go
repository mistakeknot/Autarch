package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFleetViewShowsQuickTaskCTAWhenEmpty(t *testing.T) {
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "No tasks yet") {
		t.Fatalf("expected empty-state message")
	}
	if !strings.Contains(out, "[n]") {
		t.Fatalf("expected quick task hint")
	}
}

func TestQuickTaskFlowCreatesTask(t *testing.T) {
	m := NewModel()
	called := ""
	m.QuickTaskCreator = func(raw string) (string, error) {
		called = raw
		return "TAND-123", nil
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(Model)
	if !m.QuickTaskMode {
		t.Fatalf("expected quick task mode")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Fix login")})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if called == "" {
		t.Fatalf("expected creator to be called")
	}
	if len(m.TaskList) != 1 || m.TaskList[0].ID != "TAND-123" {
		t.Fatalf("expected task list to include new task")
	}
	if !strings.Contains(m.Status, "TAND-123") {
		t.Fatalf("expected status to include task id")
	}
	if m.QuickTaskMode {
		t.Fatalf("expected quick task mode to close")
	}
}

func TestQuickTaskEscCancels(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.QuickTaskMode {
		t.Fatalf("expected quick task mode to close")
	}
}
