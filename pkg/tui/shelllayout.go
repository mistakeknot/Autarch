package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MinShellWidth is the minimum terminal width required for the shell layout.
const MinShellWidth = 100

// FocusTarget represents which pane has focus in the shell.
type FocusTarget int

const (
	FocusSidebar FocusTarget = iota
	FocusDocument
	FocusChat
)

// ShellLayout provides the unified 3-pane Cursor-style layout.
// It composes a Sidebar with an existing SplitLayout for doc + chat.
type ShellLayout struct {
	sidebar     *Sidebar
	splitLayout *SplitLayout
	width       int
	height      int
	showSidebar bool
	focus       FocusTarget
}

// NewShellLayout creates a new shell layout with default settings.
func NewShellLayout() *ShellLayout {
	return &ShellLayout{
		sidebar:     NewSidebar(),
		splitLayout: NewSplitLayout(0.66),
		showSidebar: true,
		focus:       FocusDocument,
	}
}

// SetSize sets the available dimensions for the layout.
// Applies the documented learning from tui-dimension-mismatch-splitlayout-20260126.md.
func (l *ShellLayout) SetSize(width, height int) {
	l.width = width
	l.height = height

	// Require minimum 100 chars - will show error in Render()
	if width < MinShellWidth {
		return
	}

	sidebarW := 0
	if l.showSidebar && !l.sidebar.IsCollapsed() {
		sidebarW = SidebarWidth
	}

	// Content area = width - sidebar - separator (if sidebar visible)
	contentWidth := width - sidebarW
	if sidebarW > 0 {
		contentWidth -= 1 // separator
	}

	l.sidebar.SetSize(sidebarW, height)
	l.splitLayout.SetSize(contentWidth, height)
}

// ToggleSidebar toggles sidebar visibility with focus recovery.
func (l *ShellLayout) ToggleSidebar() {
	l.sidebar.Toggle()
	// If sidebar collapses while focused, move focus to document
	if l.sidebar.IsCollapsed() && l.focus == FocusSidebar {
		l.focus = FocusDocument
	}
	// Recalculate dimensions
	l.SetSize(l.width, l.height)
}

// NextFocus cycles focus to the next pane.
// Skips sidebar when collapsed.
func (l *ShellLayout) NextFocus() {
	switch l.focus {
	case FocusSidebar:
		l.focus = FocusDocument
	case FocusDocument:
		l.focus = FocusChat
	case FocusChat:
		if l.showSidebar && !l.sidebar.IsCollapsed() {
			l.focus = FocusSidebar
		} else {
			l.focus = FocusDocument
		}
	}
}

// PrevFocus cycles focus to the previous pane.
func (l *ShellLayout) PrevFocus() {
	switch l.focus {
	case FocusSidebar:
		l.focus = FocusChat
	case FocusDocument:
		if l.showSidebar && !l.sidebar.IsCollapsed() {
			l.focus = FocusSidebar
		} else {
			l.focus = FocusChat
		}
	case FocusChat:
		l.focus = FocusDocument
	}
}

// Focus returns the current focus target.
func (l *ShellLayout) Focus() FocusTarget {
	return l.focus
}

// SetFocus sets the focus target directly.
func (l *ShellLayout) SetFocus(target FocusTarget) {
	l.focus = target
}

// Sidebar returns the sidebar component for direct access.
func (l *ShellLayout) Sidebar() *Sidebar {
	return l.sidebar
}

// SplitLayout returns the split layout component for direct access.
func (l *ShellLayout) SplitLayout() *SplitLayout {
	return l.splitLayout
}

// Update handles shell-level keyboard input.
func (l *ShellLayout) Update(msg tea.Msg) (*ShellLayout, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+b":
			l.ToggleSidebar()
			return l, nil
		case "tab":
			l.NextFocus()
			return l, nil
		case "shift+tab":
			l.PrevFocus()
			return l, nil
		}
	}

	// Forward to sidebar if focused
	if l.focus == FocusSidebar && !l.sidebar.IsCollapsed() {
		var cmd tea.Cmd
		l.sidebar, cmd = l.sidebar.Update(msg)
		return l, cmd
	}

	return l, nil
}

// Render combines sidebar, document, and chat content into the shell layout.
// Document and chat are pre-rendered strings.
func (l *ShellLayout) Render(sidebarItems []SidebarItem, document, chat string) string {
	// Check minimum width
	if l.width < MinShellWidth {
		return l.renderWidthError()
	}

	// Update sidebar items
	l.sidebar.SetItems(sidebarItems)

	// Update focus states
	l.sidebar.Blur()
	if l.focus == FocusSidebar {
		l.sidebar.Focus()
	}

	// Render sidebar
	sidebarView := l.sidebar.View()

	// Render split layout (document + chat)
	splitView := l.splitLayout.Render(document, chat)

	// Combine
	if l.showSidebar && !l.sidebar.IsCollapsed() {
		// Add separator between sidebar and content
		sepStyle := lipgloss.NewStyle().Foreground(ColorBorder)
		sep := sepStyle.Render(strings.Repeat("│\n", l.height))
		sep = strings.TrimSuffix(sep, "\n")

		return lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, sep, splitView)
	}

	return splitView
}

// RenderWithoutSidebar renders only the document and chat panes.
// Used for onboarding views that don't need a sidebar.
func (l *ShellLayout) RenderWithoutSidebar(document, chat string) string {
	if l.width < MinShellWidth {
		return l.renderWidthError()
	}

	// Use full width for split layout
	l.splitLayout.SetSize(l.width, l.height)
	return l.splitLayout.Render(document, chat)
}

// renderWidthError shows an error when terminal is too narrow.
func (l *ShellLayout) renderWidthError() string {
	errorStyle := lipgloss.NewStyle().
		Foreground(ColorError).
		Bold(true)

	msg := errorStyle.Render("Terminal too narrow")
	hint := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("Minimum width: 100 characters")

	return lipgloss.JoinVertical(lipgloss.Center, msg, hint)
}

// IsSidebarVisible returns whether the sidebar is visible (not collapsed).
func (l *ShellLayout) IsSidebarVisible() bool {
	return l.showSidebar && !l.sidebar.IsCollapsed()
}

// LeftWidth returns the document pane width.
func (l *ShellLayout) LeftWidth() int {
	return l.splitLayout.LeftWidth()
}

// RightWidth returns the chat pane width.
func (l *ShellLayout) RightWidth() int {
	return l.splitLayout.RightWidth()
}

// Height returns the available height for content panes.
func (l *ShellLayout) Height() int {
	return l.height
}
