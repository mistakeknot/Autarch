package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	// Intercore dispatches (loaded alongside sessions).
	dispatches []intercore.Dispatch

	// Project context
	projectID   string
	projectName string

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
		client:      client,
		focusPane:   FocusTasks,
		shell:       pkgtui.NewShellLayout(),
		chatPanel:   chatPanel,
		chatHandler: chatHandler,
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

// SetProjectContext sets the current project context.
func (v *BigendView) SetProjectContext(projectID, projectName string) {
	v.projectID = projectID
	v.projectName = projectName
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
}

// sessionCreatedMsg is sent after creating a new session
type sessionCreatedMsg struct {
	session autarch.Session
	err     error
}

// dispatchesLoadedMsg carries dispatches from Intercore.
type dispatchesLoadedMsg struct {
	dispatches []intercore.Dispatch
}

// Init implements View
func (v *BigendView) Init() tea.Cmd {
	return tea.Batch(v.loadSessions(), v.loadDispatches())
}

func (v *BigendView) loadSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := v.client.ListSessions("")
		return sessionsLoadedMsg{sessions: sessions, err: err}
	}
}

func (v *BigendView) loadDispatches() tea.Cmd {
	if v.iclient == nil {
		return nil
	}
	ic := v.iclient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Fetch all dispatches — active + recent completed.
		dispatches, err := ic.DispatchList(ctx, false)
		if err != nil {
			return dispatchesLoadedMsg{} // graceful degradation
		}
		return dispatchesLoadedMsg{dispatches: dispatches}
	}
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

	case sessionsLoadedMsg:
		v.loading = false
		if msg.err != nil {
			v.sessions = nil
		} else {
			v.sessions = msg.sessions
		}
		return v, nil

	case dispatchesLoadedMsg:
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
			// No sidebar items for Bigend
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
				return v, tea.Batch(v.loadSessions(), v.loadDispatches())
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

// View implements View
func (v *BigendView) View() string {
	if v.loading {
		return pkgtui.LabelStyle.Render("Loading sessions...")
	}

	// Render dashboard as document pane, chatPanel as chat pane
	document := v.renderDashboard()
	chat := v.chatPanel.View()

	return v.shell.Render(nil, document, chat)
}

func (v *BigendView) renderDashboard() string {
	var sections []string

	// Project context header
	if v.projectName != "" {
		headerStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorPrimary).
			Bold(true).
			MarginBottom(1)
		sections = append(sections, headerStyle.Render("Project: "+v.projectName))
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
	lines = append(lines, titleStyle.Render(fmt.Sprintf("Sessions (%d)", len(v.sessions))))
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

	// Dispatches section (from Intercore)
	if v.iclient != nil {
		lines = append(lines, "")
		lines = append(lines, pkgtui.SubtitleStyle.Render(fmt.Sprintf("Dispatches (%d)", len(v.dispatches))))

		if len(v.dispatches) == 0 {
			lines = append(lines, pkgtui.LabelStyle.Render("  No dispatches"))
		} else {
			for _, d := range v.dispatches {
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
	}

	return strings.Join(lines, "\n")
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
	return tea.Batch(v.chatPanel.Focus(), v.loadSessions(), v.loadDispatches())
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
				client := v.client
				project := v.projectID
				return func() tea.Msg {
					name := fmt.Sprintf("session-%s", time.Now().Format("150405"))
					s, err := client.CreateSession(autarch.Session{
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
