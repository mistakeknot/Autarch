package tui

import "testing"

func TestInitialModelHasTitle(t *testing.T) {
	m := NewModel()
	if m.Title == "" {
		t.Fatal("expected title")
	}
}

func TestModelHasSessions(t *testing.T) {
	m := NewModel()
	if m.Sessions == nil {
		t.Fatal("expected sessions slice")
	}
}

func TestRefreshTasksLoadsFromProject(t *testing.T) {
	m := NewModel()
	m.TaskLoader = func() ([]TaskItem, error) { return []TaskItem{{ID: "T1"}}, nil }
	m.RefreshTasks()
	if len(m.TaskList) != 1 {
		t.Fatalf("expected tasks loaded")
	}
}
