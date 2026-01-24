// Package toolpane defines the interface that each tool's TUI implements
// to be composed into the unified shell.
package toolpane

import tea "github.com/charmbracelet/bubbletea"

// Context is shared across all tool panes
type Context struct {
	ProjectPath string // Selected project path (empty = all projects)
	ProjectName string // Project basename
	Width       int    // Available width for the pane
	Height      int    // Available height for the pane
}

// Pane is implemented by each tool's TUI (Vauxhall, Praude, Tandemonium, Pollard)
type Pane interface {
	// Init initializes the pane with context
	Init(ctx Context) tea.Cmd

	// Update handles messages
	Update(msg tea.Msg, ctx Context) (Pane, tea.Cmd)

	// View renders the pane
	View(ctx Context) string

	// Name returns the tool name for the tab bar
	Name() string

	// SubTabs returns the tool's internal tabs (if any)
	SubTabs() []string

	// ActiveSubTab returns current sub-tab index
	ActiveSubTab() int

	// SetSubTab switches to a sub-tab
	SetSubTab(index int) tea.Cmd

	// NeedsProject returns true if tool requires project context
	NeedsProject() bool
}

// ProjectSelectedMsg is sent when the user selects a project
type ProjectSelectedMsg struct {
	Path string
	Name string
}

// RefreshMsg is sent to request a refresh of the pane data
type RefreshMsg struct{}

// ErrorMsg wraps an error from a pane
type ErrorMsg struct {
	Err error
}

func (e ErrorMsg) Error() string {
	return e.Err.Error()
}
