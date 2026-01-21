package tui

import "testing"

func TestModelHasDiffFiles(t *testing.T) {
	m := NewModel()
	if m.DiffFiles == nil {
		t.Fatal("expected diff files")
	}
}
