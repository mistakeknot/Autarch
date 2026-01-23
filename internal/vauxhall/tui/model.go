package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/aggregator"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/mcp"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/tmux"
)

type aggregatorAPI interface {
	GetState() aggregator.State
	Refresh(ctx context.Context) error
	NewSession(name, projectPath, agentType string) error
	RestartSession(name, projectPath, agentType string) error
	RenameSession(oldName, newName string) error
	ForkSession(name, projectPath, agentType string) error
	AttachSession(name string) error
	StartMCP(ctx context.Context, projectPath, component string) error
	StopMCP(projectPath, component string) error
}

type statusClient interface {
	DetectStatus(name string) tmux.Status
}

// Tab represents a view tab
type Tab int

const (
	TabDashboard Tab = iota
	TabSessions
	TabAgents
)

func (t Tab) String() string {
	switch t {
	case TabDashboard:
		return "Dashboard"
	case TabSessions:
		return "Sessions"
	case TabAgents:
		return "Agents"
	default:
		return "Unknown"
	}
}

type Pane int

const (
	PaneProjects Pane = iota
	PaneMain
)

type promptMode int

const (
	promptNone promptMode = iota
	promptNewSession
	promptRenameSession
	promptForkSession
)

type FilterState struct {
	Raw      string
	Terms    []string
	Statuses map[tmux.Status]bool
}

type cachedStatus struct {
	status tmux.Status
	at     time.Time
}

func parseFilter(input string) FilterState {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return FilterState{Raw: ""}
	}
	terms := []string{}
	statuses := map[tmux.Status]bool{}
	for _, token := range strings.Fields(strings.ToLower(raw)) {
		if strings.HasPrefix(token, "!") {
			switch strings.TrimPrefix(token, "!") {
			case "running":
				statuses[tmux.StatusRunning] = true
				continue
			case "waiting":
				statuses[tmux.StatusWaiting] = true
				continue
			case "idle":
				statuses[tmux.StatusIdle] = true
				continue
			case "error":
				statuses[tmux.StatusError] = true
				continue
			case "unknown":
				statuses[tmux.StatusUnknown] = true
				continue
			default:
				token = strings.TrimPrefix(token, "!")
			}
		}
		if token != "" {
			terms = append(terms, token)
		}
	}
	if len(statuses) == 0 {
		statuses = nil
	}
	return FilterState{Raw: raw, Terms: terms, Statuses: statuses}
}

func (m *Model) filterStateFor(tab Tab) FilterState {
	if m.filterStates == nil {
		return FilterState{Raw: ""}
	}
	if state, ok := m.filterStates[tab]; ok {
		return state
	}
	return FilterState{Raw: ""}
}

func (m *Model) setFilterState(tab Tab, state FilterState) {
	if m.filterStates == nil {
		m.filterStates = map[Tab]FilterState{}
	}
	if state.Raw == "" && len(state.Terms) == 0 && len(state.Statuses) == 0 {
		state = FilterState{Raw: ""}
	}
	m.filterStates[tab] = state
}

func (m *Model) syncFilterInputForTab(tab Tab) {
	if tab != TabSessions && tab != TabAgents {
		m.filterInput.SetValue("")
		return
	}
	state := m.filterStateFor(tab)
	m.filterInput.SetValue(state.Raw)
}

func isFilterEditKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyRunes, tea.KeySpace, tea.KeyBackspace, tea.KeyDelete, tea.KeyCtrlW, tea.KeyCtrlU:
		return true
	default:
		return false
	}
}

func filterSessionItems(items []list.Item, state FilterState) []list.Item {
	if state.Raw == "" {
		return items
	}
	filtered := make([]list.Item, 0, len(items))
	for _, item := range items {
		sessionItem, ok := item.(SessionItem)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		if len(state.Statuses) > 0 && !state.Statuses[sessionItem.Status] {
			continue
		}
		haystack := strings.ToLower(sessionItem.Title() + " " + sessionItem.Description())
		matches := true
		for _, term := range state.Terms {
			if !strings.Contains(haystack, term) {
				matches = false
				break
			}
		}
		if matches {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterAgentItems(items []list.Item, state FilterState, statusByAgent map[string]tmux.Status) []list.Item {
	if state.Raw == "" {
		return items
	}
	filtered := make([]list.Item, 0, len(items))
	for _, item := range items {
		agentItem, ok := item.(AgentItem)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		if len(state.Statuses) > 0 {
			status, ok := statusByAgent[agentItem.Agent.Name]
			if !ok {
				status = tmux.StatusUnknown
			}
			if !state.Statuses[status] {
				continue
			}
		}
		haystack := strings.ToLower(agentItem.Title() + " " + agentItem.Description())
		matches := true
		for _, term := range state.Terms {
			if !strings.Contains(haystack, term) {
				matches = false
				break
			}
		}
		if matches {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// Model is the main TUI model
type Model struct {
	agg         aggregatorAPI
	tmuxClient  statusClient
	statusCache map[string]cachedStatus
	statusTTL   time.Duration
	now         func() time.Time
	width       int
	height      int
	activeTab   Tab
	activePane  Pane
	buildInfo   string
	sessionList list.Model
	projectsList list.Model
	agentList   list.Model
	mcpList     list.Model
	mcpProject  string
	showMCP     bool
	filterActive bool
	filterInput  textinput.Model
	filterStates map[Tab]FilterState
	promptMode  promptMode
	promptInput textinput.Model
	promptSess  *aggregator.TmuxSession
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
	FocusLeft key.Binding
	FocusRight key.Binding
	Filter    key.Binding
	New       key.Binding
	Rename    key.Binding
	Fork      key.Binding
	Restart   key.Binding
	Attach    key.Binding
	ToggleMCP key.Binding
	Toggle    key.Binding
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
		key.WithKeys("ctrl+r", "R"),
		key.WithHelp("ctrl+r", "refresh"),
	),
	FocusLeft: key.NewBinding(
		key.WithKeys("["),
		key.WithHelp("[", "focus projects"),
	),
	FocusRight: key.NewBinding(
		key.WithKeys("]"),
		key.WithHelp("]", "focus main"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	New: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new"),
	),
	Rename: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "rename"),
	),
	Fork: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "fork"),
	),
	Restart: key.NewBinding(
		key.WithKeys("k"),
		key.WithHelp("k", "restart"),
	),
	Attach: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "attach"),
	),
	ToggleMCP: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "mcp"),
	),
	Toggle: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle"),
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
		key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "agents")),
	},
}

// Messages
type refreshMsg struct{}
type errMsg error
type tickMsg time.Time

// New creates a new TUI model
func New(agg aggregatorAPI, buildInfo string) Model {
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
	projectsList := list.New([]list.Item{}, projectDelegate, 0, 0)
	projectsList.Title = "Projects"
	projectsList.SetShowStatusBar(false)
	projectsList.SetFilteringEnabled(true)

	// Create agent list
	agentDelegate := list.NewDefaultDelegate()
	agentDelegate.Styles.SelectedTitle = SelectedStyle
	agentDelegate.Styles.NormalTitle = UnselectedStyle
	agentList := list.New([]list.Item{}, agentDelegate, 0, 0)
	agentList.Title = "Agents"
	agentList.SetShowStatusBar(false)
	agentList.SetFilteringEnabled(true)

	mcpDelegate := list.NewDefaultDelegate()
	mcpDelegate.Styles.SelectedTitle = SelectedStyle
	mcpDelegate.Styles.NormalTitle = UnselectedStyle
	mcpList := list.New([]list.Item{}, mcpDelegate, 0, 0)
	mcpList.Title = "MCP"
	mcpList.SetShowStatusBar(false)
	mcpList.SetFilteringEnabled(false)

	filterInput := textinput.New()
	filterInput.Placeholder = "filter"
	filterInput.Prompt = "/ "
	filterInput.CharLimit = 256

	promptInput := textinput.New()
	promptInput.Placeholder = ""
	promptInput.CharLimit = 80
	promptInput.Width = 40

	return Model{
		agg:         agg,
		tmuxClient:  tmux.NewClient(),
		statusCache: make(map[string]cachedStatus),
		statusTTL:   2 * time.Second,
		now:         time.Now,
		activeTab:   TabDashboard,
		activePane:  PaneProjects,
		buildInfo:   buildInfo,
		sessionList: sessionList,
		projectsList: projectsList,
		agentList:   agentList,
		mcpList:     mcpList,
		filterInput: filterInput,
		filterStates: map[Tab]FilterState{
			TabSessions: {Raw: ""},
			TabAgents:   {Raw: ""},
		},
		promptInput: promptInput,
	}
}

func (m Model) withFilterActive(value string) Model {
	m.filterActive = true
	m.filterInput.SetValue(value)
	m.filterInput.Focus()
	m.setFilterState(m.activeTab, parseFilter(value))
	return m
}

func (m *Model) stopFilterEditing() {
	if !m.filterActive {
		return
	}
	if m.activeTab == TabSessions || m.activeTab == TabAgents {
		m.setFilterState(m.activeTab, parseFilter(m.filterInput.Value()))
	}
	m.filterInput.Blur()
	m.filterActive = false
}

func (m *Model) statusForSession(name string) tmux.Status {
	if m.tmuxClient == nil {
		return tmux.StatusUnknown
	}
	if m.statusTTL <= 0 {
		return m.tmuxClient.DetectStatus(name)
	}
	now := time.Now()
	if m.now != nil {
		now = m.now()
	}
	if cached, ok := m.statusCache[name]; ok {
		if now.Sub(cached.at) < m.statusTTL {
			return cached.status
		}
	}
	status := m.tmuxClient.DetectStatus(name)
	m.statusCache[name] = cachedStatus{status: status, at: now}
	return status
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
		if err := m.agg.Refresh(context.Background()); err != nil {
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
		if m.promptMode != promptNone {
			switch msg.Type {
			case tea.KeyEsc:
				m.promptMode = promptNone
				m.promptSess = nil
				m.promptInput.SetValue("")
				return m, nil
			case tea.KeyEnter:
				value := strings.TrimSpace(m.promptInput.Value())
				if value == "" || m.promptSess == nil {
					m.err = fmt.Errorf("missing input")
					m.promptMode = promptNone
					m.promptSess = nil
					m.promptInput.SetValue("")
					return m, nil
				}
				switch m.promptMode {
				case promptNewSession:
					m.err = m.agg.NewSession(value, m.promptSess.ProjectPath, m.promptSess.AgentType)
				case promptRenameSession:
					m.err = m.agg.RenameSession(m.promptSess.Name, value)
				case promptForkSession:
					m.err = m.agg.ForkSession(value, m.promptSess.ProjectPath, m.promptSess.AgentType)
				}
				m.promptMode = promptNone
				m.promptSess = nil
				m.promptInput.SetValue("")
				return m, m.refresh()
			}
			var cmd tea.Cmd
			m.promptInput, cmd = m.promptInput.Update(msg)
			return m, cmd
		}
		// Global key handling
		switch {
		case key.Matches(msg, keys.Quit):
			m.quitting = true
			return m, tea.Quit
		}

		if m.filterActive {
			if msg.Type == tea.KeyEsc {
				m.filterInput.SetValue("")
				m.filterInput.Blur()
				m.filterActive = false
				if m.activeTab == TabSessions || m.activeTab == TabAgents {
					m.setFilterState(m.activeTab, FilterState{Raw: ""})
				}
				m.updateLists()
				return m, nil
			}
			if msg.Type == tea.KeyEnter {
				m.stopFilterEditing()
				return m, nil
			}
			if isFilterEditKey(msg) {
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				m.setFilterState(m.activeTab, parseFilter(m.filterInput.Value()))
				m.updateLists()
				return m, cmd
			}
		}

		switch {

		case key.Matches(msg, keys.Tab):
			m.stopFilterEditing()
			m.activeTab = Tab((int(m.activeTab) + 1) % 3)
			m.activePane = PaneMain
			m.syncFilterInputForTab(m.activeTab)
			return m, nil

		case key.Matches(msg, keys.ShiftTab):
			m.stopFilterEditing()
			m.activeTab = Tab((int(m.activeTab) + 2) % 3)
			m.activePane = PaneMain
			m.syncFilterInputForTab(m.activeTab)
			return m, nil

		case key.Matches(msg, keys.Refresh):
			return m, m.refresh()

		case key.Matches(msg, keys.FocusLeft):
			m.activePane = PaneProjects
			return m, nil

		case key.Matches(msg, keys.FocusRight):
			m.activePane = PaneMain
			return m, nil

		case key.Matches(msg, keys.Filter):
			if m.activeTab == TabSessions || m.activeTab == TabAgents {
				m.filterActive = true
				m.syncFilterInputForTab(m.activeTab)
				m.filterInput.Focus()
				return m, nil
			}
			return m, nil

		case key.Matches(msg, keys.New):
			if m.activeTab == TabSessions {
				if session, ok := m.selectedSession(); ok {
					m.promptMode = promptNewSession
					m.promptSess = &session
					m.promptInput.Placeholder = "new session name"
					m.promptInput.SetValue(session.Name + "-new")
					m.promptInput.Focus()
					return m, nil
				}
			}
			return m, nil

		case key.Matches(msg, keys.Rename):
			if m.activeTab == TabSessions {
				if session, ok := m.selectedSession(); ok {
					m.promptMode = promptRenameSession
					m.promptSess = &session
					m.promptInput.Placeholder = "rename session"
					m.promptInput.SetValue("")
					m.promptInput.Focus()
					return m, nil
				}
			}
			return m, nil

		case key.Matches(msg, keys.Fork):
			if m.activeTab == TabSessions {
				if session, ok := m.selectedSession(); ok {
					m.promptMode = promptForkSession
					m.promptSess = &session
					m.promptInput.Placeholder = "fork name"
					m.promptInput.SetValue(session.Name + "-fork")
					m.promptInput.Focus()
					return m, nil
				}
			}
			return m, nil

		case key.Matches(msg, keys.Restart):
			if m.activeTab == TabSessions {
				if session, ok := m.selectedSession(); ok {
					if err := m.agg.RestartSession(session.Name, session.ProjectPath, session.AgentType); err != nil {
						m.err = err
					}
					return m, m.refresh()
				}
			}
			return m, nil

		case key.Matches(msg, keys.Attach):
			if m.activeTab == TabSessions {
				if session, ok := m.selectedSession(); ok {
					if err := m.agg.AttachSession(session.Name); err != nil {
						m.err = err
					}
					return m, nil
				}
			}
			return m, nil

		case key.Matches(msg, keys.ToggleMCP):
			if m.activeTab == TabDashboard {
				m.showMCP = !m.showMCP
				if m.showMCP {
					if project, ok := m.selectedProject(); ok {
						m.mcpProject = project.Path
						m.updateMCPList()
					}
				}
				return m, nil
			}
			return m, nil

		case key.Matches(msg, keys.Toggle):
			if m.activeTab == TabDashboard && m.showMCP {
				if item, ok := m.mcpList.SelectedItem().(MCPItem); ok {
					if item.Status.Status == mcp.StatusRunning {
						m.err = m.agg.StopMCP(m.mcpProject, item.Status.Component)
					} else {
						m.err = m.agg.StartMCP(context.Background(), m.mcpProject, item.Status.Component)
					}
					return m, m.refresh()
				}
			}
			return m, nil

		case key.Matches(msg, keys.Number[0]):
			m.stopFilterEditing()
			m.activeTab = TabDashboard
			m.activePane = PaneMain
			m.syncFilterInputForTab(m.activeTab)
			return m, nil
		case key.Matches(msg, keys.Number[1]):
			m.stopFilterEditing()
			m.activeTab = TabSessions
			m.activePane = PaneMain
			m.syncFilterInputForTab(m.activeTab)
			return m, nil
		case key.Matches(msg, keys.Number[2]):
			m.stopFilterEditing()
			m.activeTab = TabAgents
			m.activePane = PaneMain
			m.syncFilterInputForTab(m.activeTab)
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		h := m.height - 6 // Account for header and footer
		leftW, rightW, _ := m.paneWidths()
		leftH := h
		rightH := h
		if leftW > 0 && rightW > 0 {
			leftW = max(1, leftW-2)
			rightW = max(1, rightW-2)
			leftH = max(1, h-2)
			rightH = max(1, h-2)
		}
		if leftW > 0 {
			m.projectsList.SetSize(leftW, leftH)
		} else {
			m.projectsList.SetSize(m.width, h)
		}
		if rightW > 0 {
			m.sessionList.SetSize(rightW, rightH)
			m.agentList.SetSize(rightW, rightH)
			m.mcpList.SetSize(rightW, rightH/2)
		} else {
			m.sessionList.SetSize(m.width, h)
			m.agentList.SetSize(m.width, h)
			m.mcpList.SetSize(m.width, h/2)
		}
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
	if m.activePane == PaneProjects {
		m.projectsList, cmd = m.projectsList.Update(msg)
	} else {
		switch m.activeTab {
		case TabSessions:
			m.sessionList, cmd = m.sessionList.Update(msg)
		case TabAgents:
			m.agentList, cmd = m.agentList.Update(msg)
		}
	}
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) selectedSession() (aggregator.TmuxSession, bool) {
	item, ok := m.sessionList.SelectedItem().(SessionItem)
	if !ok {
		return aggregator.TmuxSession{}, false
	}
	return item.Session, true
}

func (m Model) selectedProject() (ProjectItem, bool) {
	item, ok := m.projectsList.SelectedItem().(ProjectItem)
	if !ok {
		return ProjectItem{}, false
	}
	return item, true
}

func (m Model) selectedProjectPath() string {
	item, ok := m.projectsList.SelectedItem().(ProjectItem)
	if !ok {
		return ""
	}
	return item.Path
}

func (m *Model) selectProjectPath(path string) {
	items := m.projectsList.Items()
	for i, item := range items {
		project, ok := item.(ProjectItem)
		if !ok {
			continue
		}
		if project.Path == path {
			m.projectsList.Select(i)
			return
		}
	}
	if len(items) > 0 {
		m.projectsList.Select(0)
	}
}

func (m *Model) updateLists() {
	state := m.agg.GetState()
	prevProject := m.selectedProjectPath()

	// Update project list
	projectItems := make([]list.Item, 0, len(state.Projects)+1)
	projectItems = append(projectItems, ProjectItem{Path: "", Name: "All Projects"})
	for _, p := range state.Projects {
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
		projectItems = append(projectItems, item)
	}
	m.projectsList.SetItems(projectItems)
	m.selectProjectPath(prevProject)
	if m.showMCP {
		m.updateMCPList()
	}

	selectedProject := m.selectedProjectPath()

	// Update session list
	sessionItems := make([]list.Item, 0, len(state.Sessions))
	statusByAgent := map[string]tmux.Status{}
	for _, s := range state.Sessions {
		if selectedProject != "" && s.ProjectPath != selectedProject {
			continue
		}
		status := m.statusForSession(s.Name)
		if s.AgentName != "" {
			if _, ok := statusByAgent[s.AgentName]; !ok {
				statusByAgent[s.AgentName] = status
			}
		}
		sessionItems = append(sessionItems, SessionItem{
			Session:   s,
			Status:    status,
			AgentType: s.AgentType,
		})
	}
	m.sessionList.SetItems(filterSessionItems(sessionItems, m.filterStateFor(TabSessions)))

	// Update agent list
	agentItems := make([]list.Item, 0, len(state.Agents))
	for _, a := range state.Agents {
		if selectedProject != "" && a.ProjectPath != selectedProject {
			continue
		}
		agentItems = append(agentItems, AgentItem{Agent: a})
	}
	m.agentList.SetItems(filterAgentItems(agentItems, m.filterStateFor(TabAgents), statusByAgent))
}

func (m *Model) updateMCPList() {
	state := m.agg.GetState()
	statuses := state.MCP[m.mcpProject]
	items := make([]list.Item, len(statuses))
	for i, s := range statuses {
		items[i] = MCPItem{Status: s}
	}
	m.mcpList.SetItems(items)
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
	filterLine := m.renderFilterLine()

	// Build content based on active tab
	var content string
	switch m.activeTab {
	case TabDashboard:
		content = m.renderDashboard()
	case TabSessions:
		content = m.sessionList.View()
	case TabAgents:
		content = m.agentList.View()
	}
	if m.activeTab == TabDashboard && m.showMCP {
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", m.mcpList.View())
	}

	// Build footer
	footer := m.renderFooter()

	parts := []string{header}
	if filterLine != "" {
		parts = append(parts, filterLine)
	}
	parts = append(parts,
		m.renderTwoPane(m.projectsList.View(), content),
		m.renderPrompt(),
		footer,
	)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderHeader() string {
	title := TitleStyle.Render("⚡ Vauxhall")
	if m.buildInfo != "" {
		title = title + " " + LabelStyle.Render(m.buildInfo)
	}

	// Render tabs
	tabs := make([]string, 3)
	for i := 0; i < 3; i++ {
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

func (m Model) renderFilterLine() string {
	if m.activeTab != TabSessions && m.activeTab != TabAgents {
		return ""
	}
	state := m.filterStateFor(m.activeTab)
	if state.Raw == "" && m.filterInput.Value() == "" {
		return ""
	}
	if m.filterActive {
		return LabelStyle.Render("Filter: ") + m.filterInput.View()
	}
	return LabelStyle.Render("Filter: " + state.Raw)
}

func (m Model) renderFooter() string {
	help := HelpKeyStyle.Render("tab") + HelpDescStyle.Render(" switch • ") +
		HelpKeyStyle.Render("ctrl+r") + HelpDescStyle.Render(" refresh • ") +
		HelpKeyStyle.Render("n") + HelpDescStyle.Render(" new • ") +
		HelpKeyStyle.Render("r") + HelpDescStyle.Render(" rename • ") +
		HelpKeyStyle.Render("k") + HelpDescStyle.Render(" restart • ") +
		HelpKeyStyle.Render("f") + HelpDescStyle.Render(" fork • ") +
		HelpKeyStyle.Render("a") + HelpDescStyle.Render(" attach • ") +
		HelpKeyStyle.Render("m") + HelpDescStyle.Render(" mcp • ") +
		HelpKeyStyle.Render("space") + HelpDescStyle.Render(" toggle • ") +
		HelpKeyStyle.Render("q") + HelpDescStyle.Render(" quit")
	if m.filterActive {
		help += HelpDescStyle.Render(" • ") + HelpKeyStyle.Render("esc/enter") + HelpDescStyle.Render(" exit filter")
	}

	lastUpdate := ""
	if !m.lastRefresh.IsZero() {
		lastUpdate = LabelStyle.Render(fmt.Sprintf("Updated %s ago", time.Since(m.lastRefresh).Round(time.Second)))
	}

	padding := m.width - lipgloss.Width(help) - lipgloss.Width(lastUpdate) - 4
	if padding < 1 {
		padding = 1
	}
	return lipgloss.JoinHorizontal(lipgloss.Center,
		help,
		strings.Repeat(" ", padding),
		lastUpdate,
	)
}

func (m Model) paneWidths() (int, int, bool) {
	width := m.width
	if width <= 0 {
		return 0, 0, true
	}
	minLeft := 20
	minRight := 30
	gap := 2
	if width < minLeft+minRight+gap {
		return 0, width, true
	}
	left := width / 3
	if left < minLeft {
		left = minLeft
	}
	if width-left < minRight+gap {
		left = width - minRight - gap
	}
	right := width - left - gap
	return left, right, false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) renderTwoPane(left, right string) string {
	leftW, rightW, single := m.paneWidths()
	if single {
		return right
	}
	leftStyle := PaneUnfocusedStyle
	rightStyle := PaneUnfocusedStyle
	if m.activePane == PaneProjects {
		leftStyle = PaneFocusedStyle
	} else {
		rightStyle = PaneFocusedStyle
	}
	leftView := leftStyle.Width(leftW).Render(left)
	rightView := rightStyle.Width(rightW).Render(right)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftView, "  ", rightView)
}

func (m Model) renderPrompt() string {
	if m.promptMode == promptNone {
		return ""
	}
	label := ""
	switch m.promptMode {
	case promptNewSession:
		label = "New session"
	case promptRenameSession:
		label = "Rename session"
	case promptForkSession:
		label = "Fork session"
	}
	return lipgloss.JoinHorizontal(lipgloss.Left,
		LabelStyle.Render(label+": "),
		m.promptInput.View(),
	)
}

// MCPItem represents a MCP component in the list.
type MCPItem struct {
	Status mcp.ComponentStatus
}

func (i MCPItem) Title() string       { return i.Status.Component }
func (i MCPItem) Description() string { return string(i.Status.Status) }
func (i MCPItem) FilterValue() string { return i.Status.Component }

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
		status := m.statusForSession(s.Name)
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
		status := m.statusForSession(s.Name)
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
