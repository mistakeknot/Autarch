package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/autarch"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// FocusPane indicates which pane is focused in Bigend.
type FocusPane int

const (
	FocusSessions FocusPane = iota
	FocusTasks
)

// BigendView displays sessions and agent overview
type BigendView struct {
	client   *autarch.Client
	sessions []autarch.Session
	selected int
	width    int
	height   int
	loading  bool
	err      error

	// Ready tasks
	readyTasks   []tasks.TaskProposal
	taskSelected int
	focusPane    FocusPane

	// Project context
	projectID   string
	projectName string

	// Callbacks
	onTaskSelect func(task tasks.TaskProposal) tea.Cmd
}

// NewBigendView creates a new Bigend view
func NewBigendView(client *autarch.Client) *BigendView {
	return &BigendView{
		client:    client,
		focusPane: FocusTasks,
	}
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

// Init implements View
func (v *BigendView) Init() tea.Cmd {
	return v.loadSessions()
}

func (v *BigendView) loadSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := v.client.ListSessions("")
		return sessionsLoadedMsg{sessions: sessions, err: err}
	}
}

// Update implements View
func (v *BigendView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width - 6
		v.height = msg.Height - 4 // Account for tabs and footer
		return v, nil

	case sessionsLoadedMsg:
		v.loading = false
		if msg.err != nil {
			v.err = msg.err
		} else {
			v.sessions = msg.sessions
		}
		return v, nil

	case tea.KeyMsg:
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
		case msg.Type == tea.KeyF3, key.Matches(msg, commonKeys.TabCycle):
			// Toggle focus between panes (tab or F3 for compatibility)
			if v.focusPane == FocusSessions {
				v.focusPane = FocusTasks
			} else {
				v.focusPane = FocusSessions
			}
		case key.Matches(msg, commonKeys.Select):
			// Select task to view details
			if v.focusPane == FocusTasks && len(v.readyTasks) > 0 && v.onTaskSelect != nil {
				return v, v.onTaskSelect(v.readyTasks[v.taskSelected])
			}
		case key.Matches(msg, commonKeys.Refresh):
			v.loading = true
			return v, v.loadSessions()
		}
	}

	return v, nil
}

// View implements View
func (v *BigendView) View() string {
	if v.loading {
		return pkgtui.LabelStyle.Render("Loading sessions...")
	}

	if v.err != nil {
		return tui.ErrorView(v.err)
	}

	return v.renderDashboard()
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

	// Main content: two panes
	if v.width >= 80 {
		sections = append(sections, v.renderTwoPaneLayout())
	} else {
		sections = append(sections, v.renderStackedLayout())
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (v *BigendView) renderTwoPaneLayout() string {
	leftWidth := v.width / 2
	rightWidth := v.width - leftWidth - 2

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

func (v *BigendView) renderStackedLayout() string {
	var sections []string
	sections = append(sections, v.renderTasksPane(v.width))
	sections = append(sections, "")
	sections = append(sections, v.renderSessionsPane(v.width))
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
		return strings.Join(lines, "\n")
	}

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

	return strings.Join(lines, "\n")
}

func (v *BigendView) renderEmptyState() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		pkgtui.TitleStyle.Render("Sessions"),
		"",
		pkgtui.LabelStyle.Render("No sessions running"),
		"",
		pkgtui.LabelStyle.Render("Sessions will appear here when agents are started."),
	)
}

func (v *BigendView) renderSplitView() string {
	// Calculate widths
	listWidth := v.width / 3
	detailWidth := v.width - listWidth - 3

	// Render list
	list := v.renderList(listWidth)

	// Render detail
	detail := v.renderDetail(detailWidth)

	// Join horizontally
	listStyle := lipgloss.NewStyle().
		Width(listWidth).
		Height(v.height)

	detailStyle := lipgloss.NewStyle().
		Width(detailWidth).
		Height(v.height).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(pkgtui.ColorMuted).
		PaddingLeft(1)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		listStyle.Render(list),
		detailStyle.Render(detail),
	)
}

func (v *BigendView) renderList(width int) string {
	var lines []string

	lines = append(lines, pkgtui.TitleStyle.Render("Sessions"))
	lines = append(lines, "")

	for i, s := range v.sessions {
		icon := v.statusIcon(s.Status)
		name := s.Name
		if name == "" {
			name = s.ID[:8]
		}

		line := fmt.Sprintf("%s %s", icon, name)
		if i == v.selected {
			line = pkgtui.SelectedStyle.Render(line)
		} else {
			line = pkgtui.UnselectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (v *BigendView) renderDetail(width int) string {
	var lines []string

	lines = append(lines, pkgtui.TitleStyle.Render("Details"))
	lines = append(lines, "")

	if len(v.sessions) == 0 || v.selected >= len(v.sessions) {
		lines = append(lines, pkgtui.LabelStyle.Render("No session selected"))
		return strings.Join(lines, "\n")
	}

	s := v.sessions[v.selected]

	lines = append(lines, fmt.Sprintf("Name: %s", s.Name))
	lines = append(lines, fmt.Sprintf("Agent: %s", s.Agent))
	lines = append(lines, fmt.Sprintf("Status: %s", s.Status))
	lines = append(lines, fmt.Sprintf("Project: %s", s.Project))

	if s.TaskID != "" {
		lines = append(lines, fmt.Sprintf("Task: %s", s.TaskID))
	}

	lines = append(lines, fmt.Sprintf("Started: %s", s.StartedAt.Format("2006-01-02 15:04")))

	return strings.Join(lines, "\n")
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
	return v.loadSessions()
}

// Blur implements View
func (v *BigendView) Blur() {}

// Name implements View
func (v *BigendView) Name() string {
	return "Bigend"
}

// ShortHelp implements View
func (v *BigendView) ShortHelp() string {
	return "↑/↓ navigate  tab switch pane  enter select  ctrl+r refresh"
}

// Commands implements CommandProvider
func (v *BigendView) Commands() []tui.Command {
	return []tui.Command{
		{
			Name:        "New Session",
			Description: "Start a new agent session",
			Action: func() tea.Cmd {
				// TODO: implement
				return nil
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
