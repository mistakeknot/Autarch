package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/pkg/autarch"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// App is the main unified TUI application
type App struct {
	client  *autarch.Client
	tabs    *TabBar
	views   []View
	palette *Palette
	width   int
	height  int
	err     error
	keys    pkgtui.CommonKeys
	help    pkgtui.HelpOverlay
}

// NewApp creates a new unified TUI app
func NewApp(client *autarch.Client, views ...View) *App {
	names := make([]string, len(views))
	for i, v := range views {
		names[i] = v.Name()
	}

	app := &App{
		client:  client,
		tabs:    NewTabBar(names),
		views:   views,
		palette: NewPalette(),
		keys:    pkgtui.NewCommonKeys(),
		help:    pkgtui.NewHelpOverlay(),
	}

	// Collect commands from all views
	app.updateCommands()

	return app
}

func (a *App) updateCommands() {
	var cmds []Command

	// Global commands
	cmds = append(cmds,
		Command{
			Name:        "Switch model",
			Description: "Toggle model selector",
			Action: func() tea.Cmd {
				return func() tea.Msg {
					return tea.KeyMsg{Type: tea.KeyF2}
				}
			},
		},
		Command{
			Name:        "Switch to Bigend",
			Description: "View sessions",
			Action:      func() tea.Cmd { return a.switchTab(0) },
		},
		Command{
			Name:        "Switch to Gurgeh",
			Description: "View specs",
			Action:      func() tea.Cmd { return a.switchTab(1) },
		},
		Command{
			Name:        "Switch to Coldwine",
			Description: "View epics and tasks",
			Action:      func() tea.Cmd { return a.switchTab(2) },
		},
		Command{
			Name:        "Switch to Pollard",
			Description: "View insights",
			Action:      func() tea.Cmd { return a.switchTab(3) },
		},
	)

	// Collect commands from views
	for _, v := range a.views {
		if provider, ok := v.(CommandProvider); ok {
			cmds = append(cmds, provider.Commands()...)
		}
	}

	a.palette.SetCommands(cmds)
}

func (a *App) switchTab(index int) tea.Cmd {
	return func() tea.Msg {
		return switchTabMsg{index: index}
	}
}

type switchTabMsg struct {
	index int
}

// Init implements tea.Model
func (a *App) Init() tea.Cmd {
	var cmds []tea.Cmd

	// Initialize all views
	for _, v := range a.views {
		cmds = append(cmds, v.Init())
	}

	// Focus the first view
	if len(a.views) > 0 {
		cmds = append(cmds, a.views[0].Focus())
	}

	return tea.Batch(cmds...)
}

// Update implements tea.Model
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.tabs.SetWidth(msg.Width)
		a.palette.SetSize(msg.Width, msg.Height)

		// Pass size to active view
		if len(a.views) > 0 {
			active := a.tabs.Active()
			if active < len(a.views) {
				var cmd tea.Cmd
				a.views[active], cmd = a.views[active].Update(msg)
				cmds = append(cmds, cmd)
			}
		}
		return a, tea.Batch(cmds...)

	case tea.KeyMsg:
		if key.Matches(msg, a.keys.Quit) {
			return a, tea.Quit
		}
		if a.help.Visible {
			switch {
			case key.Matches(msg, a.keys.Help), key.Matches(msg, a.keys.Back):
				a.help.Toggle()
			}
			return a, nil
		}
		// Handle palette first if visible
		if a.palette.Visible() {
			var cmd tea.Cmd
			a.palette, cmd = a.palette.Update(msg)
			return a, cmd
		}

		if cmd := pkgtui.HandleCommon(msg, a.keys); cmd != nil {
			return a, cmd
		}

		switch msg.String() {
		case "ctrl+p":
			return a, a.palette.Show()

		default:
			switch {
			case len(a.keys.Sections) >= 4 && key.Matches(msg, a.keys.Sections[0]):
				return a, a.doSwitchTab(0)
			case len(a.keys.Sections) >= 4 && key.Matches(msg, a.keys.Sections[1]):
				return a, a.doSwitchTab(1)
			case len(a.keys.Sections) >= 4 && key.Matches(msg, a.keys.Sections[2]):
				return a, a.doSwitchTab(2)
			case len(a.keys.Sections) >= 4 && key.Matches(msg, a.keys.Sections[3]):
				return a, a.doSwitchTab(3)
			case key.Matches(msg, a.keys.TabCycle):
				if msg.String() == "shift+tab" {
					return a, a.doSwitchTab((a.tabs.Active() - 1 + len(a.views)) % len(a.views))
				}
				return a, a.doSwitchTab((a.tabs.Active() + 1) % len(a.views))
			}
		}

	case switchTabMsg:
		return a, a.doSwitchTab(msg.index)

	case pkgtui.ToggleHelpMsg:
		a.help.Toggle()
		return a, nil
	}

	// Pass message to active view
	if len(a.views) > 0 {
		active := a.tabs.Active()
		if active < len(a.views) {
			var cmd tea.Cmd
			a.views[active], cmd = a.views[active].Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return a, tea.Batch(cmds...)
}

func (a *App) doSwitchTab(index int) tea.Cmd {
	if index < 0 || index >= len(a.views) {
		return nil
	}

	oldActive := a.tabs.Active()
	if oldActive == index {
		return nil
	}

	// Blur old view
	if oldActive < len(a.views) {
		a.views[oldActive].Blur()
	}

	// Switch tab
	a.tabs.SetActive(index)

	// Focus new view
	return a.views[index].Focus()
}

// View implements tea.Model
func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		return "Loading..."
	}

	if a.help.Visible {
		return a.help.Render(a.keys, a.helpExtras(), a.width)
	}

	var b strings.Builder

	// Tab bar (2 lines)
	b.WriteString(a.tabs.View())
	b.WriteString("\n")

	// Calculate content height (total - tabs - footer)
	contentHeight := a.height - 4

	// Active view content
	var content string
	if len(a.views) > 0 {
		active := a.tabs.Active()
		if active < len(a.views) {
			content = a.views[active].View()
		}
	}

	// Ensure content fills the space
	contentLines := strings.Split(content, "\n")
	for len(contentLines) < contentHeight {
		contentLines = append(contentLines, "")
	}
	if len(contentLines) > contentHeight {
		contentLines = contentLines[:contentHeight]
	}
	b.WriteString(strings.Join(contentLines, "\n"))
	b.WriteString("\n")

	// Footer
	b.WriteString(a.renderFooter())

	// Overlay palette if visible
	if a.palette.Visible() {
		paletteView := a.palette.View()
		return a.overlay(b.String(), paletteView)
	}

	return b.String()
}

func (a *App) helpExtras() []pkgtui.HelpBinding {
	if len(a.views) == 0 {
		return nil
	}
	active := a.tabs.Active()
	if active >= len(a.views) {
		return nil
	}
	if provider, ok := a.views[active].(pkgtui.FullHelpProvider); ok {
		return provider.FullHelp()
	}
	return nil
}

func (a *App) renderFooter() string {
	// Get help from active view
	help := "1-4 tabs  ctrl+p palette  ctrl+c quit"
	if len(a.views) > 0 {
		active := a.tabs.Active()
		if active < len(a.views) {
			viewHelp := a.views[active].ShortHelp()
			if viewHelp != "" {
				help = viewHelp + "  " + help
			}
		}
	}

	style := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted).
		Width(a.width)

	return style.Render(help)
}

func (a *App) overlay(base, overlay string) string {
	// Simple overlay: center the overlay on top of the base
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Calculate position
	startRow := (a.height - len(overlayLines)) / 4
	startCol := (a.width - lipgloss.Width(overlayLines[0])) / 2

	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	// Overlay
	for i, line := range overlayLines {
		row := startRow + i
		if row >= len(baseLines) {
			break
		}
		baseLines[row] = a.insertAt(baseLines[row], startCol, line)
	}

	return strings.Join(baseLines, "\n")
}

func (a *App) insertAt(base string, col int, overlay string) string {
	// Handle ANSI sequences in base
	baseRunes := []rune(base)

	// Pad base if needed
	for len(baseRunes) < col {
		baseRunes = append(baseRunes, ' ')
	}

	// Calculate visible width of overlay
	overlayWidth := lipgloss.Width(overlay)

	// Build result
	var result strings.Builder

	// Write prefix
	if col > 0 && col < len(baseRunes) {
		result.WriteString(string(baseRunes[:col]))
	}

	// Write overlay
	result.WriteString(overlay)

	// Write suffix (skip overlayed chars)
	end := col + overlayWidth
	if end < len(baseRunes) {
		result.WriteString(string(baseRunes[end:]))
	}

	return result.String()
}

// Run starts the TUI application
func Run(client *autarch.Client, views ...View) error {
	app := NewApp(client, views...)
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// ErrorView shows an error state
func ErrorView(err error) string {
	return fmt.Sprintf("%s\n\n%s",
		pkgtui.StatusError.Render("Error"),
		pkgtui.LabelStyle.Render(err.Error()),
	)
}

// EmptyView shows an empty state
func EmptyView(message string) string {
	return pkgtui.LabelStyle.Render(message)
}
