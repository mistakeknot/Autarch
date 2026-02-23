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
	PaneRunList
	PaneRunDetail
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
	showRunPane   bool          // Kernel run list+detail pane visible
	runList       RunListState  // Kernel run list state
	filterActive  bool
	filterInput   textinput.Model
	filterStates  map[Tab]FilterState
	groupExpanded map[string]bool
	promptMode    promptMode
	promptInput   textinput.Model
	promptSess    *aggregator.TmuxSession
	err             error
	lastRefresh     time.Time
	quitting        bool
	keys            shared.CommonKeys
	helpOverlay     shared.HelpOverlay
	resizeCoalescer *shared.ResizeCoalescer
	dashCache       *sectionCache
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
	ToggleRuns     key.Binding
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
	ToggleRuns: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "runs"),
	),
}

// Messages
type refreshMsg struct{}
type errMsg error
type tickMsg time.Time
type resizeTickMsg struct{}

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
		promptInput:     promptInput,
		keys:            shared.NewCommonKeys(),
		helpOverlay:     shared.NewHelpOverlay(),
		resizeCoalescer: shared.NewResizeCoalescer(),
		dashCache:       newSectionCache(),
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

		// Run pane navigation
		if m.showRunPane && (m.activePane == PaneRunList || m.activePane == PaneRunDetail) {
			switch msg.Type {
			case tea.KeyUp:
				if m.runList.SelectedIdx > 0 {
					m.runList.SelectedIdx--
				}
				return m, nil
			case tea.KeyDown:
				if m.runList.SelectedIdx < len(m.runList.Runs)-1 {
					m.runList.SelectedIdx++
				}
				return m, nil
			}
			switch msg.String() {
			case "a":
				m.runList.ShowAll = !m.runList.ShowAll
				m.updateRunList()
				return m, nil
			case "l", "right":
				if m.activePane == PaneRunList {
					m.activePane = PaneRunDetail
				}
				return m, nil
			case "h", "left":
				if m.activePane == PaneRunDetail {
					m.activePane = PaneRunList
				} else if m.activePane == PaneRunList {
					m.activePane = PaneProjects
				}
				return m, nil
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
			case PaneRunDetail:
				m.activePane = PaneRunList
			case PaneRunList:
				m.activePane = PaneProjects
			case PaneMain:
				if m.showRunPane {
					m.activePane = PaneRunDetail
				} else {
					m.activePane = PaneProjects
				}
			}
			return m, nil

		case key.Matches(msg, keys.FocusRight):
			switch m.activePane {
			case PaneProjects:
				if m.showRunPane {
					m.activePane = PaneRunList
				} else {
					m.activePane = PaneMain
				}
			case PaneRunList:
				m.activePane = PaneRunDetail
			case PaneRunDetail:
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

		case key.Matches(msg, keys.ToggleRuns):
			if m.activeTab == TabDashboard {
				m.showRunPane = !m.showRunPane
				if m.showRunPane {
					m.updateRunList()
					m.activePane = PaneRunList
				} else {
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
		action := m.resizeCoalescer.Receive(msg, time.Now())
		if action == shared.ActionApply {
			return m.applyResize(msg), nil
		}
		return m, tea.Tick(m.resizeCoalescer.Delay(), func(time.Time) tea.Msg {
			return resizeTickMsg{}
		})

	case resizeTickMsg:
		if pending := m.resizeCoalescer.Tick(time.Now()); pending != nil {
			return m.applyResize(*pending), nil
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
	if m.showRunPane {
		m.updateRunList()
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

func (m *Model) updateRunList() {
	state := m.agg.GetState()
	projectPath := m.selectedProjectPath()
	m.runList.ProjectPath = projectPath
	m.runList.Runs = buildRunList(state, projectPath, m.runList.ShowAll)
	if m.runList.SelectedIdx >= len(m.runList.Runs) {
		m.runList.SelectedIdx = max(0, len(m.runList.Runs)-1)
	}
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

	// Build main pane content (with optional terminal preview or run pane)
	var mainContent string
	if m.showRunPane && m.activeTab == TabDashboard {
		mainContent = m.renderRunPaneLayout(content)
	} else if m.showTerminal && m.activeTab == TabSessions && m.terminalPane != nil {
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
		HelpKeyStyle.Render("r") + HelpDescStyle.Render(" runs • ") +
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
		shared.HelpBindingFromKey(keys.ToggleRuns),
	}
}

// applyResize applies a (possibly coalesced) window size to all child panes.
func (m Model) applyResize(msg tea.WindowSizeMsg) Model {
	m.dashCache.invalidateAll()
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
	if m.showTerminal {
		termW := rightW / 2
		rightW = rightW - termW - 2
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
	return m
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
