package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// CommonKeys defines the shared keybindings used across all Autarch TUIs.
type CommonKeys struct {
	Quit     key.Binding
	Help     key.Binding
	Search   key.Binding
	Back     key.Binding
	NavUp    key.Binding
	NavDown  key.Binding
	Top      key.Binding
	Bottom   key.Binding
	Next     key.Binding
	Prev     key.Binding
	Refresh  key.Binding
	TabCycle key.Binding
	Select   key.Binding
	Toggle   key.Binding
	Sections []key.Binding
}

// NewCommonKeys returns a CommonKeys with the canonical Autarch keybindings.
// Note: Function keys (F1-F12) are awkward on MacBooks where they're bound to
// system functions by default. We provide Ctrl-based alternatives for all actions.
func NewCommonKeys() CommonKeys {
	return CommonKeys{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c×2", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?", "f1"),
			key.WithHelp("/help", "help"),
		),
		Search: key.NewBinding(
			key.WithKeys("ctrl+f"),
			key.WithHelp("ctrl+f", "search"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		NavUp: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("up", "up"),
		),
		NavDown: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("down", "down"),
		),
		Top: key.NewBinding(
			key.WithKeys("home"),
			key.WithHelp("home", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("end"),
			key.WithHelp("end", "bottom"),
		),
		Next: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "next"),
		),
		Prev: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "prev"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "refresh"),
		),
		TabCycle: key.NewBinding(
			key.WithKeys("tab", "shift+tab"),
			key.WithHelp("tab", "cycle panes"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "toggle"),
		),
		Sections: nil,
	}
}

// ToggleHelpMsg is sent when the user presses the help key.
type ToggleHelpMsg struct{}

// SlashCommandMsg is sent when the user submits a slash command (e.g., /help, /quit).
type SlashCommandMsg struct {
	Command string   // The command name without the leading slash
	Args    []string // Any arguments after the command
}

// HandleCommon processes a key message against the common keybindings.
// It returns a tea.Cmd if the key was handled (tea.Quit for quit,
// a ToggleHelpMsg command for help), or nil if unhandled.
func HandleCommon(msg tea.KeyMsg, keys CommonKeys) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Quit):
		return tea.Quit
	case key.Matches(msg, keys.Help):
		return func() tea.Msg { return ToggleHelpMsg{} }
	}
	return nil
}
