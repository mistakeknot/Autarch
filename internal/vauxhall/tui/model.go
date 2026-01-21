package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/aggregator"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/tmux"
)

// Tab represents a view tab
type Tab int

const (
	TabDashboard Tab = iota
	TabSessions
	TabProjects
	TabAgents
)

func (t Tab) String() string {
	switch t {
	case TabDashboard:
		return "Dashboard"
	case TabSessions:
		return "Sessions"
	case TabProjects:
		return "Projects"
	case TabAgents:
		return "Agents"
	default:
		return "Unknown"
	}
}

// Model is the main TUI model
type Model struct {
	agg         *aggregator.Aggregator
	tmuxClient  *tmux.Client
	width       int
	height      int
	activeTab   Tab
	sessionList list.Model
	projectList list.Model
	agentList   list.Model
	err         error
	lastRefresh time.Time
	quitting    bool
}

// SessionItem represents a session in the list
type SessionItem struct {
	Session   aggregator.TmuxSession
	Status    tmux.Status
	AgentType string
}

func (i SessionItem) Title() string {
	name := i.Session.Name
	if i.Session.AgentName != "" {
		name = i.Session.AgentName
	}
	return name
}

func (i SessionItem) Description() string {
	parts := []string{}
	if i.Session.ProjectPath != "" {
		parts = append(parts, filepath.Base(i.Session.ProjectPath))
	}
	if i.Session.AgentType != "" {
		parts = append(parts, i.Session.AgentType)
	}
	parts = append(parts, string(i.Status))
	return strings.Join(parts, " • ")
}

func (i SessionItem) FilterValue() string {
	return i.Session.Name + " " + i.Session.ProjectPath
}

// ProjectItem represents a project in the list
type ProjectItem struct {
	Path           string
	Name           string
	HasTandemonium bool
	TaskStats      *struct {
		Todo       int
		InProgress int
		Done       int
	}
}

func (i ProjectItem) Title() string       { return i.Name }
func (i ProjectItem) Description() string {
	if i.TaskStats != nil {
		return fmt.Sprintf("📋 %d todo, %d in progress, %d done", i.TaskStats.Todo, i.TaskStats.InProgress, i.TaskStats.Done)
	}
	return i.Path
}
func (i ProjectItem) FilterValue() string { return i.Name + " " + i.Path }

// AgentItem represents an agent in the list
type AgentItem struct {
	Agent aggregator.Agent
}

func (i AgentItem) Title() string { return i.Agent.Name }
func (i AgentItem) Description() string {
	parts := []string{i.Agent.Program, i.Agent.Model}
	if i.Agent.UnreadCount > 0 {
		parts = append(parts, fmt.Sprintf("📬 %d unread", i.Agent.UnreadCount))
	}
	return strings.Join(parts, " • ")
}
func (i AgentItem) FilterValue() string { return i.Agent.Name + " " + i.Agent.Program }

// Key bindings
type keyMap struct {
	Tab       key.Binding
	ShiftTab  key.Binding
	Refresh   key.Binding
	Enter     key.Binding
	Quit      key.Binding
	Help      key.Binding
	Number    []key.Binding
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
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
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
		key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "dashboard")),
		key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "sessions")),
		key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "projects")),
		key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "agents")),
	},
}

// Messages
type refreshMsg struct{}
type errMsg error
type tickMsg time.Time

// New creates a new TUI model
func New(agg *aggregator.Aggregator) Model {
	// Create session list
	sessionDelegate := list.NewDefaultDelegate()
	sessionDelegate.Styles.SelectedTitle = SelectedStyle
	sessionDelegate.Styles.NormalTitle = UnselectedStyle
	sessionList := list.New([]list.Item{}, sessionDelegate, 0, 0)
	sessionList.Title = "Sessions"
	sessionList.SetShowStatusBar(false)
	sessionList.SetFilteringEnabled(true)

	// Create project list
	projectDelegate := list.NewDefaultDelegate()
	projectDelegate.Styles.SelectedTitle = SelectedStyle
	projectDelegate.Styles.NormalTitle = UnselectedStyle
	projectList := list.New([]list.Item{}, projectDelegate, 0, 0)
	projectList.Title = "Projects"
	projectList.SetShowStatusBar(false)
	projectList.SetFilteringEnabled(true)

	// Create agent list
	agentDelegate := list.NewDefaultDelegate()
	agentDelegate.Styles.SelectedTitle = SelectedStyle
	agentDelegate.Styles.NormalTitle = UnselectedStyle
	agentList := list.New([]list.Item{}, agentDelegate, 0, 0)
	agentList.Title = "Agents"
	agentList.SetShowStatusBar(false)
	agentList.SetFilteringEnabled(true)

	return Model{
		agg:         agg,
		tmuxClient:  tmux.NewClient(),
		activeTab:   TabDashboard,
		sessionList: sessionList,
		projectList: projectList,
		agentList:   agentList,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.refresh(),
		m.tick(),
	)
}

func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		if err := m.agg.Refresh(nil); err != nil {
			return errMsg(err)
		}
		return refreshMsg{}
	}
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
			m.activeTab = Tab((int(m.activeTab) + 1) % 4)
			return m, nil

		case key.Matches(msg, keys.ShiftTab):
			m.activeTab = Tab((int(m.activeTab) + 3) % 4)
			return m, nil

		case key.Matches(msg, keys.Refresh):
			return m, m.refresh()

		case key.Matches(msg, keys.Number[0]):
			m.activeTab = TabDashboard
			return m, nil
		case key.Matches(msg, keys.Number[1]):
			m.activeTab = TabSessions
			return m, nil
		case key.Matches(msg, keys.Number[2]):
			m.activeTab = TabProjects
			return m, nil
		case key.Matches(msg, keys.Number[3]):
			m.activeTab = TabAgents
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		h := m.height - 6 // Account for header and footer
		w := m.width - 4
		m.sessionList.SetSize(w, h)
		m.projectList.SetSize(w, h)
		m.agentList.SetSize(w, h)
		return m, nil

	case refreshMsg:
		m.lastRefresh = time.Now()
		m.updateLists()
		return m, nil

	case tickMsg:
		// Auto-refresh every tick
		cmds = append(cmds, m.refresh(), m.tick())
		return m, tea.Batch(cmds...)

	case errMsg:
		m.err = msg
		return m, nil
	}

	// Update active list
	var cmd tea.Cmd
	switch m.activeTab {
	case TabSessions:
		m.sessionList, cmd = m.sessionList.Update(msg)
	case TabProjects:
		m.projectList, cmd = m.projectList.Update(msg)
	case TabAgents:
		m.agentList, cmd = m.agentList.Update(msg)
	}
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) updateLists() {
	state := m.agg.GetState()

	// Update session list
	sessionItems := make([]list.Item, len(state.Sessions))
	for i, s := range state.Sessions {
		status := m.tmuxClient.DetectStatus(s.Name)
		sessionItems[i] = SessionItem{
			Session:   s,
			Status:    status,
			AgentType: s.AgentType,
		}
	}
	m.sessionList.SetItems(sessionItems)

	// Update project list
	projectItems := make([]list.Item, len(state.Projects))
	for i, p := range state.Projects {
		item := ProjectItem{
			Path:           p.Path,
			Name:           filepath.Base(p.Path),
			HasTandemonium: p.HasTandemonium,
		}
		if p.TaskStats != nil {
			item.TaskStats = &struct {
				Todo       int
				InProgress int
				Done       int
			}{
				Todo:       p.TaskStats.Todo,
				InProgress: p.TaskStats.InProgress,
				Done:       p.TaskStats.Done,
			}
		}
		projectItems[i] = item
	}
	m.projectList.SetItems(projectItems)

	// Update agent list
	agentItems := make([]list.Item, len(state.Agents))
	for i, a := range state.Agents {
		agentItems[i] = AgentItem{Agent: a}
	}
	m.agentList.SetItems(agentItems)
}

// View renders the model
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.width == 0 {
		return "Loading..."
	}

	// Build header
	header := m.renderHeader()

	// Build content based on active tab
	var content string
	switch m.activeTab {
	case TabDashboard:
		content = m.renderDashboard()
	case TabSessions:
		content = m.sessionList.View()
	case TabProjects:
		content = m.projectList.View()
	case TabAgents:
		content = m.agentList.View()
	}

	// Build footer
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		content,
		footer,
	)
}

func (m Model) renderHeader() string {
	title := TitleStyle.Render("⚡ Vauxhall")

	// Render tabs
	tabs := make([]string, 4)
	for i := 0; i < 4; i++ {
		tab := Tab(i)
		style := TabStyle
		if tab == m.activeTab {
			style = ActiveTabStyle
		}
		tabs[i] = style.Render(fmt.Sprintf("%d %s", i+1, tab.String()))
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Center, tabs...)

	return lipgloss.JoinHorizontal(lipgloss.Center,
		title,
		strings.Repeat(" ", 4),
		tabBar,
	) + "\n"
}

func (m Model) renderFooter() string {
	help := HelpKeyStyle.Render("tab") + HelpDescStyle.Render(" switch • ") +
		HelpKeyStyle.Render("r") + HelpDescStyle.Render(" refresh • ") +
		HelpKeyStyle.Render("q") + HelpDescStyle.Render(" quit")

	lastUpdate := ""
	if !m.lastRefresh.IsZero() {
		lastUpdate = LabelStyle.Render(fmt.Sprintf("Updated %s ago", time.Since(m.lastRefresh).Round(time.Second)))
	}

	return lipgloss.JoinHorizontal(lipgloss.Center,
		help,
		strings.Repeat(" ", m.width-lipgloss.Width(help)-lipgloss.Width(lastUpdate)-4),
		lastUpdate,
	)
}

func (m Model) renderDashboard() string {
	state := m.agg.GetState()

	// Stats row
	statsStyle := PanelStyle.Copy().Width(m.width/4 - 2)

	projectStats := statsStyle.Render(
		TitleStyle.Render(fmt.Sprintf("%d", len(state.Projects))) + "\n" +
			LabelStyle.Render("Projects"),
	)

	sessionStats := statsStyle.Render(
		TitleStyle.Render(fmt.Sprintf("%d", len(state.Sessions))) + "\n" +
			LabelStyle.Render("Sessions"),
	)

	agentStats := statsStyle.Render(
		TitleStyle.Render(fmt.Sprintf("%d", len(state.Agents))) + "\n" +
			LabelStyle.Render("Agents"),
	)

	// Count active sessions
	activeCount := 0
	for _, s := range state.Sessions {
		status := m.tmuxClient.DetectStatus(s.Name)
		if status == tmux.StatusRunning || status == tmux.StatusWaiting {
			activeCount++
		}
	}
	activeStats := statsStyle.Render(
		TitleStyle.Render(fmt.Sprintf("%d", activeCount)) + "\n" +
			LabelStyle.Render("Active"),
	)

	statsRow := lipgloss.JoinHorizontal(lipgloss.Top,
		projectStats, sessionStats, agentStats, activeStats,
	)

	// Recent sessions
	recentTitle := SubtitleStyle.Render("Recent Sessions")
	var recentSessions []string
	for i, s := range state.Sessions {
		if i >= 5 {
			break
		}
		status := m.tmuxClient.DetectStatus(s.Name)
		name := s.Name
		if s.AgentName != "" {
			name = s.AgentName
		}
		line := fmt.Sprintf("  %s %s %s",
			StatusIndicator(string(status)),
			name,
			LabelStyle.Render(filepath.Base(s.ProjectPath)),
		)
		recentSessions = append(recentSessions, line)
	}
	if len(recentSessions) == 0 {
		recentSessions = append(recentSessions, LabelStyle.Render("  No sessions"))
	}

	// Recent agents
	agentsTitle := SubtitleStyle.Render("Registered Agents")
	var recentAgents []string
	for i, a := range state.Agents {
		if i >= 5 {
			break
		}
		line := fmt.Sprintf("  %s %s • %s",
			AgentBadge(a.Program),
			a.Name,
			LabelStyle.Render(filepath.Base(a.ProjectPath)),
		)
		recentAgents = append(recentAgents, line)
	}
	if len(recentAgents) == 0 {
		recentAgents = append(recentAgents, LabelStyle.Render("  No agents registered"))
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		statsRow,
		"",
		recentTitle,
		strings.Join(recentSessions, "\n"),
		"",
		agentsTitle,
		strings.Join(recentAgents, "\n"),
	)
}
