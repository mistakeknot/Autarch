package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/internal/tui"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// Type aliases to avoid import cycles
type SpecSummary = tui.SpecSummary
type SpecDecision = tui.SpecDecision

// SpecSummaryView displays a completed spec with key decisions and research attributions.
// Uses the unified shell layout with chat for Q&A during review (no sidebar).
type SpecSummaryView struct {
	spec        *SpecSummary
	coordinator *research.Coordinator
	width       int
	height      int
	selected    int
	expanded    map[int]bool
	chatLines   []string

	// Shell layout for unified 3-pane layout (chat only, no sidebar)
	shell *pkgtui.ShellLayout
	// Model selector shown under chat pane
	agentSelector *pkgtui.AgentSelector

	// Research state
	researchComplete bool
	researchFindings int

	// Callbacks
	onGenerateEpics func(spec *SpecSummary) tea.Cmd
	onEditSpec      func(spec *SpecSummary) tea.Cmd
	onWaitResearch  func() tea.Cmd
}

// NewSpecSummaryView creates a new spec summary view.
func NewSpecSummaryView(spec *SpecSummary, coordinator *research.Coordinator) *SpecSummaryView {
	return &SpecSummaryView{
		spec:        spec,
		coordinator: coordinator,
		expanded:    make(map[int]bool),
		shell:       pkgtui.NewShellLayout(),
	}
}

// SetAgentSelector sets the shared agent selector.
func (v *SpecSummaryView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.agentSelector = selector
}

// AppendChatLine appends a streaming agent line to the chat pane.
func (v *SpecSummaryView) AppendChatLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	v.chatLines = append(v.chatLines, line)
	if len(v.chatLines) > 200 {
		v.chatLines = v.chatLines[len(v.chatLines)-200:]
	}
}

// SetCallbacks sets the action callbacks.
func (v *SpecSummaryView) SetCallbacks(
	onGenerateEpics func(*SpecSummary) tea.Cmd,
	onEditSpec func(*SpecSummary) tea.Cmd,
	onWaitResearch func() tea.Cmd,
) {
	v.onGenerateEpics = onGenerateEpics
	v.onEditSpec = onEditSpec
	v.onWaitResearch = onWaitResearch
}

// Init implements View
func (v *SpecSummaryView) Init() tea.Cmd {
	return v.checkResearchStatus()
}

func (v *SpecSummaryView) checkResearchStatus() tea.Cmd {
	return func() tea.Msg {
		return specResearchCheckMsg{}
	}
}

type specResearchCheckMsg struct{}

// Update implements View
func (v *SpecSummaryView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height - 4
		v.shell.SetSize(v.width, v.height)
		return v, nil

	case specResearchCheckMsg:
		v.updateResearchStatus()
		return v, nil

	case research.RunCompletedMsg:
		v.researchComplete = true
		v.researchFindings = msg.TotalFindings
		return v, nil

	case research.HunterUpdateMsg:
		v.updateResearchStatus()
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
		case key.Matches(msg, commonKeys.Select):
			// Generate epics
			if v.onGenerateEpics != nil {
				return v, v.onGenerateEpics(v.spec)
			}
			return v, nil

		case msg.String() == "e":
			// Edit spec
			if v.onEditSpec != nil {
				return v, v.onEditSpec(v.spec)
			}
			return v, nil

		case key.Matches(msg, commonKeys.Refresh):
			// Wait for research / refresh
			if !v.researchComplete && v.onWaitResearch != nil {
				return v, v.onWaitResearch()
			}
			return v, v.checkResearchStatus()

		case msg.String() == "ctrl+r":
			// View research
			return v, nil // Could open overlay

		case key.Matches(msg, commonKeys.NavDown):
			maxItems := len(v.spec.Requirements) + len(v.spec.Decisions)
			if v.selected < maxItems-1 {
				v.selected++
			}
			return v, nil

		case key.Matches(msg, commonKeys.NavUp):
			if v.selected > 0 {
				v.selected--
			}
			return v, nil

		case key.Matches(msg, commonKeys.Toggle):
			v.expanded[v.selected] = !v.expanded[v.selected]
			return v, nil

		case key.Matches(msg, commonKeys.Back) || msg.String() == "backspace" || msg.String() == "b":
			// Back navigation (note: spec_summary may need a back callback)
			// For now, esc cancels any pending operation
			return v, nil
		}
	}

	return v, nil
}

func (v *SpecSummaryView) updateResearchStatus() {
	if v.coordinator == nil {
		return
	}

	run := v.coordinator.GetActiveRun()
	if run == nil {
		v.researchComplete = true
		return
	}

	v.researchComplete = run.IsComplete()
	v.researchFindings = run.TotalFindings()
}

// View implements View
func (v *SpecSummaryView) View() string {
	// Render using shell layout (without sidebar for review views)
	document := v.renderDocument()
	chat := v.renderChat()

	return v.shell.RenderWithoutSidebar(document, chat)
}

// renderDocument renders the main document pane (spec summary).
func (v *SpecSummaryView) renderDocument() string {
	if v.spec == nil {
		return pkgtui.LabelStyle.Render("No spec to display")
	}

	var sections []string

	// Header
	headerStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true).
		MarginBottom(1)
	sections = append(sections, headerStyle.Render("Spec Summary"))

	// Basic info
	sections = append(sections, v.renderField("Vision", v.spec.Vision))
	sections = append(sections, v.renderField("Users", v.spec.Users))
	sections = append(sections, v.renderField("Problem", v.spec.Problem))
	sections = append(sections, "")

	// Platform and Language with potential research attribution
	sections = append(sections, v.renderDecisionField("Platform", v.spec.Platform))
	sections = append(sections, v.renderDecisionField("Language", v.spec.Language))
	sections = append(sections, "")

	// Requirements
	reqHeader := pkgtui.SubtitleStyle.Render("Requirements")
	sections = append(sections, reqHeader)
	for i, req := range v.spec.Requirements {
		marker := "•"
		reqLine := fmt.Sprintf("  %s %s", marker, req)
		if i == v.selected {
			reqLine = pkgtui.SelectedStyle.Render(reqLine)
		}
		sections = append(sections, reqLine)
	}
	sections = append(sections, "")

	// Research status
	sections = append(sections, v.renderResearchStatus())
	sections = append(sections, "")

	// Actions
	sections = append(sections, v.renderActions())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderChat renders the chat pane.
func (v *SpecSummaryView) renderChat() string {
	var lines []string

	chatTitle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)

	lines = append(lines, chatTitle.Render("Spec Chat"))
	lines = append(lines, "")

	mutedStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted).
		Italic(true)

	lines = append(lines, mutedStyle.Render("Ask questions about the spec..."))
	lines = append(lines, "")

	hintStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted)

	lines = append(lines, hintStyle.Render("Tab to focus chat"))

	if len(v.chatLines) > 0 {
		lines = append(lines, "")
		lines = append(lines, pkgtui.SubtitleStyle.Render("Live output"))
		contentStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorFg).
			PaddingLeft(2)
		contentWidth := v.shell.SplitLayout().RightWidth() - 4
		if contentWidth < 10 {
			contentWidth = 10
		}
		start := 0
		if len(v.chatLines) > 12 {
			start = len(v.chatLines) - 12
		}
		for _, line := range v.chatLines[start:] {
			wrapped := pkgtui.WrapText(line, contentWidth)
			for _, part := range strings.Split(wrapped, "\n") {
				lines = append(lines, contentStyle.Render(part))
			}
		}
	}

	if v.agentSelector != nil {
		lines = append(lines, "")
		lines = append(lines, v.agentSelector.View())
	}

	return strings.Join(lines, "\n")
}

func (v *SpecSummaryView) renderField(label, value string) string {
	labelStyle := pkgtui.LabelStyle.Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFg)

	if value == "" {
		value = "(not specified)"
		valueStyle = valueStyle.Foreground(pkgtui.ColorMuted).Italic(true)
	}

	// Truncate long values
	maxLen := v.width - len(label) - 5
	if maxLen > 0 && len(value) > maxLen {
		value = value[:maxLen-3] + "..."
	}

	return fmt.Sprintf("%s %s", labelStyle.Render(label+":"), valueStyle.Render(value))
}

func (v *SpecSummaryView) renderDecisionField(label, value string) string {
	// Check if this decision came from research
	var insightID string
	for _, d := range v.spec.Decisions {
		if strings.EqualFold(d.Key, label) {
			insightID = d.InsightID
			break
		}
	}

	line := v.renderField(label, value)

	if insightID != "" {
		// Add research attribution
		attrStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorSecondary).
			Italic(true)
		line += attrStyle.Render(fmt.Sprintf(" (from research: %s)", insightID[:8]))
	}

	return line
}

func (v *SpecSummaryView) renderResearchStatus() string {
	var status string
	var style lipgloss.Style

	if v.coordinator == nil {
		status = "Research: unavailable"
		style = lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)
	} else if v.researchComplete {
		status = fmt.Sprintf("Research: ✓ complete (%d findings)", v.researchFindings)
		style = lipgloss.NewStyle().Foreground(pkgtui.ColorSuccess)
	} else {
		running := v.coordinator.RunningHunterCount()
		status = fmt.Sprintf("Research: ↻ running (%d hunters)", running)
		style = lipgloss.NewStyle().Foreground(pkgtui.ColorWarning)
	}

	return style.Render(status)
}

func (v *SpecSummaryView) renderActions() string {
	var actions []string

	enterStyle := pkgtui.HelpKeyStyle
	descStyle := pkgtui.HelpDescStyle

	actions = append(actions, fmt.Sprintf("%s %s",
		enterStyle.Render("Enter"),
		descStyle.Render("generate epics")))

	actions = append(actions, fmt.Sprintf("%s %s",
		enterStyle.Render("e"),
		descStyle.Render("edit spec")))

	if !v.researchComplete {
		actions = append(actions, fmt.Sprintf("%s %s",
			enterStyle.Render("r"),
			descStyle.Render("wait for research")))
	} else {
		actions = append(actions, fmt.Sprintf("%s %s",
			enterStyle.Render("r"),
			descStyle.Render("refresh")))
	}

	actions = append(actions, fmt.Sprintf("%s %s",
		enterStyle.Render("Ctrl+R"),
		descStyle.Render("view research")))

	return strings.Join(actions, "  ")
}

// Focus implements View
func (v *SpecSummaryView) Focus() tea.Cmd {
	return v.checkResearchStatus()
}

// Blur implements View
func (v *SpecSummaryView) Blur() {}

// Name implements View
func (v *SpecSummaryView) Name() string {
	return "Summary"
}

// ShortHelp implements View
func (v *SpecSummaryView) ShortHelp() string {
	return "enter generate  e edit  r refresh  F2 model  Tab focus"
}

// FullHelp implements FullHelpProvider
func (v *SpecSummaryView) FullHelp() []tui.HelpBinding {
	return []tui.HelpBinding{
		{Key: "j/k", Description: "Navigate down/up"},
		{Key: "enter", Description: "Generate epics from spec"},
		{Key: "e", Description: "Edit spec (go back to interview)"},
		{Key: "r", Description: "Refresh/wait for research"},
		{Key: "ctrl+r", Description: "View research findings"},
		{Key: "space", Description: "Toggle expand selected"},
		{Key: "esc", Description: "Go back"},
		{Key: "b", Description: "Go back"},
	}
}

// CreateSpecSummaryFromAnswers creates a SpecSummary from interview answers.
// This is a convenience wrapper around tui.CreateSpecSummaryFromAnswers.
func CreateSpecSummaryFromAnswers(projectID string, answers map[string]string, decisions []SpecDecision) *SpecSummary {
	return tui.CreateSpecSummaryFromAnswers(projectID, answers, decisions)
}
