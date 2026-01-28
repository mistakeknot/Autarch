package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/internal/tui"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// AgentType represents available agent types.
type AgentType string

const (
	AgentClaude AgentType = "claude"
	AgentCodex  AgentType = "codex"
	AgentAider  AgentType = "aider"
	AgentManual AgentType = "manual"
)

// TaskDetailView shows full task context and start options.
// Uses the unified shell layout with chat for Q&A during review (no sidebar).
type TaskDetailView struct {
	task        tasks.TaskProposal
	coordinator *research.Coordinator
	width       int
	height      int

	// Shell layout for unified 3-pane layout (chat only, no sidebar)
	shell *pkgtui.ShellLayout
	// Model selector shown under chat pane
	agentSelector *pkgtui.AgentSelector

	// Agent selection
	agents        []AgentType
	selectedAgent int
	useWorktree   bool

	// Related research
	findings []research.Finding

	// Callbacks
	onStart func(task tasks.TaskProposal, agent AgentType, worktree bool) tea.Cmd
	onBack  func() tea.Cmd

	keys pkgtui.CommonKeys
}

// NewTaskDetailView creates a new task detail view.
func NewTaskDetailView(task tasks.TaskProposal, coordinator *research.Coordinator) *TaskDetailView {
	return &TaskDetailView{
		task:          task,
		coordinator:   coordinator,
		shell:         pkgtui.NewShellLayout(),
		agents:        []AgentType{AgentClaude, AgentCodex, AgentAider, AgentManual},
		selectedAgent: 0,
		useWorktree:   false,
		keys:          pkgtui.NewCommonKeys(),
	}
}

// SetAgentSelector sets the shared agent selector.
func (v *TaskDetailView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.agentSelector = selector
}

// SetCallbacks sets the action callbacks.
func (v *TaskDetailView) SetCallbacks(
	onStart func(tasks.TaskProposal, AgentType, bool) tea.Cmd,
	onBack func() tea.Cmd,
) {
	v.onStart = onStart
	v.onBack = onBack
}

// Init implements View
func (v *TaskDetailView) Init() tea.Cmd {
	return v.loadResearch()
}

func (v *TaskDetailView) loadResearch() tea.Cmd {
	return func() tea.Msg {
		return taskResearchLoadedMsg{}
	}
}

type taskResearchLoadedMsg struct{}

// Update implements View
func (v *TaskDetailView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height - 4
		v.shell.SetSize(v.width, v.height)
		return v, nil

	case taskResearchLoadedMsg:
		v.loadResearchFindings()
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
		case key.Matches(msg, v.keys.Back) || msg.String() == "backspace":
			if v.onBack != nil {
				return v, v.onBack()
			}
			return v, nil

		case key.Matches(msg, v.keys.Select):
			// Start with selected agent
			if v.onStart != nil {
				agent := v.agents[v.selectedAgent]
				return v, v.onStart(v.task, agent, v.useWorktree)
			}
			return v, nil

		case msg.String() == "left":
			if v.selectedAgent > 0 {
				v.selectedAgent--
			}
			return v, nil

		case msg.String() == "right" || msg.String() == "l":
			if v.selectedAgent < len(v.agents)-1 {
				v.selectedAgent++
			}
			return v, nil

		case msg.String() == "w":
			v.useWorktree = !v.useWorktree
			return v, nil
		}
	}

	return v, nil
}

func (v *TaskDetailView) loadResearchFindings() {
	if v.coordinator == nil {
		return
	}

	run := v.coordinator.GetActiveRun()
	if run == nil {
		return
	}

	// Get all findings (or filter by task topic if available)
	for _, update := range run.GetAllUpdates() {
		v.findings = append(v.findings, update.Findings...)
	}
}

// View implements View
func (v *TaskDetailView) View() string {
	// Render using shell layout (without sidebar for review views)
	document := v.renderDocument()
	chat := v.renderChat()

	return v.shell.RenderWithoutSidebar(document, chat)
}

// renderDocument renders the main document pane (task details).
func (v *TaskDetailView) renderDocument() string {
	var sections []string

	// Header
	headerStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true).
		MarginBottom(1)
	sections = append(sections, headerStyle.Render("Task Details"))

	// Task info panel
	sections = append(sections, v.renderTaskInfo())
	sections = append(sections, "")

	// Research panel (if available)
	if len(v.findings) > 0 {
		sections = append(sections, v.renderResearchPanel())
		sections = append(sections, "")
	}

	// Model selector
	sections = append(sections, v.renderAgentSelector())
	sections = append(sections, "")

	// Worktree toggle
	sections = append(sections, v.renderWorktreeToggle())
	sections = append(sections, "")

	// Actions
	sections = append(sections, v.renderActions())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderChat renders the chat pane.
func (v *TaskDetailView) renderChat() string {
	var lines []string

	chatTitle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)

	lines = append(lines, chatTitle.Render("Task Chat"))
	lines = append(lines, "")

	mutedStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted).
		Italic(true)

	lines = append(lines, mutedStyle.Render("Ask questions about this task..."))
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

func (v *TaskDetailView) renderTaskInfo() string {
	var lines []string

	// Task ID and type
	idStyle := pkgtui.LabelStyle.Bold(true)
	lines = append(lines, idStyle.Render(v.task.ID))

	// Type badge
	typeStyle := lipgloss.NewStyle().
		Background(pkgtui.ColorBgLight).
		Foreground(pkgtui.ColorFgDim).
		Padding(0, 1)
	lines = append(lines, typeStyle.Render(string(v.task.Type)))
	lines = append(lines, "")

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorFg).
		Bold(true)
	lines = append(lines, titleStyle.Render(v.task.Title))
	lines = append(lines, "")

	// Description
	if v.task.Description != "" {
		descStyle := pkgtui.LabelStyle.Width(v.width - 4)
		lines = append(lines, descStyle.Render(v.task.Description))
		lines = append(lines, "")
	}

	// Epic and Story links
	if v.task.EpicID != "" {
		lines = append(lines, pkgtui.LabelStyle.Render("Epic: "+v.task.EpicID))
	}
	if v.task.StoryID != "" {
		lines = append(lines, pkgtui.LabelStyle.Render("Story: "+v.task.StoryID))
	}

	// Dependencies
	if len(v.task.Dependencies) > 0 {
		deps := strings.Join(v.task.Dependencies, ", ")
		lines = append(lines, pkgtui.LabelStyle.Render("Depends on: "+deps))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (v *TaskDetailView) renderResearchPanel() string {
	var lines []string

	headerStyle := pkgtui.SubtitleStyle
	lines = append(lines, headerStyle.Render(fmt.Sprintf("Related Research (%d findings)", len(v.findings))))

	// Show top 3 findings
	maxFindings := 3
	for i, f := range v.findings {
		if i >= maxFindings {
			remaining := len(v.findings) - maxFindings
			lines = append(lines, pkgtui.LabelStyle.Render(fmt.Sprintf("  ... and %d more [Ctrl+R to view]", remaining)))
			break
		}

		// Source badge
		sourceStyle := lipgloss.NewStyle().
			Background(pkgtui.ColorBgLight).
			Foreground(pkgtui.ColorFgDim).
			Padding(0, 1)
		sourceBadge := sourceStyle.Render(f.SourceType)

		line := fmt.Sprintf("  %s %s", sourceBadge, f.Title)
		lines = append(lines, line)

		// Source URL (copyable)
		if f.Source != "" {
			urlStyle := pkgtui.LabelStyle.Foreground(pkgtui.ColorSecondary).MarginLeft(4)
			lines = append(lines, urlStyle.Render(f.Source))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (v *TaskDetailView) renderAgentSelector() string {
	var lines []string

	headerStyle := pkgtui.SubtitleStyle
	lines = append(lines, headerStyle.Render("Select Agent"))

	var agentButtons []string
	for i, agent := range v.agents {
		isSelected := i == v.selectedAgent

		var style lipgloss.Style
		switch agent {
		case AgentClaude:
			if isSelected {
				style = pkgtui.BadgeClaudeStyle.Bold(true).Underline(true)
			} else {
				style = pkgtui.BadgeClaudeStyle
			}
		case AgentCodex:
			if isSelected {
				style = pkgtui.BadgeCodexStyle.Bold(true).Underline(true)
			} else {
				style = pkgtui.BadgeCodexStyle
			}
		case AgentAider:
			if isSelected {
				style = pkgtui.BadgeAiderStyle.Bold(true).Underline(true)
			} else {
				style = pkgtui.BadgeAiderStyle
			}
		default:
			if isSelected {
				style = pkgtui.BadgeStyle.Bold(true).Underline(true)
			} else {
				style = pkgtui.BadgeStyle.Background(pkgtui.ColorMuted)
			}
		}

		selector := " "
		if isSelected {
			selector = ">"
		}
		button := fmt.Sprintf("%s %s", selector, style.Render(string(agent)))
		agentButtons = append(agentButtons, button)
	}

	lines = append(lines, strings.Join(agentButtons, "  "))
	lines = append(lines, pkgtui.LabelStyle.Render("  ← → to select"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (v *TaskDetailView) renderWorktreeToggle() string {
	var icon, label string
	var style lipgloss.Style

	if v.useWorktree {
		icon = "☑"
		label = "Create worktree (isolated branch)"
		style = lipgloss.NewStyle().Foreground(pkgtui.ColorSuccess)
	} else {
		icon = "☐"
		label = "Work in current directory"
		style = lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)
	}

	line := fmt.Sprintf("  [w] %s %s", style.Render(icon), label)
	return line
}

func (v *TaskDetailView) renderActions() string {
	var actions []string

	keyStyle := pkgtui.HelpKeyStyle
	descStyle := pkgtui.HelpDescStyle

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("Enter"),
		descStyle.Render("start task")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("←→"),
		descStyle.Render("select agent")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("w"),
		descStyle.Render("toggle worktree")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("Esc"),
		descStyle.Render("back")))

	return strings.Join(actions, "  ")
}

// Focus implements View
func (v *TaskDetailView) Focus() tea.Cmd {
	return v.loadResearch()
}

// Blur implements View
func (v *TaskDetailView) Blur() {}

// Name implements View
func (v *TaskDetailView) Name() string {
	return "Task"
}

// ShortHelp implements View
func (v *TaskDetailView) ShortHelp() string {
	return "enter start  ←→ agent  w worktree  F2 model  Tab focus"
}

// FullHelp implements FullHelpProvider
func (v *TaskDetailView) FullHelp() []tui.HelpBinding {
	return []tui.HelpBinding{
		{Key: "enter", Description: "Start task with selected agent"},
		{Key: "←/h", Description: "Select previous agent"},
		{Key: "→/l", Description: "Select next agent"},
		{Key: "w", Description: "Toggle worktree mode"},
		{Key: "esc", Description: "Go back"},
		{Key: "b", Description: "Go back"},
		{Key: "backspace", Description: "Go back"},
	}
}

// GetSelectedAgent returns the currently selected agent.
func (v *TaskDetailView) GetSelectedAgent() AgentType {
	return v.agents[v.selectedAgent]
}

// UseWorktree returns whether to create a worktree.
func (v *TaskDetailView) UseWorktree() bool {
	return v.useWorktree
}
