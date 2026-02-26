package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// TestSidebarProviderImplementations verifies that browse views implement SidebarProvider.
// This is a compile-time check that also validates runtime behavior.
func TestSidebarProviderImplementations(t *testing.T) {
	// These are already compile-time assertions in the respective files,
	// but we verify them here for documentation.
	var _ pkgtui.SidebarProvider = (*GurgehView)(nil)
	var _ pkgtui.SidebarProvider = (*PollardView)(nil)
	var _ pkgtui.SidebarProvider = (*ColdwineView)(nil)
}

// TestColdwineViewShellIntegration tests ColdwineView's shell layout integration.
func TestColdwineViewShellIntegration(t *testing.T) {
	view := NewColdwineView(nil) // nil client for testing
	view.width = 120
	view.height = 40

	// Set shell size
	view.shell.SetSize(120, 40)

	// Verify SidebarItems returns mode toggle items even when no epics
	items := view.SidebarItems()
	if len(items) != 2 {
		t.Errorf("SidebarItems should return 2 mode toggle items when no epics, got %d items", len(items))
	}
	if len(items) >= 2 {
		if items[0].ID != "__mode_epics" || items[1].ID != "__mode_runs" {
			t.Errorf("first two items should be mode toggles, got %q and %q", items[0].ID, items[1].ID)
		}
	}

	// Test Tab key cycles focus
	msg := tea.KeyMsg{Type: tea.KeyTab}
	initialFocus := view.shell.Focus()
	view.shell, _ = view.shell.Update(msg)
	newFocus := view.shell.Focus()

	if initialFocus == newFocus {
		t.Error("Tab should cycle focus")
	}
}

// TestReviewViewsProvideSidebarItems ensures review views expose left-nav content.
func TestReviewViewsProvideSidebarItems(t *testing.T) {
	if len(NewSpecSummaryView(nil, nil).SidebarItems()) == 0 {
		t.Fatalf("expected spec summary sidebar items")
	}
	if len(NewEpicReviewView(nil).SidebarItems()) == 0 {
		t.Fatalf("expected epic review sidebar items")
	}
	if len(NewTaskReviewView(nil).SidebarItems()) == 0 {
		t.Fatalf("expected task review sidebar items")
	}
	view := &TaskDetailView{
		shell: pkgtui.NewShellLayout(),
	}
	if len(view.SidebarItems()) == 0 {
		t.Fatalf("expected task detail sidebar items")
	}
}

// TestViewsHandleTabKey tests that all views properly delegate Tab to shell.
func TestViewsHandleTabKey(t *testing.T) {
	tabMsg := tea.KeyMsg{Type: tea.KeyTab}

	t.Run("ColdwineView", func(t *testing.T) {
		view := NewColdwineView(nil)
		view.shell.SetSize(120, 40)

		before := view.shell.Focus()
		view.Update(tabMsg)
		after := view.shell.Focus()

		if before == after {
			t.Error("Tab should cycle focus in ColdwineView")
		}
	})

	t.Run("EpicReviewView", func(t *testing.T) {
		view := NewEpicReviewView(nil)
		view.shell.SetSize(120, 40)

		before := view.shell.Focus()
		view.Update(tabMsg)
		after := view.shell.Focus()

		if before == after {
			t.Error("Tab should cycle focus in EpicReviewView")
		}
	})

	t.Run("TaskReviewView", func(t *testing.T) {
		view := NewTaskReviewView(nil)
		view.shell.SetSize(120, 40)

		before := view.shell.Focus()
		view.Update(tabMsg)
		after := view.shell.Focus()

		if before == after {
			t.Error("Tab should cycle focus in TaskReviewView")
		}
	})

	t.Run("SpecSummaryView", func(t *testing.T) {
		view := NewSpecSummaryView(nil, nil)
		view.shell.SetSize(120, 40)

		before := view.shell.Focus()
		view.Update(tabMsg)
		after := view.shell.Focus()

		if before == after {
			t.Error("Tab should cycle focus in SpecSummaryView")
		}
	})
}

// TestViewsHandleCtrlB tests sidebar toggle in views with sidebar.
func TestViewsHandleCtrlB(t *testing.T) {
	ctrlBMsg := tea.KeyMsg{Type: tea.KeyCtrlB}

	t.Run("ColdwineView", func(t *testing.T) {
		view := NewColdwineView(nil)
		view.shell.SetSize(120, 40)

		before := view.shell.Sidebar().IsCollapsed()
		view.Update(ctrlBMsg)
		after := view.shell.Sidebar().IsCollapsed()

		if before == after {
			t.Error("Ctrl+B should toggle sidebar in ColdwineView")
		}
	})
}

// TestViewShortHelpIncludesTab tests that ShortHelp mentions Tab focus.
func TestViewShortHelpIncludesTab(t *testing.T) {
	tests := []struct {
		name string
		help string
	}{
		{"ColdwineView", NewColdwineView(nil).ShortHelp()},
		{"EpicReviewView", NewEpicReviewView(nil).ShortHelp()},
		{"TaskReviewView", NewTaskReviewView(nil).ShortHelp()},
		{"SpecSummaryView", NewSpecSummaryView(nil, nil).ShortHelp()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.help == "" {
				t.Error("ShortHelp should not be empty")
			}
			// Check that help mentions Tab/focus
			if !containsAny(tt.help, "Tab", "focus") {
				t.Errorf("ShortHelp should mention Tab or focus: %q", tt.help)
			}
		})
	}
}

func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
