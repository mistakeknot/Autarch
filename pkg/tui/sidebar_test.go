package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSidebarToggle(t *testing.T) {
	s := NewSidebar()

	if s.IsCollapsed() {
		t.Fatal("expected sidebar to start expanded")
	}

	s.Toggle()
	if !s.IsCollapsed() {
		t.Fatal("expected sidebar to be collapsed after toggle")
	}

	s.Toggle()
	if s.IsCollapsed() {
		t.Fatal("expected sidebar to be expanded after second toggle")
	}
}

func TestSidebarWidthWhenCollapsed(t *testing.T) {
	s := NewSidebar()
	s.SetSize(100, 40)

	if s.Width() != SidebarWidth {
		t.Fatalf("expected width %d, got %d", SidebarWidth, s.Width())
	}

	s.Toggle()
	s.SetSize(100, 40)

	if s.Width() != 0 {
		t.Fatalf("expected collapsed width 0, got %d", s.Width())
	}
}

func TestSidebarNavigationJK(t *testing.T) {
	s := NewSidebar()
	s.SetItems([]SidebarItem{
		{ID: "1", Label: "Item 1"},
		{ID: "2", Label: "Item 2"},
		{ID: "3", Label: "Item 3"},
	})
	s.Focus()

	// Initial selection
	item, ok := s.Selected()
	if !ok || item.ID != "1" {
		t.Fatal("expected first item selected initially")
	}

	// Move down with j
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	item, _ = s.Selected()
	if item.ID != "2" {
		t.Fatalf("expected item 2 after j, got %s", item.ID)
	}

	// Move up with k
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	item, _ = s.Selected()
	if item.ID != "1" {
		t.Fatalf("expected item 1 after k, got %s", item.ID)
	}
}

func TestSidebarNavigationClamps(t *testing.T) {
	s := NewSidebar()
	s.SetItems([]SidebarItem{
		{ID: "1", Label: "Item 1"},
		{ID: "2", Label: "Item 2"},
	})
	s.Focus()

	// Try to move up when at first item
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	item, _ := s.Selected()
	if item.ID != "1" {
		t.Fatal("expected to stay at first item")
	}

	// Move to last item and try to move down
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	item, _ = s.Selected()
	if item.ID != "2" {
		t.Fatal("expected to stay at last item")
	}
}

func TestSidebarEnterSelectsItem(t *testing.T) {
	s := NewSidebar()
	s.SetItems([]SidebarItem{
		{ID: "test-id", Label: "Test Item"},
	})
	s.Focus()

	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected command from enter key")
	}

	msg := cmd()
	selectMsg, ok := msg.(SidebarSelectMsg)
	if !ok {
		t.Fatal("expected SidebarSelectMsg")
	}
	if selectMsg.ItemID != "test-id" {
		t.Fatalf("expected item ID 'test-id', got %s", selectMsg.ItemID)
	}
}

func TestSidebarTruncation(t *testing.T) {
	s := NewSidebar()
	s.SetItems([]SidebarItem{
		{ID: "1", Label: "This is a very long label that should be truncated"},
	})
	s.SetSize(SidebarWidth, 10)

	view := s.View()
	// Should contain ellipsis for truncated label
	// The view includes ANSI escape codes, so check for the Unicode ellipsis
	if !strings.Contains(view, "…") && !strings.Contains(view, "This is a very") {
		t.Fatal("expected truncated label with ellipsis or truncated text")
	}
	// Should not contain the full original label
	if strings.Contains(view, "should be truncated") {
		t.Fatal("expected label to be truncated, but found full text")
	}
}

func TestSidebarEmptyState(t *testing.T) {
	s := NewSidebar()
	s.SetSize(SidebarWidth, 10)

	view := s.View()
	if !strings.Contains(view, "No items yet") {
		t.Fatal("expected empty state message")
	}
}

func TestSidebarCollapsedViewIsEmpty(t *testing.T) {
	s := NewSidebar()
	s.SetItems([]SidebarItem{
		{ID: "1", Label: "Item 1"},
	})
	s.Toggle() // Collapse
	s.SetSize(0, 10)

	view := s.View()
	if view != "" {
		t.Fatalf("expected empty view when collapsed, got %q", view)
	}
}

func TestSidebarIgnoresInputWhenNotFocused(t *testing.T) {
	s := NewSidebar()
	s.SetItems([]SidebarItem{
		{ID: "1", Label: "Item 1"},
		{ID: "2", Label: "Item 2"},
	})
	// Don't call Focus()

	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	item, _ := s.Selected()
	if item.ID != "1" {
		t.Fatal("expected selection not to change when not focused")
	}
}

func TestSidebarSetItemsClampsSelection(t *testing.T) {
	s := NewSidebar()
	s.SetItems([]SidebarItem{
		{ID: "1", Label: "Item 1"},
		{ID: "2", Label: "Item 2"},
		{ID: "3", Label: "Item 3"},
	})
	s.Focus()

	// Move to last item
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Reduce items
	s.SetItems([]SidebarItem{
		{ID: "1", Label: "Item 1"},
	})

	item, _ := s.Selected()
	if item.ID != "1" {
		t.Fatalf("expected selection clamped to valid range, got %s", item.ID)
	}
}
