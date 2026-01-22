package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilterClearsOnEscape(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m = m.withFilterActive("codex")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(Model)
	if mm.filterInput.Value() != "" {
		t.Fatalf("expected empty filter")
	}
	if mm.filterActive {
		t.Fatalf("expected filter inactive")
	}
}

func TestFilterUIHiddenWhenEmpty(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.width = 80
	m.height = 20
	view := m.View()
	if strings.Contains(view, "Filter:") {
		t.Fatalf("did not expect filter line")
	}
}

func TestFilterUIShownWhenActive(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.width = 80
	m.height = 20
	m = m.withFilterActive("codex")
	view := m.View()
	if !strings.Contains(view, "Filter:") {
		t.Fatalf("expected filter line")
	}
}
