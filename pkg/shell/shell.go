package shell

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/vauxpraudemonium/pkg/toolpane"
	"github.com/mistakeknot/vauxpraudemonium/pkg/tui"
)

// Pane focus state
type Focus int

const (
	FocusProjects Focus = iota
	FocusContent
)

// Model is the main unified shell model
type Model struct {
	// Context
	ctx *Context

	// Layout
	focus Focus

	// Projects pane (always visible)
	projects *ProjectsPane

	// Tool panes
	panes     []toolpane.Pane
	activeTab ToolTab

	// State
	buildInfo   string
	lastRefresh time.Time
	err         error
	quitting    bool
}

// Key bindings
type keyMap struct {
	Tab        key.Binding
	ShiftTab   key.Binding
	Refresh    key.Binding
	FocusLeft  key.Binding
	FocusRight key.Binding
	Quit       key.Binding
	Help       key.Binding
	Number     []key.Binding
}

var keys = keyMap{
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next tab"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev tab"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("ctrl+r", "R"),
		key.WithHelp("ctrl+r", "refresh"),
	),
	FocusLeft: key.NewBinding(
		key.WithKeys("[", "h"),
		key.WithHelp("[", "focus projects"),
	),
	FocusRight: key.NewBinding(
		key.WithKeys("]", "l"),
		key.WithHelp("]", "focus content"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Number: []key.Binding{
		key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "vauxhall")),
		key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "pollard")),
		key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "praude")),
		key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "tandemonium")),
	},
}

// Messages
type refreshMsg struct{}
type tickMsg time.Time

// New creates a new unified shell model
func New(panes []toolpane.Pane, buildInfo string) Model {
	return Model{
		ctx:       NewContext(),
		projects:  NewProjectsPane(),
		panes:     panes,
		activeTab: TabVauxhall,
		focus:     FocusContent,
		buildInfo: buildInfo,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd

	// Initialize all panes
	for _, pane := range m.panes {
		cmd := pane.Init(m.ctx.ToToolpaneContext())
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Start tick timer
	cmds = append(cmds, m.tick())

	return tea.Batch(cmds...)
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global key handling
		switch {
		case key.Matches(msg, keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Tab):
			m.activeTab = ToolTab((int(m.activeTab) + 1) % TabCount)
			m.focus = FocusContent
			return m, nil

		case key.Matches(msg, keys.ShiftTab):
			m.activeTab = ToolTab((int(m.activeTab) + TabCount - 1) % TabCount)
			m.focus = FocusContent
			return m, nil

		case key.Matches(msg, keys.FocusLeft):
			m.focus = FocusProjects
			m.projects.SetFocused(true)
			return m, nil

		case key.Matches(msg, keys.FocusRight):
			m.focus = FocusContent
			m.projects.SetFocused(false)
			return m, nil

		case key.Matches(msg, keys.Refresh):
			return m, func() tea.Msg { return refreshMsg{} }
		}

		// Number keys for direct tab access
		for i, binding := range keys.Number {
			if key.Matches(msg, binding) && i < len(m.panes) {
				m.activeTab = ToolTab(i)
				m.focus = FocusContent
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.ctx.SetSize(msg.Width, msg.Height)
		m.projects.SetSize(m.ctx.ProjectsWidth, m.ctx.ContentHeight)
		// Update pane sizes
		paneCtx := m.ctx.ToToolpaneContext()
		for _, pane := range m.panes {
			pane.Init(paneCtx) // Re-init with new size
		}
		return m, nil

	case refreshMsg:
		m.lastRefresh = time.Now()
		return m, nil

	case tickMsg:
		cmds = append(cmds, m.tick())
		return m, tea.Batch(cmds...)
	}

	// Route messages based on focus
	if m.focus == FocusProjects {
		var cmd tea.Cmd
		m.projects, cmd = m.projects.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

		// Check for project selection change
		if project, ok := m.projects.SelectedProject(); ok {
			if project.Path != m.ctx.ProjectPath {
				m.ctx.SetProject(project.Path)
				// Notify active pane of project change
				if int(m.activeTab) < len(m.panes) {
					pane := m.panes[m.activeTab]
					cmd := pane.Init(m.ctx.ToToolpaneContext())
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
			}
		}
	} else if int(m.activeTab) < len(m.panes) {
		pane := m.panes[m.activeTab]
		newPane, cmd := pane.Update(msg, m.ctx.ToToolpaneContext())
		m.panes[m.activeTab] = newPane
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// View renders the model
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.ctx.Width == 0 {
		return "Loading..."
	}

	// Header
	header := m.renderHeader()

	// Content
	var content string
	if int(m.activeTab) < len(m.panes) {
		pane := m.panes[m.activeTab]
		content = pane.View(m.ctx.ToToolpaneContext())

		// Add sub-tab bar if pane has sub-tabs
		subTabs := pane.SubTabs()
		if len(subTabs) > 0 {
			subTabBar := RenderSubTabBar(subTabs, pane.ActiveSubTab())
			content = lipgloss.JoinVertical(lipgloss.Left, subTabBar, content)
		}
	} else {
		content = "No pane available"
	}

	// Two-pane layout
	mainContent := m.renderTwoPane(m.projects.View(), content)

	// Footer
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		mainContent,
		footer,
	)
}

func (m Model) renderHeader() string {
	title := tui.TitleStyle.Render("⚡ Vauxpraudemonium")
	if m.buildInfo != "" {
		title = title + " " + tui.LabelStyle.Render(m.buildInfo)
	}

	// Render tool tabs
	tabBar := RenderTabBar(m.activeTab)

	// Project indicator
	projectIndicator := ""
	if m.ctx.ProjectName != "" {
		projectIndicator = tui.LabelStyle.Render("  project: ") +
			tui.SubtitleStyle.Render(m.ctx.ProjectName)
	}

	return lipgloss.JoinHorizontal(lipgloss.Center,
		title,
		strings.Repeat(" ", 2),
		tabBar,
		projectIndicator,
	) + "\n"
}

func (m Model) renderFooter() string {
	help := tui.HelpKeyStyle.Render("1-4") + tui.HelpDescStyle.Render(" tools • ") +
		tui.HelpKeyStyle.Render("tab") + tui.HelpDescStyle.Render(" next • ") +
		tui.HelpKeyStyle.Render("[/]") + tui.HelpDescStyle.Render(" focus • ") +
		tui.HelpKeyStyle.Render("ctrl+r") + tui.HelpDescStyle.Render(" refresh • ") +
		tui.HelpKeyStyle.Render("q") + tui.HelpDescStyle.Render(" quit")

	lastUpdate := ""
	if !m.lastRefresh.IsZero() {
		lastUpdate = tui.LabelStyle.Render(fmt.Sprintf("Updated %s ago", time.Since(m.lastRefresh).Round(time.Second)))
	}

	padding := m.ctx.Width - lipgloss.Width(help) - lipgloss.Width(lastUpdate) - 4
	if padding < 1 {
		padding = 1
	}

	return lipgloss.JoinHorizontal(lipgloss.Center,
		help,
		strings.Repeat(" ", padding),
		lastUpdate,
	)
}

func (m Model) renderTwoPane(left, right string) string {
	if m.ctx.IsSinglePane() {
		return right
	}

	leftStyle := tui.PanelStyle.Copy()
	rightStyle := tui.PanelStyle.Copy()

	if m.focus == FocusProjects {
		leftStyle = leftStyle.BorderForeground(tui.ColorPrimary)
	} else {
		rightStyle = rightStyle.BorderForeground(tui.ColorPrimary)
	}

	leftView := leftStyle.Width(m.ctx.ProjectsWidth).Height(m.ctx.ContentHeight).Render(left)
	rightView := rightStyle.Width(m.ctx.ContentWidth).Height(m.ctx.ContentHeight).Render(right)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView)
}

// SetProjects updates the projects list
func (m *Model) SetProjects(projects []Project) {
	m.projects.SetProjects(projects)
}
