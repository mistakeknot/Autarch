package tui

import (
	"strings"
	"testing"
)

func TestViewUsesWindowHeight(t *testing.T) {
	m := NewModel()
	m.Width = 80
	m.Height = 5
	out := m.View()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
}
