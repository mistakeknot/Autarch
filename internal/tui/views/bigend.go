package views

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/bigend/discovery"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/autarch"
	"github.com/mistakeknot/autarch/pkg/intercore"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// FocusPane indicates which sub-pane is focused within the document area.
type FocusPane int

const (
	FocusSessions FocusPane = iota
	FocusTasks
)

// BigendView displays sessions and agent overview
type BigendView struct {
	client   *autarch.Client
	iclient  *intercore.Client // optional — nil when ic unavailable
	sessions []autarch.Session
	selected int
	width    int
	height   int
	loading  bool

	// Ready tasks
	readyTasks   []tasks.TaskProposal
	taskSelected int
	focusPane    FocusPane

	// Intercore data (loaded alongside sessions).
	runs       []intercore.Run
	dispatches []intercore.Dispatch

	// Project discovery + navigation
	scanner       *discovery.Scanner
	projects      []discovery.Project
	activeProject int // -1 = all projects, 0..N = specific project index
	projectEpoch  int // incremented on every project switch; stale loads discarded

	// Callbacks
	onTaskSelect func(task tasks.TaskProposal) tea.Cmd

	// Shell layout for unified 3-pane layout
	shell *pkgtui.ShellLayout
	// Chat panel for interactive input
	chatPanel *pkgtui.ChatPanel
	// Chat handler for Bigend-specific context
	chatHandler *BigendChatHandler
}

// NewBigendView creates a new Bigend view
func NewBigendView(client *autarch.Client) *BigendView {
	chatPanel := pkgtui.NewChatPanel()
	chatPanel.SetComposerPlaceholder("Type / for commands, or ask about tasks...")
	chatPanel.SetComposerHint("enter send  tab focus  F3 panes")
	chatHandler := NewBigendChatHandler()
	chatPanel.SetHandler(chatHandler)

	return &BigendView{
		client:        client,
		focusPane:     FocusTasks,
		activeProject: -1, // "All Projects" by default
		shell:         pkgtui.NewShellLayout(),
		chatPanel:     chatPanel,
		chatHandler:   chatHandler,
	}
}

// SetAgentSelector sets the shared agent selector.
func (v *BigendView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.chatPanel.SetAgentSelector(selector)
}

// SetAgentName sets the selected agent name (satisfies interface).
func (v *BigendView) SetAgentName(name string) {}

// SetChatSettings sets chat settings on the chat panel.
func (v *BigendView) SetChatSettings(settings pkgtui.ChatSettings) {
	v.chatPanel.SetSettings(settings)
}

// SetIntercore sets the Intercore client for dispatch monitoring.
func (v *BigendView) SetIntercore(ic *intercore.Client) {
	v.iclient = ic
}

// ClearInput clears the chat composer (for ctrl+c soft cancel).
func (v *BigendView) ClearInput() {
	v.chatPanel.ClearComposer()
}

// SetScanner sets the project discovery scanner.
func (v *BigendView) SetScanner(s *discovery.Scanner) {
	v.scanner = s
}

// SetReadyTasks updates the ready tasks queue.
func (v *BigendView) SetReadyTasks(taskList []tasks.TaskProposal) {
	v.readyTasks = tasks.GetReadyTasks(taskList)
	if v.taskSelected >= len(v.readyTasks) {
		v.taskSelected = max(0, len(v.readyTasks)-1)
	}
}

// SetTaskSelectCallback sets the callback for task selection.
func (v *BigendView) SetTaskSelectCallback(cb func(tasks.TaskProposal) tea.Cmd) {
	v.onTaskSelect = cb
}

// sessionsLoadedMsg is sent when sessions are loaded
type sessionsLoadedMsg struct {
	sessions []autarch.Session
	err      error
	epoch    int // project epoch at time of request
}

// sessionCreatedMsg is sent after creating a new session
type sessionCreatedMsg struct {
	session autarch.Session
	err     error
}

// projectsLoadedMsg carries discovered projects from the scanner.
type projectsLoadedMsg struct {
	projects []discovery.Project
}

// bigendRunsLoadedMsg carries runs from Intercore.
type bigendRunsLoadedMsg struct {
	runs  []intercore.Run
	epoch int
}

// dispatchesLoadedMsg carries dispatches from Intercore.
type dispatchesLoadedMsg struct {
	dispatches []intercore.Dispatch
	epoch      int
}

// Init implements View
func (v *BigendView) Init() tea.Cmd {
	return tea.Batch(v.loadProjects(), v.loadSessions(), v.loadRuns(), v.loadDispatches())
}

func (v *BigendView) loadProjects() tea.Cmd {
	if v.scanner == nil {
		return nil
	}
	s := v.scanner
	return func() tea.Msg {
		projects, _ := s.Scan()
		return projectsLoadedMsg{projects: projects}
	}
}

func (v *BigendView) loadSessions() tea.Cmd {
	// Use ProjectClient for goroutine-safe project scoping
	c := v.client
	if v.activeProject >= 0 && v.activeProject < len(v.projects) {
		c = v.client.ProjectClient(v.projects[v.activeProject].Name)
	}
	epoch := v.projectEpoch
	return func() tea.Msg {
		sessions, err := c.ListSessions("")
		return sessionsLoadedMsg{sessions: sessions, err: err, epoch: epoch}
	}
}

func (v *BigendView) loadRuns() tea.Cmd {
	if v.iclient == nil {
		return nil
	}
	ic := v.iclient
	epoch := v.projectEpoch
	// Snapshot filter state for goroutine safety
	var filterPath string
	if v.activeProject >= 0 && v.activeProject < len(v.projects) {
		filterPath = v.projects[v.activeProject].Path
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		runs, err := ic.RunList(ctx, false)
		if err != nil {
			return bigendRunsLoadedMsg{epoch: epoch}
		}
		if filterPath != "" {
			var filtered []intercore.Run
			for _, r := range runs {
				if normalizePath(r.ProjectDir) == filterPath {
					filtered = append(filtered, r)
				}
			}
			runs = filtered
		}
		return bigendRunsLoadedMsg{runs: runs, epoch: epoch}
	}
}

func (v *BigendView) loadDispatches() tea.Cmd {
	if v.iclient == nil {
		return nil
	}
	ic := v.iclient
	epoch := v.projectEpoch
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Fetch all dispatches — active + recent completed.
		// Dispatches are filtered at render time by matching RunIDs against
		// the project-scoped runs set (dispatches don't carry ProjectDir).
		dispatches, err := ic.DispatchList(ctx, false)
		if err != nil {
			return dispatchesLoadedMsg{epoch: epoch} // graceful degradation
		}
		return dispatchesLoadedMsg{dispatches: dispatches, epoch: epoch}
	}
}

// filteredDispatches returns dispatches belonging to the active project's runs.
// When activeProject == -1 (All), returns all dispatches unfiltered.
func (v *BigendView) filteredDispatches() []intercore.Dispatch {
	if v.activeProject < 0 || len(v.runs) == 0 {
		return v.dispatches
	}
	runIDs := make(map[string]bool, len(v.runs))
	for _, r := range v.runs {
		runIDs[r.ID] = true
	}
	var filtered []intercore.Dispatch
	for _, d := range v.dispatches {
		if runIDs[d.RunID] {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// Update implements View
func (v *BigendView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd
	if _, isKey := msg.(tea.KeyMsg); !isKey {
		v.chatPanel, cmd = v.chatPanel.Update(msg)
		if cmd != nil {
			return v, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width - 6
		v.height = msg.Height - 4 - 2
		v.shell.SetSize(v.width, v.height)
		v.chatPanel.SetSize(v.shell.RightWidth(), v.shell.Height())
		return v, nil

	case projectsLoadedMsg:
		v.projects = msg.projects
		return v, nil

	case pkgtui.SidebarSelectMsg:
		if msg.ItemID == "__all_projects" {
			v.activeProject = -1
		} else {
			for i, p := range v.projects {
				if p.Path == msg.ItemID {
					v.activeProject = i
					break
				}
			}
		}
		v.projectEpoch++ // invalidate in-flight loads from prior project
		// Reload all data with new project scope
		return v, tea.Batch(v.loadSessions(), v.loadRuns(), v.loadDispatches())

	case sessionsLoadedMsg:
		if msg.epoch != v.projectEpoch {
			return v, nil // stale load from prior project — discard
		}
		v.loading = false
		if msg.err != nil {
			v.sessions = nil
		} else {
			v.sessions = msg.sessions
		}
		return v, nil

	case bigendRunsLoadedMsg:
		if msg.epoch != v.projectEpoch {
			return v, nil
		}
		v.runs = msg.runs
		return v, nil

	case dispatchesLoadedMsg:
		if msg.epoch != v.projectEpoch {
			return v, nil
		}
		v.dispatches = msg.dispatches
		return v, nil

	case tui.DispatchCompletedMsg:
		// Refresh dispatches when one completes.
		return v, v.loadDispatches()

	case sessionCreatedMsg:
		if msg.err != nil {
			v.chatPanel.AddMessage("system", fmt.Sprintf("Failed to create session: %v", msg.err))
			return v, nil
		}
		v.chatPanel.AddMessage("system", fmt.Sprintf("Created session: %s", msg.session.Name))
		return v, v.loadSessions()

	case tea.KeyMsg:
		// Let shell handle global keys first (Tab, Shift-Tab, Ctrl+B)
		v.shell, cmd = v.shell.Update(msg)
		if cmd != nil {
			return v, cmd
		}

		// Handle view-specific keys based on focus
		switch v.shell.Focus() {
		case pkgtui.FocusSidebar:
			// Sidebar navigation handled by ShellLayout
		case pkgtui.FocusDocument:
			switch {
			case key.Matches(msg, commonKeys.NavDown):
				if v.focusPane == FocusSessions {
					if v.selected < len(v.sessions)-1 {
						v.selected++
					}
				} else {
					if v.taskSelected < len(v.readyTasks)-1 {
						v.taskSelected++
					}
				}
			case key.Matches(msg, commonKeys.NavUp):
				if v.focusPane == FocusSessions {
					if v.selected > 0 {
						v.selected--
					}
				} else {
					if v.taskSelected > 0 {
						v.taskSelected--
					}
				}
			case msg.Type == tea.KeyF3:
				// Toggle focus between tasks and sessions sub-panes
				if v.focusPane == FocusSessions {
					v.focusPane = FocusTasks
				} else {
					v.focusPane = FocusSessions
				}
			case key.Matches(msg, commonKeys.Select):
				if v.focusPane == FocusTasks && len(v.readyTasks) > 0 && v.onTaskSelect != nil {
					return v, v.onTaskSelect(v.readyTasks[v.taskSelected])
				}
			case key.Matches(msg, commonKeys.Refresh):
				v.loading = true
				return v, tea.Batch(v.loadProjects(), v.loadSessions(), v.loadRuns(), v.loadDispatches())
			}
		case pkgtui.FocusChat:
			if msg.Type == tea.KeyEnter {
				if slashCmd := v.chatPanel.SubmitInput(); slashCmd != nil {
					return v, slashCmd
				}
				return v, nil
			}
			v.chatPanel, cmd = v.chatPanel.Update(msg)
			return v, cmd
		}
	}

	return v, nil
}

// SidebarItems implements SidebarProvider — returns project list for sidebar.
func (v *BigendView) SidebarItems() []pkgtui.SidebarItem {
	var items []pkgtui.SidebarItem

	// "All Projects" sentinel at top
	allIcon := "◎"
	if v.activeProject == -1 {
		allIcon = "●"
	}
	items = append(items, pkgtui.SidebarItem{
		ID: "__all_projects", Label: "All Projects", Icon: allIcon,
	})

	for i, p := range v.projects {
		icon := projectIcon(p)
		if i == v.activeProject {
			icon = "●"
		}
		items = append(items, pkgtui.SidebarItem{
			ID: p.Path, Label: p.Name, Icon: icon,
		})
	}

	return items
}

// projectIcon returns a compact tooling indicator for a project.
func projectIcon(p discovery.Project) string {
	var parts []string
	if p.HasIntercore {
		parts = append(parts, "IC")
	}
	if p.HasColdwine {
		parts = append(parts, "CW")
	}
	if p.HasGurgeh {
		parts = append(parts, "G")
	}
	if p.HasPollard {
		parts = append(parts, "P")
	}
	if len(parts) > 0 {
		return strings.Join(parts, "·")
	}
	return "○"
}

// View implements View
func (v *BigendView) View() string {
	if v.loading {
		return pkgtui.LabelStyle.Render("Loading sessions...")
	}

	// Render dashboard as document pane, chatPanel as chat pane
	document := v.renderDashboard()
	chat := v.chatPanel.View()

	return v.shell.Render(v.SidebarItems(), document, chat)
}

// activeProjectName returns the name of the currently selected project.
func (v *BigendView) activeProjectName() string {
	if v.activeProject >= 0 && v.activeProject < len(v.projects) {
		return v.projects[v.activeProject].Name
	}
	return ""
}

func (v *BigendView) renderDashboard() string {
	var sections []string

	// Project context header
	if name := v.activeProjectName(); name != "" {
		headerStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorPrimary).
			Bold(true).
			MarginBottom(1)
		sections = append(sections, headerStyle.Render("Project: "+name))
	}

	// Main content: two panes within the document area
	docWidth := v.shell.LeftWidth()
	if docWidth <= 0 {
		docWidth = v.width / 2
	}

	if docWidth >= 80 {
		sections = append(sections, v.renderTwoPaneLayout(docWidth))
	} else {
		sections = append(sections, v.renderStackedLayout(docWidth))
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (v *BigendView) renderTwoPaneLayout(totalWidth int) string {
	leftWidth := totalWidth / 2
	rightWidth := totalWidth - leftWidth - 2

	left := v.renderTasksPane(leftWidth)
	right := v.renderSessionsPane(rightWidth)

	leftStyle := lipgloss.NewStyle().Width(leftWidth)
	rightStyle := lipgloss.NewStyle().
		Width(rightWidth).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(pkgtui.ColorMuted).
		PaddingLeft(1)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		leftStyle.Render(left),
		rightStyle.Render(right),
	)
}

func (v *BigendView) renderStackedLayout(totalWidth int) string {
	var sections []string
	sections = append(sections, v.renderTasksPane(totalWidth))
	sections = append(sections, "")
	sections = append(sections, v.renderSessionsPane(totalWidth))
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (v *BigendView) renderTasksPane(width int) string {
	var lines []string

	// Title with focus indicator
	titleStyle := pkgtui.TitleStyle
	if v.focusPane == FocusTasks {
		titleStyle = titleStyle.Underline(true)
	}
	readyCount := len(v.readyTasks)
	lines = append(lines, titleStyle.Render(fmt.Sprintf("Ready Tasks (%d)", readyCount)))
	lines = append(lines, "")

	if len(v.readyTasks) == 0 {
		lines = append(lines, pkgtui.LabelStyle.Render("No tasks ready"))
		lines = append(lines, pkgtui.LabelStyle.Render("Complete the onboarding flow to generate tasks"))
		return strings.Join(lines, "\n")
	}

	// Show ready tasks
	maxTasks := (v.height - 4) / 2
	if maxTasks < 3 {
		maxTasks = 3
	}

	for i, t := range v.readyTasks {
		if i >= maxTasks {
			remaining := len(v.readyTasks) - maxTasks
			lines = append(lines, pkgtui.LabelStyle.Render(fmt.Sprintf("  ... and %d more", remaining)))
			break
		}

		isSelected := i == v.taskSelected && v.focusPane == FocusTasks

		// Task type badge
		typeStyle := lipgloss.NewStyle().
			Background(pkgtui.ColorBgLight).
			Foreground(pkgtui.ColorFgDim).
			Padding(0, 1)

		var typeAbbrev string
		switch t.Type {
		case tasks.TaskTypeImplementation:
			typeAbbrev = "impl"
		case tasks.TaskTypeTest:
			typeAbbrev = "test"
		case tasks.TaskTypeSetup:
			typeAbbrev = "setup"
		default:
			typeAbbrev = string(t.Type)
		}

		selector := "  "
		titleStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFg)
		if isSelected {
			selector = "> "
			titleStyle = titleStyle.Bold(true).Foreground(pkgtui.ColorPrimary)
		}

		line := fmt.Sprintf("%s%s %s",
			selector,
			typeStyle.Render(typeAbbrev),
			titleStyle.Render(t.Title),
		)
		lines = append(lines, line)

		// Show epic context
		if t.EpicID != "" {
			epicStyle := pkgtui.LabelStyle.MarginLeft(4)
			lines = append(lines, epicStyle.Render(t.EpicID))
		}
	}

	return strings.Join(lines, "\n")
}

func (v *BigendView) renderSessionsPane(width int) string {
	var lines []string

	// Title with focus indicator
	titleStyle := pkgtui.TitleStyle
	if v.focusPane == FocusSessions {
		titleStyle = titleStyle.Underline(true)
	}
	sessTitle := fmt.Sprintf("Sessions (%d)", len(v.sessions))
	if name := v.activeProjectName(); name != "" {
		sessTitle = fmt.Sprintf("Sessions · %s (%d)", name, len(v.sessions))
	}
	lines = append(lines, titleStyle.Render(sessTitle))
	lines = append(lines, "")

	if len(v.sessions) == 0 {
		lines = append(lines, pkgtui.LabelStyle.Render("No sessions running"))
		lines = append(lines, pkgtui.LabelStyle.Render("Start a task to launch an agent"))
	} else {
		for i, s := range v.sessions {
			icon := v.statusIcon(s.Status)
			name := s.Name
			if name == "" {
				name = s.ID[:8]
			}

			line := fmt.Sprintf("%s %s", icon, name)
			if i == v.selected && v.focusPane == FocusSessions {
				line = pkgtui.SelectedStyle.Render("> " + line)
			} else {
				line = pkgtui.UnselectedStyle.Render("  " + line)
			}
			lines = append(lines, line)
		}
	}

	// Intercore sections (runs + dispatches)
	if v.iclient != nil {
		// Runs section
		lines = append(lines, "")
		lines = append(lines, pkgtui.SubtitleStyle.Render(fmt.Sprintf("Runs (%d)", len(v.runs))))

		if len(v.runs) == 0 {
			lines = append(lines, pkgtui.LabelStyle.Render("  No runs"))
		} else {
			for _, r := range v.runs {
				icon, color := runStatusDisplay(r.Status)
				rStyle := lipgloss.NewStyle().Foreground(color)

				idPrefix := r.ID
				if len(idPrefix) > 6 {
					idPrefix = idPrefix[:6]
				}

				goal := r.Goal
				if len(goal) > 30 {
					goal = goal[:27] + "..."
				}

				phase := ""
				if r.Phase != "" {
					phase = fmt.Sprintf(" [%s]", r.Phase)
				}

				line := fmt.Sprintf("  %s %s %s%s",
					rStyle.Render(icon), idPrefix, goal, phase)
				lines = append(lines, line)
			}
		}

		// Dispatches section (filtered by project-scoped runs)
		visibleDispatches := v.filteredDispatches()
		lines = append(lines, "")
		lines = append(lines, pkgtui.SubtitleStyle.Render(fmt.Sprintf("Dispatches (%d)", len(visibleDispatches))))

		if len(visibleDispatches) == 0 {
			lines = append(lines, pkgtui.LabelStyle.Render("  No dispatches"))
		} else {
			for _, d := range visibleDispatches {
				icon, color := dispatchStatusDisplay(d.Status)
				dispStyle := lipgloss.NewStyle().Foreground(color)

				agent := d.Agent
				if d.Name != nil && *d.Name != "" {
					agent = *d.Name
				}
				if agent == "" {
					agent = d.Type
				}

				idPrefix := d.ID
				if len(idPrefix) > 6 {
					idPrefix = idPrefix[:6]
				}

				elapsed := ""
				if d.Status == "running" {
					dur := time.Since(time.Unix(d.CreatedAt, 0))
					elapsed = fmt.Sprintf(" %s", formatDuration(dur))
				}

				line := fmt.Sprintf("  %s %s %s%s",
					dispStyle.Render(icon), idPrefix, agent, elapsed)
				lines = append(lines, line)
			}
		}
	} else {
		// Kernel unavailable — show degraded indicator
		lines = append(lines, "")
		warnStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorWarning)
		lines = append(lines, warnStyle.Render("  kernel unavailable — runs and dispatches hidden"))
	}

	return strings.Join(lines, "\n")
}

func runStatusDisplay(status string) (string, lipgloss.Color) {
	switch status {
	case "active":
		return "●", pkgtui.ColorPrimary
	case "completed":
		return "✓", pkgtui.ColorSuccess
	case "cancelled":
		return "✗", pkgtui.ColorWarning
	default:
		return "○", pkgtui.ColorMuted
	}
}

func dispatchStatusDisplay(status string) (string, lipgloss.Color) {
	switch status {
	case "running":
		return "●", pkgtui.ColorPrimary
	case "completed":
		return "✓", pkgtui.ColorSuccess
	case "failed":
		return "✗", pkgtui.ColorError
	case "cancelled":
		return "✗", pkgtui.ColorWarning
	default:
		return "○", pkgtui.ColorMuted
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// normalizePath resolves symlinks and cleans the path for consistent comparison.
// The scanner stores resolved paths, but Intercore stores raw $PWD which may
// differ (symlinks, trailing slashes). Best-effort: returns cleaned path on error.
func normalizePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(p)
}

func (v *BigendView) statusIcon(status autarch.SessionStatus) string {
	switch status {
	case autarch.SessionStatusRunning:
		return pkgtui.StatusRunning.Render("●")
	case autarch.SessionStatusIdle:
		return pkgtui.StatusIdle.Render("○")
	case autarch.SessionStatusError:
		return pkgtui.StatusError.Render("✕")
	default:
		return pkgtui.StatusIdle.Render("?")
	}
}

// Focus implements View
func (v *BigendView) Focus() tea.Cmd {
	v.shell.SetFocus(pkgtui.FocusChat)
	return tea.Batch(v.chatPanel.Focus(), v.loadProjects(), v.loadSessions(), v.loadRuns(), v.loadDispatches())
}

// Blur implements View
func (v *BigendView) Blur() {
	v.chatPanel.CancelStream()
	v.chatPanel.Blur()
}

// Name implements View
func (v *BigendView) Name() string {
	return "Bigend"
}

// ShortHelp implements View
func (v *BigendView) ShortHelp() string {
	return "↑/↓ navigate  F3 switch pane  enter select  ctrl+r refresh  tab focus"
}

// Commands implements CommandProvider
func (v *BigendView) Commands() []tui.Command {
	return []tui.Command{
		{
			Name:        "New Session",
			Description: "Start a new agent session",
			Action: func() tea.Cmd {
				// Snapshot project name for goroutine safety (tea.Cmd runs on pool)
				project := v.activeProjectName()
				c := v.client
				if project != "" {
					c = v.client.ProjectClient(project)
				}
				return func() tea.Msg {
					name := fmt.Sprintf("session-%s", time.Now().Format("150405"))
					s, err := c.CreateSession(autarch.Session{
						Name:    name,
						Project: project,
					})
					return sessionCreatedMsg{session: s, err: err}
				}
			},
		},
		{
			Name:        "Refresh Sessions",
			Description: "Reload session list",
			Action: func() tea.Cmd {
				return v.loadSessions()
			},
		},
	}
}
