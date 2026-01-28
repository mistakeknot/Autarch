package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	"github.com/mistakeknot/autarch/internal/tui"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// TaskReviewView displays proposed tasks for review and editing.
// Uses the unified shell layout with chat for Q&A during review (no sidebar).
type TaskReviewView struct {
	tasks        []tasks.TaskProposal
	groupedView  bool // Group by epic
	selected     int
	expanded     map[int]bool
	width        int
	height       int
	scrollOffset int

	// Shell layout for unified 3-pane layout (chat only, no sidebar)
	shell *pkgtui.ShellLayout
	// Model selector shown under chat pane
	agentSelector *pkgtui.AgentSelector

	// Callbacks
	onAccept func(tasks []tasks.TaskProposal) tea.Cmd
	onBack   func() tea.Cmd
}

// NewTaskReviewView creates a new task review view.
func NewTaskReviewView(taskList []tasks.TaskProposal) *TaskReviewView {
	// Resolve cross-epic dependencies
	tasks.ResolveCrossEpicDependencies(taskList)

	return &TaskReviewView{
		tasks:       taskList,
		groupedView: true,
		expanded:    make(map[int]bool),
		shell:       pkgtui.NewShellLayout(),
	}
}

// SetAgentSelector sets the shared agent selector.
func (v *TaskReviewView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.agentSelector = selector
}

// SetAcceptCallback sets the callback for when tasks are accepted.
func (v *TaskReviewView) SetAcceptCallback(cb func([]tasks.TaskProposal) tea.Cmd) {
	v.onAccept = cb
}

// SetBackCallback sets the callback for going back.
func (v *TaskReviewView) SetBackCallback(cb func() tea.Cmd) {
	v.onBack = cb
}

// Init implements View
func (v *TaskReviewView) Init() tea.Cmd {
	return nil
}

// Update implements View
func (v *TaskReviewView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height - 4
		v.shell.SetSize(v.width, v.height)
		return v, nil

	case tea.KeyMsg:
		if v.agentSelector != nil {
			selectorMsg, selectorCmd := v.agentSelector.Update(msg)
			if selectorMsg != nil {
				return v, tea.Batch(selectorCmd, func() tea.Msg { return selectorMsg })
			}
			if v.agentSelector.Open || msg.Type == tea.KeyF2 {
				return v, selectorCmd
			}
		}

		// Let shell handle global keys first (Tab for focus cycling)
		v.shell, cmd = v.shell.Update(msg)
		if cmd != nil {
			return v, cmd
		}
		switch {
		case key.Matches(msg, commonKeys.Select) || key.Matches(msg, commonKeys.Toggle):
			// Toggle expand selected (same as space for consistency)
			v.expanded[v.selected] = !v.expanded[v.selected]
			return v, nil

		case msg.String() == "A":
			// Accept ALL tasks (uppercase A for intentional action)
			if v.onAccept != nil {
				return v, v.onAccept(v.tasks)
			}
			return v, nil

		case msg.String() == "g":
			// Toggle grouped view
			v.groupedView = !v.groupedView
			return v, nil

		case msg.String() == "e":
			// Edit selected
			// TODO: Add inline editing
			return v, nil

		case msg.String() == "d":
			// Delete selected
			if v.selected >= 0 && v.selected < len(v.tasks) {
				v.tasks = append(v.tasks[:v.selected], v.tasks[v.selected+1:]...)
				if v.selected >= len(v.tasks) {
					v.selected = len(v.tasks) - 1
				}
				// Recompute readiness
				tasks.ResolveCrossEpicDependencies(v.tasks)
			}
			return v, nil

		case key.Matches(msg, commonKeys.NavDown):
			if v.selected < len(v.tasks)-1 {
				v.selected++
				v.ensureVisible()
			}
			return v, nil

		case key.Matches(msg, commonKeys.NavUp):
			if v.selected > 0 {
				v.selected--
				v.ensureVisible()
			}
			return v, nil

		case key.Matches(msg, commonKeys.Back) || msg.String() == "backspace" || msg.String() == "b":
			if v.onBack != nil {
				return v, v.onBack()
			}
			return v, nil

		case msg.String() == "tab":
			// Cycle through types
			if v.selected >= 0 && v.selected < len(v.tasks) {
				v.tasks[v.selected].Edited = true
				switch v.tasks[v.selected].Type {
				case tasks.TaskTypeImplementation:
					v.tasks[v.selected].Type = tasks.TaskTypeTest
				case tasks.TaskTypeTest:
					v.tasks[v.selected].Type = tasks.TaskTypeDocumentation
				case tasks.TaskTypeDocumentation:
					v.tasks[v.selected].Type = tasks.TaskTypeReview
				case tasks.TaskTypeReview:
					v.tasks[v.selected].Type = tasks.TaskTypeResearch
				case tasks.TaskTypeResearch:
					v.tasks[v.selected].Type = tasks.TaskTypeSetup
				default:
					v.tasks[v.selected].Type = tasks.TaskTypeImplementation
				}
			}
			return v, nil
		}
	}

	return v, nil
}

func (v *TaskReviewView) ensureVisible() {
	visibleHeight := v.height - 8
	if v.selected < v.scrollOffset {
		v.scrollOffset = v.selected
	} else if v.selected >= v.scrollOffset+visibleHeight {
		v.scrollOffset = v.selected - visibleHeight + 1
	}
}

// View implements View
func (v *TaskReviewView) View() string {
	// Render using shell layout (without sidebar for review views)
	document := v.renderDocument()
	chat := v.renderChat()

	return v.shell.RenderWithoutSidebar(document, chat)
}

// renderDocument renders the main document pane (task list).
func (v *TaskReviewView) renderDocument() string {
	if len(v.tasks) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorMuted).
			Italic(true)
		return emptyStyle.Render("No tasks proposed. Press b to go back.")
	}

	var sections []string

	// Header
	headerStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)
	sections = append(sections, headerStyle.Render("Review Tasks"))

	// Stats line
	readyCount := tasks.CountReady(v.tasks)
	statsStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted)
	stats := fmt.Sprintf("%d tasks  •  %d ready to start", len(v.tasks), readyCount)
	sections = append(sections, statsStyle.Render(stats))

	// View mode indicator
	modeStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorSecondary).
		Italic(true).
		MarginBottom(1)
	if v.groupedView {
		sections = append(sections, modeStyle.Render("Grouped by Epic"))
	} else {
		sections = append(sections, modeStyle.Render("Flat list"))
	}
	sections = append(sections, "")

	// Task list in styled container
	var taskLines []string
	if v.groupedView {
		taskLines = v.renderGroupedTasks()
	} else {
		taskLines = v.renderFlatTasks()
	}

	tasksContent := strings.Join(taskLines, "\n")
	listWidth := v.shell.LeftWidth()
	if listWidth <= 0 {
		listWidth = v.width / 2
	}
	listStyle := lipgloss.NewStyle().
		Background(pkgtui.ColorBgLight).
		Padding(1, 2).
		Width(listWidth - 4)

	sections = append(sections, listStyle.Render(tasksContent))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderChat renders the chat pane.
func (v *TaskReviewView) renderChat() string {
	var lines []string

	chatTitle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)

	lines = append(lines, chatTitle.Render("Task Review Chat"))
	lines = append(lines, "")

	mutedStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted).
		Italic(true)

	lines = append(lines, mutedStyle.Render("Ask questions about the tasks..."))
	lines = append(lines, "")

	hintStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted)

	lines = append(lines, hintStyle.Render("Tab to focus chat"))

	if v.agentSelector != nil {
		lines = append(lines, "")
		lines = append(lines, v.agentSelector.View())
	}

	return strings.Join(lines, "\n")
}

func (v *TaskReviewView) renderGroupedTasks() []string {
	var lines []string

	groups := tasks.GroupByEpic(v.tasks)

	// Get sorted epic IDs
	epicIDs := make([]string, 0, len(groups))
	for epicID := range groups {
		epicIDs = append(epicIDs, epicID)
	}

	globalIdx := 0
	for _, epicID := range epicIDs {
		epicTasks := groups[epicID]

		// Epic header
		epicStyle := pkgtui.SubtitleStyle.MarginTop(1)
		lines = append(lines, epicStyle.Render(epicID))

		for _, t := range epicTasks {
			isSelected := globalIdx == v.selected
			isExpanded := v.expanded[globalIdx]
			taskLine := v.renderTask(t, isSelected, isExpanded, 2)
			lines = append(lines, taskLine)
			globalIdx++
		}
	}

	return lines
}

func (v *TaskReviewView) renderFlatTasks() []string {
	var lines []string

	for i, t := range v.tasks {
		isSelected := i == v.selected
		isExpanded := v.expanded[i]
		taskLine := v.renderTask(t, isSelected, isExpanded, 0)
		lines = append(lines, taskLine)
	}

	return lines
}

func (v *TaskReviewView) renderTask(t tasks.TaskProposal, selected, expanded bool, indent int) string {
	var lines []string

	indentStr := strings.Repeat(" ", indent)

	// Selector
	var selector string
	if selected {
		selector = ">"
	} else {
		selector = " "
	}

	// Ready indicator
	var readyIcon string
	var readyStyle lipgloss.Style
	if t.Ready {
		readyIcon = "●"
		readyStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorSuccess)
	} else {
		readyIcon = "○"
		readyStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)
	}

	// Type badge
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
	case tasks.TaskTypeDocumentation:
		typeAbbrev = "docs"
	case tasks.TaskTypeReview:
		typeAbbrev = "review"
	case tasks.TaskTypeSetup:
		typeAbbrev = "setup"
	case tasks.TaskTypeResearch:
		typeAbbrev = "research"
	default:
		typeAbbrev = string(t.Type)
	}
	typeBadge := typeStyle.Render(typeAbbrev)

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFg)
	if selected {
		titleStyle = titleStyle.Bold(true).Foreground(pkgtui.ColorPrimary)
	}

	// Edited indicator
	editedMark := ""
	if t.Edited {
		editedMark = lipgloss.NewStyle().Foreground(pkgtui.ColorWarning).Render(" *")
	}

	header := fmt.Sprintf("%s%s %s %s  %s%s",
		indentStr,
		selector,
		readyStyle.Render(readyIcon),
		typeBadge,
		titleStyle.Render(t.Title),
		editedMark,
	)
	lines = append(lines, header)

	// Expanded view
	if expanded {
		expandIndent := indentStr + "    "

		// Task ID
		idStyle := pkgtui.LabelStyle
		lines = append(lines, expandIndent+idStyle.Render(t.ID))

		// Description
		if t.Description != "" {
			descStyle := pkgtui.LabelStyle.Width(v.width - len(expandIndent) - 4)
			lines = append(lines, expandIndent+descStyle.Render(t.Description))
		}

		// Dependencies
		if len(t.Dependencies) > 0 {
			depStyle := pkgtui.LabelStyle
			deps := strings.Join(t.Dependencies, ", ")
			lines = append(lines, expandIndent+depStyle.Render("Depends on: "+deps))
		}

		lines = append(lines, "") // Spacer
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (v *TaskReviewView) renderActions() string {
	var actions []string

	keyStyle := pkgtui.HelpKeyStyle
	descStyle := pkgtui.HelpDescStyle

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("A"),
		descStyle.Render("accept all")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("Enter"),
		descStyle.Render("expand")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("e"),
		descStyle.Render("edit")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("d"),
		descStyle.Render("delete")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("tab"),
		descStyle.Render("change type")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("g"),
		descStyle.Render("toggle grouping")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("space"),
		descStyle.Render("expand")))

	return strings.Join(actions, "  ")
}

// Focus implements View
func (v *TaskReviewView) Focus() tea.Cmd {
	return nil
}

// Blur implements View
func (v *TaskReviewView) Blur() {}

// Name implements View
func (v *TaskReviewView) Name() string {
	return "Tasks"
}

// ShortHelp implements View
func (v *TaskReviewView) ShortHelp() string {
	return "A accept  b back  d delete  g group  F2 model  Tab focus"
}

// FullHelp implements FullHelpProvider
func (v *TaskReviewView) FullHelp() []tui.HelpBinding {
	return []tui.HelpBinding{
		{Key: "j/k", Description: "Navigate down/up"},
		{Key: "enter", Description: "Toggle expand selected"},
		{Key: "space", Description: "Toggle expand selected"},
		{Key: "A", Description: "Accept ALL tasks"},
		{Key: "e", Description: "Edit selected task"},
		{Key: "d", Description: "Delete selected task"},
		{Key: "tab", Description: "Cycle task type"},
		{Key: "g", Description: "Toggle grouped view"},
		{Key: "b", Description: "Go back"},
		{Key: "esc", Description: "Go back"},
		{Key: "backspace", Description: "Go back"},
	}
}

// GetTasks returns the current tasks (potentially edited).
func (v *TaskReviewView) GetTasks() []tasks.TaskProposal {
	return v.tasks
}
