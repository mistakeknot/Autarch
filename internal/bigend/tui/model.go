package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/bigend/mcp"
	"github.com/mistakeknot/autarch/internal/bigend/tmux"
	"github.com/mistakeknot/autarch/internal/icdata"
	"github.com/mistakeknot/autarch/pkg/timeout"
	shared "github.com/mistakeknot/autarch/pkg/tui"
)

// Re-export shared styles for convenience
var (
	TitleStyle         = shared.TitleStyle
	SubtitleStyle      = shared.SubtitleStyle
	LabelStyle         = shared.LabelStyle
	SelectedStyle      = shared.SelectedStyle
	UnselectedStyle    = shared.UnselectedStyle
	PanelStyle         = shared.PanelStyle
	PaneFocusedStyle   = shared.PaneFocusedStyle
	PaneUnfocusedStyle = shared.PaneUnfocusedStyle
	TabStyle           = shared.TabStyle
	ActiveTabStyle     = shared.ActiveTabStyle
	HelpKeyStyle       = shared.HelpKeyStyle
	HelpDescStyle      = shared.HelpDescStyle
	StatusRunning      = shared.StatusRunning
	StatusWaiting      = shared.StatusWaiting
	StatusIdle         = shared.StatusIdle
	StatusError        = shared.StatusError
)

// Use shared functions
var (
	StatusIndicator = shared.StatusIndicator
	AgentBadge      = shared.AgentBadge
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
	PaneTerminal
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
	Statuses map[icdata.UnifiedStatus]bool
}

func parseFilter(input string) FilterState {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return FilterState{Raw: ""}
	}
	terms := []string{}
	statuses := map[icdata.UnifiedStatus]bool{}
	for _, token := range strings.Fields(strings.ToLower(raw)) {
		if strings.HasPrefix(token, "!") {
			switch strings.TrimPrefix(token, "!") {
			case "running", "active":
				statuses[icdata.StatusActive] = true
				continue
			case "waiting", "idle":
				statuses[icdata.StatusWaiting] = true
				continue
			case "blocked":
				statuses[icdata.StatusBlocked] = true
				continue
			case "error":
				statuses[icdata.StatusErr] = true
				continue
			case "done":
				statuses[icdata.StatusDone] = true
				continue
			case "unknown":
				statuses[icdata.StatusUnknown] = true
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

func filterAgentItems(items []list.Item, state FilterState, statusByAgent map[string]icdata.UnifiedStatus) []list.Item {
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
				status = icdata.StatusUnknown
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
	tmuxCapture *tmux.Client // For terminal capture (separate from status detection)
	width         int
	height        int
	activeTab     Tab
	activePane    Pane
	buildInfo     string
	sessionList   list.Model
	projectsList  list.Model
	agentList     list.Model
	mcpList       list.Model
	mcpProject    string
	showMCP       bool
	showTerminal  bool          // Terminal preview pane visible
	terminalPane  *TerminalPane // Terminal preview component
	filterActive  bool
	filterInput   textinput.Model
	filterStates  map[Tab]FilterState
	groupExpanded map[string]bool
	promptMode    promptMode
	promptInput   textinput.Model
	promptSess    *aggregator.TmuxSession
	err           error
	lastRefresh   time.Time
	quitting      bool
	keys          shared.CommonKeys
	helpOverlay   shared.HelpOverlay
}

// SessionItem represents a session in the list
type SessionItem struct {
	Session   aggregator.TmuxSession
	Status    icdata.UnifiedStatus
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
	parts = append(parts, i.Status.String())
	return strings.Join(parts, " • ")
}

func (i SessionItem) FilterValue() string {
	return i.Session.Name + " " + i.Session.ProjectPath
}

// ProjectItem represents a project in the list
type ProjectItem struct {
	Path        string
	Name        string
	HasColdwine bool
	RunCount     int
	BlockedCount int
	KernelError  string
	TaskStats   *struct {
		Todo       int
		InProgress int
		Done       int
	}
}

func (i ProjectItem) Title() string {
	name := i.Name
	if i.KernelError != "" {
		name = "! " + name
	}
	if i.BlockedCount > 0 {
		name = fmt.Sprintf("%s %s", name,
			StatusError.Render(fmt.Sprintf("[%d blocked]", i.BlockedCount)))
	} else if i.RunCount > 0 {
		name = fmt.Sprintf("%s [%d]", name, i.RunCount)
	}
	return name
}
func (i ProjectItem) Description() string {
	if i.TaskStats != nil {
		return fmt.Sprintf("%d todo, %d in progress, %d done", i.TaskStats.Todo, i.TaskStats.InProgress, i.TaskStats.Done)
	}
	return i.Path
}
func (i ProjectItem) FilterValue() string { return i.Name + " " + i.Path }

// GroupHeaderItem represents a grouped header in session/agent lists.
type GroupHeaderItem struct {
	ProjectPath string
	Name        string
	Count       int
	Expanded    bool
}

func (i GroupHeaderItem) Title() string {
	if i.Count > 0 {
		return fmt.Sprintf("%s (%d)", i.Name, i.Count)
	}
	return i.Name
}

func (i GroupHeaderItem) Description() string { return "" }
func (i GroupHeaderItem) FilterValue() string { return i.Name + " " + i.ProjectPath }

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

func (m *Model) groupSessionItemsByProject(items []list.Item) []list.Item {
	if len(items) == 0 {
		return items
	}
	grouped := map[string][]SessionItem{}
	for _, item := range items {
		session, ok := item.(SessionItem)
		if !ok {
			continue
		}
		grouped[session.Session.ProjectPath] = append(grouped[session.Session.ProjectPath], session)
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]list.Item, 0, len(items)+len(keys))
	for _, key := range keys {
		name := filepath.Base(key)
		if key == "" {
			name = "Unassigned"
		}
		groupItems := grouped[key]
		expanded := m.isGroupExpanded(TabSessions, key)
		out = append(out, GroupHeaderItem{
			ProjectPath: key,
			Name:        name,
			Count:       len(groupItems),
			Expanded:    expanded,
		})
		if expanded {
			for _, session := range groupItems {
				out = append(out, session)
			}
		}
	}
	return out
}

func (m *Model) isGroupExpanded(tab Tab, projectPath string) bool {
	if m.groupExpanded == nil {
		m.groupExpanded = map[string]bool{}
	}
	key := groupKey(tab, projectPath)
	expanded, ok := m.groupExpanded[key]
	if !ok {
		return true
	}
	return expanded
}

func (m *Model) toggleGroup(tab Tab, projectPath string) {
	if m.groupExpanded == nil {
		m.groupExpanded = map[string]bool{}
	}
	key := groupKey(tab, projectPath)
	current := m.groupExpanded[key]
	if !current {
		m.groupExpanded[key] = true
		return
	}
	m.groupExpanded[key] = false
}

func groupKey(tab Tab, projectPath string) string {
	prefix := "sessions"
	if tab == TabAgents {
		prefix = "agents"
	}
	return prefix + ":" + projectPath
}

func (m *Model) groupAgentItemsByProject(items []list.Item) []list.Item {
	if len(items) == 0 {
		return items
	}
	grouped := map[string][]AgentItem{}
	for _, item := range items {
		agent, ok := item.(AgentItem)
		if !ok {
			continue
		}
		grouped[agent.Agent.ProjectPath] = append(grouped[agent.Agent.ProjectPath], agent)
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]list.Item, 0, len(items)+len(keys))
	for _, key := range keys {
		name := filepath.Base(key)
		if key == "" {
			name = "Unassigned"
		}
		groupItems := grouped[key]
		expanded := m.isGroupExpanded(TabAgents, key)
		out = append(out, GroupHeaderItem{
			ProjectPath: key,
			Name:        name,
			Count:       len(groupItems),
			Expanded:    expanded,
		})
		if expanded {
			for _, agent := range groupItems {
				out = append(out, agent)
			}
		}
	}
	return out
}

// Key bindings
type keyMap struct {
	FocusLeft      key.Binding
	FocusRight     key.Binding
	New            key.Binding
	Rename         key.Binding
	Fork           key.Binding
	Restart        key.Binding
	Attach         key.Binding
	ToggleMCP      key.Binding
	ToggleTerminal key.Binding
}

var keys = keyMap{
	FocusLeft: key.NewBinding(
		key.WithKeys("["),
		key.WithHelp("[", "focus projects"),
	),
	FocusRight: key.NewBinding(
		key.WithKeys("]"),
		key.WithHelp("]", "focus main"),
	),
	New: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new"),
	),
	Rename: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "rename"),
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
	ToggleTerminal: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "preview"),
	),
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

	tmuxCapture := tmux.NewClient()

	return Model{
		agg:         agg,
		tmuxCapture: tmuxCapture,
		activeTab:   TabDashboard,
		activePane:   PaneProjects,
		buildInfo:    buildInfo,
		sessionList:  sessionList,
		projectsList: projectsList,
		agentList:    agentList,
		mcpList:      mcpList,
		terminalPane: NewTerminalPane(tmuxCapture),
		filterInput:  filterInput,
		filterStates: map[Tab]FilterState{
			TabSessions: {Raw: ""},
			TabAgents:   {Raw: ""},
		},
		groupExpanded: map[string]bool{},
		promptInput:   promptInput,
		keys:          shared.NewCommonKeys(),
		helpOverlay:   shared.NewHelpOverlay(),
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

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.refresh(),
		m.tick(),
	)
}

func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.TODO(), timeout.HTTPDefault)
		defer cancel()
		if err := m.agg.Refresh(ctx); err != nil {
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
		if m.helpOverlay.Visible {
			switch {
			case key.Matches(msg, m.keys.Help), key.Matches(msg, m.keys.Back):
				m.helpOverlay.Toggle()
			}
			return m, nil
		}

		if cmd := shared.HandleCommon(msg, m.keys); cmd != nil {
			return m, cmd
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
		case key.Matches(msg, m.keys.Toggle):
			if m.activeTab == TabSessions || m.activeTab == TabAgents {
				var selected list.Item
				if m.activeTab == TabSessions {
					selected = m.sessionList.SelectedItem()
				} else {
					selected = m.agentList.SelectedItem()
				}
				if header, ok := selected.(GroupHeaderItem); ok {
					m.toggleGroup(m.activeTab, header.ProjectPath)
					m.updateLists()
					return m, nil
				}
			}

		case key.Matches(msg, m.keys.TabCycle):
			m.stopFilterEditing()
			if msg.String() == "shift+tab" {
				m.activeTab = Tab((int(m.activeTab) + 2) % 3)
			} else {
				m.activeTab = Tab((int(m.activeTab) + 1) % 3)
			}
			m.activePane = PaneMain
			m.syncFilterInputForTab(m.activeTab)
			return m, nil

		case key.Matches(msg, m.keys.Refresh):
			return m, m.refresh()

		case key.Matches(msg, keys.FocusLeft):
			switch m.activePane {
			case PaneTerminal:
				m.activePane = PaneMain
				m.terminalPane.SetFocused(false)
			case PaneMain:
				m.activePane = PaneProjects
			}
			return m, nil

		case key.Matches(msg, keys.FocusRight):
			switch m.activePane {
			case PaneProjects:
				m.activePane = PaneMain
			case PaneMain:
				if m.showTerminal {
					m.activePane = PaneTerminal
					m.terminalPane.SetFocused(true)
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Search):
			if m.activeTab == TabSessions || m.activeTab == TabAgents {
				m.filterActive = true
				m.syncFilterInputForTab(m.activeTab)
				m.filterInput.Focus()
				return m, nil
			}
			return m, nil

		case key.Matches(msg, m.keys.Toggle):
			if m.activeTab == TabDashboard && m.showMCP {
				if item, ok := m.mcpList.SelectedItem().(MCPItem); ok {
					if item.Status.Status == mcp.StatusRunning {
						m.err = m.agg.StopMCP(m.mcpProject, item.Status.Component)
					} else {
						ctx, cancel := context.WithTimeout(context.TODO(), timeout.HTTPDefault)
						m.err = m.agg.StartMCP(ctx, m.mcpProject, item.Status.Component)
						cancel()
					}
					return m, m.refresh()
				}
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

		case key.Matches(msg, keys.ToggleTerminal):
			if m.activeTab == TabSessions {
				m.showTerminal = !m.showTerminal
				if m.showTerminal {
					// Start terminal preview for selected session
					if session, ok := m.selectedSession(); ok {
						cmd := m.terminalPane.SetSession(session.Name)
						m.terminalPane.SetFocused(false)
						return m, tea.Batch(cmd, TickTerminal(200*time.Millisecond))
					}
				} else {
					m.terminalPane.SetSession("")
					m.activePane = PaneMain
				}
				return m, nil
			}
			return m, nil

		case msg.String() == "ctrl+left" || msg.String() == "ctrl+pgup":
			m.stopFilterEditing()
			switch m.activeTab {
			case TabDashboard:
				m.activeTab = TabAgents
			case TabSessions:
				m.activeTab = TabDashboard
			case TabAgents:
				m.activeTab = TabSessions
			}
			m.activePane = PaneMain
			m.syncFilterInputForTab(m.activeTab)
			return m, nil
		case msg.String() == "ctrl+right" || msg.String() == "ctrl+pgdown":
			m.stopFilterEditing()
			switch m.activeTab {
			case TabDashboard:
				m.activeTab = TabSessions
			case TabSessions:
				m.activeTab = TabAgents
			case TabAgents:
				m.activeTab = TabDashboard
			}
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
		// Adjust for terminal pane if visible
		if m.showTerminal {
			termW := rightW / 2
			rightW = rightW - termW - 2 // gap
			m.terminalPane.SetSize(termW, rightH)
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

	case terminalContentMsg, terminalTickMsg:
		// Forward to terminal pane
		if m.terminalPane != nil && m.showTerminal {
			var cmd tea.Cmd
			m.terminalPane, cmd = m.terminalPane.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case shared.ToggleHelpMsg:
		m.helpOverlay.Toggle()
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
			Path:        p.Path,
			Name:        filepath.Base(p.Path),
			HasColdwine: p.HasColdwine,
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
		// Kernel enrichment: active/blocked run counts and errors
		if state.Kernel != nil {
			if runs, ok := state.Kernel.Runs[p.Path]; ok {
				for _, r := range runs {
					us := icdata.UnifyStatus(r.Status)
					switch us {
					case icdata.StatusActive:
						item.RunCount++
					case icdata.StatusBlocked:
						item.BlockedCount++
					}
				}
			}
			if errMsg, ok := state.Kernel.Metrics.KernelErrors[p.Path]; ok {
				item.KernelError = errMsg
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

	// Update session list — status comes from aggregator's UnifiedState
	sessionItems := make([]list.Item, 0, len(state.Sessions))
	statusByAgent := map[string]icdata.UnifiedStatus{}
	for _, s := range state.Sessions {
		if selectedProject != "" && s.ProjectPath != selectedProject {
			continue
		}
		status := s.UnifiedState
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
	filteredSessions := filterSessionItems(sessionItems, m.filterStateFor(TabSessions))
	m.sessionList.SetItems(m.groupSessionItemsByProject(filteredSessions))

	// Update agent list
	agentItems := make([]list.Item, 0, len(state.Agents))
	for _, a := range state.Agents {
		if selectedProject != "" && a.ProjectPath != selectedProject {
			continue
		}
		agentItems = append(agentItems, AgentItem{Agent: a})
	}
	filteredAgents := filterAgentItems(agentItems, m.filterStateFor(TabAgents), statusByAgent)
	m.agentList.SetItems(m.groupAgentItemsByProject(filteredAgents))
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

	// Build main pane content (with optional terminal preview)
	var mainContent string
	if m.showTerminal && m.activeTab == TabSessions && m.terminalPane != nil {
		mainContent = m.renderThreePane(m.projectsList.View(), content, m.terminalPane.View())
	} else {
		mainContent = m.renderTwoPane(m.projectsList.View(), content)
	}

	// Build footer
	footer := m.renderFooter()

	parts := []string{header}
	if m.helpOverlay.Visible {
		parts = append(parts, m.helpOverlay.Render(m.keys, m.helpExtras(), m.width), footer)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}
	if filterLine != "" {
		parts = append(parts, filterLine)
	}
	parts = append(parts, mainContent, m.renderPrompt(), footer)
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
		HelpKeyStyle.Render("e") + HelpDescStyle.Render(" rename • ") +
		HelpKeyStyle.Render("k") + HelpDescStyle.Render(" restart • ") +
		HelpKeyStyle.Render("f") + HelpDescStyle.Render(" fork • ") +
		HelpKeyStyle.Render("a") + HelpDescStyle.Render(" attach • ") +
		HelpKeyStyle.Render("p") + HelpDescStyle.Render(" preview • ") +
		HelpKeyStyle.Render("m") + HelpDescStyle.Render(" mcp • ") +
		HelpKeyStyle.Render("enter") + HelpDescStyle.Render(" toggle • ") +
		HelpKeyStyle.Render("ctrl+c") + HelpDescStyle.Render(" quit")
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

func (m Model) helpExtras() []shared.HelpBinding {
	return []shared.HelpBinding{
		shared.HelpBindingFromKey(keys.FocusLeft),
		shared.HelpBindingFromKey(keys.FocusRight),
		shared.HelpBindingFromKey(keys.New),
		shared.HelpBindingFromKey(keys.Rename),
		shared.HelpBindingFromKey(keys.Fork),
		shared.HelpBindingFromKey(keys.Restart),
		shared.HelpBindingFromKey(keys.Attach),
		shared.HelpBindingFromKey(keys.ToggleMCP),
		shared.HelpBindingFromKey(keys.ToggleTerminal),
	}
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

func (m Model) renderThreePane(left, middle, right string) string {
	width := m.width
	gap := 2

	// Calculate widths: 20% left, 40% middle, 40% right
	leftW := width / 5
	remaining := width - leftW - gap
	middleW := remaining / 2
	rightW := remaining - middleW - gap

	// Ensure minimum widths
	minPane := 15
	if leftW < minPane || middleW < minPane || rightW < minPane {
		// Fall back to two panes if too narrow
		return m.renderTwoPane(left, middle)
	}

	leftStyle := PaneUnfocusedStyle
	middleStyle := PaneUnfocusedStyle
	rightStyle := PaneUnfocusedStyle

	switch m.activePane {
	case PaneProjects:
		leftStyle = PaneFocusedStyle
	case PaneMain:
		middleStyle = PaneFocusedStyle
	case PaneTerminal:
		rightStyle = PaneFocusedStyle
	}

	leftView := leftStyle.Width(leftW).Render(left)
	middleView := middleStyle.Width(middleW).Render(middle)
	rightView := rightStyle.Width(rightW).Render(right)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftView, "  ", middleView, "  ", rightView)
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
	statsStyle := PanelStyle.Copy().Width(m.width/5 - 2)

	projectCount := len(state.Projects)
	projectLabel := "Projects"
	if state.Kernel != nil && len(state.Kernel.Metrics.KernelErrors) > 0 {
		kernelCount := 0
		for _, p := range state.Projects {
			if p.HasIntercore {
				kernelCount++
			}
		}
		okCount := kernelCount - len(state.Kernel.Metrics.KernelErrors)
		projectLabel = fmt.Sprintf("Projects (%d/%d)", okCount, kernelCount)
	}
	projectStats := statsStyle.Render(
		TitleStyle.Render(fmt.Sprintf("%d", projectCount)) + "\n" +
			LabelStyle.Render(projectLabel),
	)

	sessionStats := statsStyle.Render(
		TitleStyle.Render(fmt.Sprintf("%d", len(state.Sessions))) + "\n" +
			LabelStyle.Render("Sessions"),
	)

	agentStats := statsStyle.Render(
		TitleStyle.Render(fmt.Sprintf("%d", len(state.Agents))) + "\n" +
			LabelStyle.Render("Agents"),
	)

	// Kernel metrics stats
	var runsStats, dispatchStats string
	if state.Kernel != nil {
		km := state.Kernel.Metrics
		runsStats = statsStyle.Render(
			TitleStyle.Render(fmt.Sprintf("%d", km.ActiveRuns)) + "\n" +
				LabelStyle.Render("Active Runs"),
		)
		blockedStyle := LabelStyle
		if km.BlockedAgents > 0 {
			blockedStyle = StatusError
		}
		dispatchStats = statsStyle.Render(
			TitleStyle.Render(fmt.Sprintf("%d", km.ActiveDispatches)) + "\n" +
				blockedStyle.Render(fmt.Sprintf("Dispatches (%d blocked)", km.BlockedAgents)),
		)
	} else {
		// Count active sessions as before when no kernel
		activeCount := 0
		for _, s := range state.Sessions {
			if s.UnifiedState == icdata.StatusActive || s.UnifiedState == icdata.StatusWaiting {
				activeCount++
			}
		}
		runsStats = statsStyle.Render(
			TitleStyle.Render(fmt.Sprintf("%d", activeCount)) + "\n" +
				LabelStyle.Render("Active"),
		)
		dispatchStats = ""
	}

	statsItems := []string{projectStats, sessionStats, agentStats, runsStats}
	if dispatchStats != "" {
		statsItems = append(statsItems, dispatchStats)
	}
	statsRow := lipgloss.JoinHorizontal(lipgloss.Top, statsItems...)

	// Build sections
	sections := []string{statsRow, ""}

	// Active Runs section (kernel)
	if state.Kernel != nil {
		runsTitle := SubtitleStyle.Render("Active Runs")
		var runLines []string
		for projPath, runs := range state.Kernel.Runs {
			projName := filepath.Base(projPath)
			for _, r := range runs {
				if r.Status == "" || r.Status == "done" || r.Status == "cancelled" {
					continue
				}
				goal := r.Goal
				if len(goal) > 40 {
					goal = goal[:37] + "..."
				}
				id := r.ID
				if len(id) > 8 {
					id = id[:8]
				}
				line := fmt.Sprintf("  %s %s %s %s %s",
					shared.UnifiedStatusSymbol(shared.UnifyStatusForRender(r.Status)),
					LabelStyle.Render(id),
					projName,
					TitleStyle.Render(r.Phase),
					goal,
				)
				runLines = append(runLines, line)
			}
		}
		if len(runLines) > 0 {
			sections = append(sections, runsTitle, strings.Join(runLines, "\n"), "")
		} else {
			sections = append(sections, runsTitle, LabelStyle.Render("  No active runs"), "")
		}
	}

	// Recent sessions
	recentTitle := SubtitleStyle.Render("Recent Sessions")
	var recentSessions []string
	for i, s := range state.Sessions {
		if i >= 5 {
			break
		}
		name := s.Name
		if s.AgentName != "" {
			name = s.AgentName
		}
		line := fmt.Sprintf("  %s %s %s",
			shared.UnifiedStatusIndicator(s.UnifiedState),
			name,
			LabelStyle.Render(filepath.Base(s.ProjectPath)),
		)
		recentSessions = append(recentSessions, line)
	}
	if len(recentSessions) == 0 {
		recentSessions = append(recentSessions, LabelStyle.Render("  No sessions"))
	}
	sections = append(sections, recentTitle, strings.Join(recentSessions, "\n"), "")

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
	sections = append(sections, agentsTitle, strings.Join(recentAgents, "\n"), "")

	// Activity feed
	if len(state.Activities) > 0 {
		actTitle := SubtitleStyle.Render("Recent Activity")
		var actLines []string
		for i, a := range state.Activities {
			if i >= 10 {
				break
			}
			prefix := LabelStyle.Render("[T]") // tmux default
			switch a.Source {
			case "kernel":
				prefix = shared.StatusRunning.Render("[K]")
			case "intermute":
				prefix = StatusWaiting.Render("[M]")
			}
			line := fmt.Sprintf("  %s %s", prefix, a.Summary)
			actLines = append(actLines, line)
		}
		sections = append(sections, actTitle, strings.Join(actLines, "\n"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}
