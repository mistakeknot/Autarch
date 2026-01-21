package tui

import "testing"

func TestSearchFiltersTasks(t *testing.T) {
	m := NewModel()
	m.TaskList = []TaskItem{
		{ID: "T1", Title: "Alpha"},
		{ID: "T2", Title: "Beta"},
	}
	m.SearchQuery = "alp"
	filtered := m.filteredTasks()
	if len(filtered) != 1 || filtered[0].ID != "T1" {
		t.Fatalf("expected Alpha only")
	}
}
