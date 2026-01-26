package tui

import tea "github.com/charmbracelet/bubbletea"

// View represents a tool view in the unified TUI
type View interface {
	// Init initializes the view
	Init() tea.Cmd

	// Update handles messages
	Update(msg tea.Msg) (View, tea.Cmd)

	// View renders the view
	View() string

	// Focus is called when this view becomes active
	Focus() tea.Cmd

	// Blur is called when this view becomes inactive
	Blur()

	// Name returns the view name for the tab bar
	Name() string

	// ShortHelp returns keybinding hints for the footer
	ShortHelp() string
}

// HelpBinding represents a single keybinding for the help overlay
type HelpBinding struct {
	Key         string // The key(s) to press (e.g., "j/k", "enter", "A")
	Description string // What the key does
}

// FullHelpProvider can provide complete keybinding documentation
type FullHelpProvider interface {
	// FullHelp returns all available keybindings for the help overlay
	FullHelp() []HelpBinding
}

// Command represents an action that can be invoked from the command palette
type Command struct {
	Name        string
	Description string
	Action      func() tea.Cmd
}

// CommandProvider can provide commands for the palette
type CommandProvider interface {
	Commands() []Command
}
