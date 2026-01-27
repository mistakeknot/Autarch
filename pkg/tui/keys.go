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
	Refresh  key.Binding
	TabCycle key.Binding
	Select   key.Binding
	Toggle   key.Binding
}

// NewCommonKeys returns a CommonKeys with the canonical Autarch keybindings.
func NewCommonKeys() CommonKeys {
	return CommonKeys{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q/ctrl+c", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		NavUp: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/up", "up"),
		),
		NavDown: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/down", "down"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
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
			key.WithKeys(" "),
			key.WithHelp("space", "toggle"),
		),
	}
}

// ToggleHelpMsg is sent when the user presses the help key.
type ToggleHelpMsg struct{}

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
