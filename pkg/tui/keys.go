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
			key.WithKeys("esc", "h"),
			key.WithHelp("esc/h", "back"),
		),
		NavUp: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/up", "up"),
		),
		NavDown: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/down", "down"),
		),
		Top: key.NewBinding(
			key.WithKeys("g", "home"),
			key.WithHelp("g/home", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G/end", "bottom"),
		),
		Next: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next"),
		),
		Prev: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "prev"),
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
		Sections: []key.Binding{
			key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "section 1")),
			key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "section 2")),
			key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "section 3")),
			key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "section 4")),
			key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "section 5")),
			key.NewBinding(key.WithKeys("6"), key.WithHelp("6", "section 6")),
			key.NewBinding(key.WithKeys("7"), key.WithHelp("7", "section 7")),
			key.NewBinding(key.WithKeys("8"), key.WithHelp("8", "section 8")),
			key.NewBinding(key.WithKeys("9"), key.WithHelp("9", "section 9")),
		},
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
