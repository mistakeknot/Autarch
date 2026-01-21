package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestStartTaskCallsStarter(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewFleet
	m.TaskList = []TaskItem{{ID: "T1", Title: "One", Status: "todo"}}
	called := false
	m.TaskStarter = func(id string) error {
		called = true
		if id != "T1" {
			t.Fatalf("expected T1, got %s", id)
		}
		return nil
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated.(Model)
	if !called {
		t.Fatalf("expected starter to be called")
	}
}

func TestStopTaskCallsStopper(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewFleet
	m.TaskList = []TaskItem{{ID: "T1", Title: "One", Status: "in_progress"}}
	called := false
	m.TaskStopper = func(id string) error {
		called = true
		if id != "T1" {
			t.Fatalf("expected T1, got %s", id)
		}
		return nil
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)
	if !called {
		t.Fatalf("expected stopper to be called")
	}
}
