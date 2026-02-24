package tui

import tea "github.com/charmbracelet/bubbletea"

// View represents a tool view in the unified Autarch TUI.
// Each tool (Bigend, Gurgeh, Coldwine, Pollard) implements this interface
// to provide its UI within the tabbed application.
type View interface {
	// Init initializes the view and returns any initial commands
	Init() tea.Cmd

	// Update handles messages and returns the updated view and any commands
	Update(msg tea.Msg) (View, tea.Cmd)

	// View renders the view as a string
	View() string

	// Focus is called when this view becomes the active tab
	Focus() tea.Cmd

	// Blur is called when this view is no longer the active tab
	Blur()

	// Name returns the view name for display in the tab bar
	Name() string

	// ShortHelp returns keybinding hints for the footer
	ShortHelp() string
}

// HelpBinding represents a single keybinding for the help overlay
type HelpBinding struct {
	Key         string // The key(s) to press (e.g., "up/down", "enter", "F3")
	Description string // What the key does
}

// FullHelpProvider can provide complete keybinding documentation.
// Views that implement this interface can display detailed help
// when the user presses F1.
type FullHelpProvider interface {
	// FullHelp returns all available keybindings for the help overlay
	FullHelp() []HelpBinding
}

// Command represents an action that can be invoked from the command palette
type Command struct {
	Name        string
	Description string
	Action      func() tea.Cmd
	Broadcast   bool // If true, enters target selection before executing
}

// CommandProvider can provide commands for the palette.
// Views that implement this interface can contribute commands
// to the global command palette (Ctrl+P).
type CommandProvider interface {
	Commands() []Command
}
