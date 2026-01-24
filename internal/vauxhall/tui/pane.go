package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/mcp"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/tmux"
	"github.com/mistakeknot/vauxpraudemonium/pkg/toolpane"
)

// VauxhallPane implements toolpane.Pane for the Vauxhall tool
type VauxhallPane struct {
	agg          aggregatorAPI
	tmuxClient   statusClient
	statusCache  map[string]cachedStatus
	statusTTL    time.Duration
	now          func() time.Time
	width        int
	height       int
	activeTab    Tab
	sessionList  list.Model
	agentList    list.Model
	mcpList      list.Model
	mcpProject   string
	showMCP      bool
	groupExpanded map[string]bool
	lastRefresh  time.Time
	err          error
}

// NewPane creates a new VauxhallPane
func NewPane(agg aggregatorAPI) *VauxhallPane {
	// Create session list
	sessionDelegate := list.NewDefaultDelegate()
	sessionDelegate.Styles.SelectedTitle = SelectedStyle
	sessionDelegate.Styles.NormalTitle = UnselectedStyle
	sessionList := list.New([]list.Item{}, sessionDelegate, 0, 0)
	sessionList.Title = "Sessions"
	sessionList.SetShowStatusBar(false)
	sessionList.SetFilteringEnabled(true)

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

	return &VauxhallPane{
		agg:           agg,
		tmuxClient:    tmux.NewClient(),
		statusCache:   make(map[string]cachedStatus),
		statusTTL:     2 * time.Second,
		now:           time.Now,
		activeTab:     TabDashboard,
		sessionList:   sessionList,
		agentList:     agentList,
		mcpList:       mcpList,
		groupExpanded: map[string]bool{},
	}
}

// Init initializes the pane with context
func (p *VauxhallPane) Init(ctx toolpane.Context) tea.Cmd {
	p.width = ctx.Width
	p.height = ctx.Height
	p.updateListSizes()
	return p.refresh()
}

// Update handles messages
func (p *VauxhallPane) Update(msg tea.Msg, ctx toolpane.Context) (toolpane.Pane, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Tab):
			p.activeTab = Tab((int(p.activeTab) + 1) % 3)
			return p, nil

		case key.Matches(msg, keys.ShiftTab):
			p.activeTab = Tab((int(p.activeTab) + 2) % 3)
			return p, nil

		case key.Matches(msg, keys.Refresh):
			return p, p.refresh()

		case key.Matches(msg, keys.Toggle):
			if p.activeTab == TabSessions || p.activeTab == TabAgents {
				var selected list.Item
				if p.activeTab == TabSessions {
					selected = p.sessionList.SelectedItem()
				} else {
					selected = p.agentList.SelectedItem()
				}
				if header, ok := selected.(GroupHeaderItem); ok {
					p.toggleGroup(p.activeTab, header.ProjectPath)
					p.updateLists(ctx.ProjectPath)
					return p, nil
				}
			}
			if p.activeTab == TabDashboard && p.showMCP {
				if item, ok := p.mcpList.SelectedItem().(MCPItem); ok {
					if item.Status.Status == mcp.StatusRunning {
						p.err = p.agg.StopMCP(p.mcpProject, item.Status.Component)
					} else {
						p.err = p.agg.StartMCP(context.Background(), p.mcpProject, item.Status.Component)
					}
					return p, p.refresh()
				}
			}
			return p, nil

		case key.Matches(msg, keys.ToggleMCP):
			if p.activeTab == TabDashboard {
				p.showMCP = !p.showMCP
				if p.showMCP && ctx.ProjectPath != "" {
					p.mcpProject = ctx.ProjectPath
					p.updateMCPList()
				}
				return p, nil
			}
			return p, nil

		case key.Matches(msg, keys.Number[0]):
			p.activeTab = TabDashboard
			return p, nil
		case key.Matches(msg, keys.Number[1]):
			p.activeTab = TabSessions
			return p, nil
		case key.Matches(msg, keys.Number[2]):
			p.activeTab = TabAgents
			return p, nil
		}

	case refreshMsg:
		p.lastRefresh = time.Now()
		p.updateLists(ctx.ProjectPath)
		return p, nil
	}

	// Update active list
	var cmd tea.Cmd
	switch p.activeTab {
	case TabSessions:
		p.sessionList, cmd = p.sessionList.Update(msg)
	case TabAgents:
		p.agentList, cmd = p.agentList.Update(msg)
	}
	cmds = append(cmds, cmd)

	return p, tea.Batch(cmds...)
}

// View renders the pane
func (p *VauxhallPane) View(ctx toolpane.Context) string {
	if p.width == 0 {
		return "Loading..."
	}

	var content string
	switch p.activeTab {
	case TabDashboard:
		content = p.renderDashboard()
	case TabSessions:
		content = p.sessionList.View()
	case TabAgents:
		content = p.agentList.View()
	}
	if p.activeTab == TabDashboard && p.showMCP {
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", p.mcpList.View())
	}

	return content
}

// Name returns the tool name
func (p *VauxhallPane) Name() string {
	return "Vauxhall"
}

// SubTabs returns the tool's internal tabs
func (p *VauxhallPane) SubTabs() []string {
	return []string{"Dashboard", "Sessions", "Agents"}
}

// ActiveSubTab returns current sub-tab index
func (p *VauxhallPane) ActiveSubTab() int {
	return int(p.activeTab)
}

// SetSubTab switches to a sub-tab
func (p *VauxhallPane) SetSubTab(index int) tea.Cmd {
	if index >= 0 && index < 3 {
		p.activeTab = Tab(index)
	}
	return nil
}

// NeedsProject returns true if tool requires project context
func (p *VauxhallPane) NeedsProject() bool {
	return false // Can show all projects
}

func (p *VauxhallPane) refresh() tea.Cmd {
	return func() tea.Msg {
		if err := p.agg.Refresh(context.Background()); err != nil {
			return errMsg(err)
		}
		return refreshMsg{}
	}
}

func (p *VauxhallPane) updateListSizes() {
	p.sessionList.SetSize(p.width, p.height)
	p.agentList.SetSize(p.width, p.height)
	p.mcpList.SetSize(p.width, p.height/2)
}

func (p *VauxhallPane) statusForSession(name string) tmux.Status {
	if p.tmuxClient == nil {
		return tmux.StatusUnknown
	}
	if p.statusTTL <= 0 {
		return p.tmuxClient.DetectStatus(name)
	}
	now := time.Now()
	if p.now != nil {
		now = p.now()
	}
	if cached, ok := p.statusCache[name]; ok {
		if now.Sub(cached.at) < p.statusTTL {
			return cached.status
		}
	}
	status := p.tmuxClient.DetectStatus(name)
	p.statusCache[name] = cachedStatus{status: status, at: now}
	return status
}

func (p *VauxhallPane) toggleGroup(tab Tab, projectPath string) {
	if p.groupExpanded == nil {
		p.groupExpanded = map[string]bool{}
	}
	key := groupKey(tab, projectPath)
	current := p.groupExpanded[key]
	if !current {
		p.groupExpanded[key] = true
		return
	}
	p.groupExpanded[key] = false
}

func (p *VauxhallPane) isGroupExpanded(tab Tab, projectPath string) bool {
	if p.groupExpanded == nil {
		p.groupExpanded = map[string]bool{}
	}
	key := groupKey(tab, projectPath)
	expanded, ok := p.groupExpanded[key]
	if !ok {
		return true
	}
	return expanded
}

func (p *VauxhallPane) updateLists(projectPath string) {
	state := p.agg.GetState()

	// Update session list
	sessionItems := make([]list.Item, 0, len(state.Sessions))
	statusByAgent := map[string]tmux.Status{}
	for _, s := range state.Sessions {
		if projectPath != "" && s.ProjectPath != projectPath {
			continue
		}
		status := p.statusForSession(s.Name)
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
	p.sessionList.SetItems(p.groupSessionItems(sessionItems))

	// Update agent list
	agentItems := make([]list.Item, 0, len(state.Agents))
	for _, a := range state.Agents {
		if projectPath != "" && a.ProjectPath != projectPath {
			continue
		}
		agentItems = append(agentItems, AgentItem{Agent: a})
	}
	p.agentList.SetItems(p.groupAgentItems(agentItems))
}

func (p *VauxhallPane) groupSessionItems(items []list.Item) []list.Item {
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
	out := make([]list.Item, 0, len(items)+len(grouped))
	for key, groupItems := range grouped {
		name := filepath.Base(key)
		if key == "" {
			name = "Unassigned"
		}
		expanded := p.isGroupExpanded(TabSessions, key)
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

func (p *VauxhallPane) groupAgentItems(items []list.Item) []list.Item {
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
	out := make([]list.Item, 0, len(items)+len(grouped))
	for key, groupItems := range grouped {
		name := filepath.Base(key)
		if key == "" {
			name = "Unassigned"
		}
		expanded := p.isGroupExpanded(TabAgents, key)
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

func (p *VauxhallPane) updateMCPList() {
	state := p.agg.GetState()
	statuses := state.MCP[p.mcpProject]
	items := make([]list.Item, len(statuses))
	for i, s := range statuses {
		items[i] = MCPItem{Status: s}
	}
	p.mcpList.SetItems(items)
}

func (p *VauxhallPane) renderDashboard() string {
	state := p.agg.GetState()

	// Stats row
	statsStyle := PanelStyle.Copy().Width(p.width/4 - 2)

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
		status := p.statusForSession(s.Name)
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
		status := p.statusForSession(s.Name)
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
