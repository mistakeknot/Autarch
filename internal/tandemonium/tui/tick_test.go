package tui

import "testing"

func TestTickRefreshesTasks(t *testing.T) {
	m := NewModel()
	calls := 0
	m.TaskLoader = func() ([]TaskItem, error) {
		calls++
		return []TaskItem{}, nil
	}
	updated, _ := m.Update(tickMsg{})
	m = updated.(Model)
	if calls != 1 {
		t.Fatalf("expected TaskLoader called once, got %d", calls)
	}
}
