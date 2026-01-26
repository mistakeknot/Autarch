package views

import (
	"fmt"
	"strings"

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
type SpecSummaryView struct {
	spec        *SpecSummary
	coordinator *research.Coordinator
	width       int
	height      int
	selected    int
	expanded    map[int]bool

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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height - 4
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
		switch msg.String() {
		case "enter":
			// Generate epics
			if v.onGenerateEpics != nil {
				return v, v.onGenerateEpics(v.spec)
			}
			return v, nil

		case "e":
			// Edit spec
			if v.onEditSpec != nil {
				return v, v.onEditSpec(v.spec)
			}
			return v, nil

		case "r":
			// Wait for research / refresh
			if !v.researchComplete && v.onWaitResearch != nil {
				return v, v.onWaitResearch()
			}
			return v, v.checkResearchStatus()

		case "ctrl+r":
			// View research
			return v, nil // Could open overlay

		case "j", "down":
			maxItems := len(v.spec.Requirements) + len(v.spec.Decisions)
			if v.selected < maxItems-1 {
				v.selected++
			}
			return v, nil

		case "k", "up":
			if v.selected > 0 {
				v.selected--
			}
			return v, nil

		case " ":
			// Toggle expand (space key returns " " in Bubble Tea)
			v.expanded[v.selected] = !v.expanded[v.selected]
			return v, nil

		case "esc", "b", "backspace":
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
	return "enter generate  e edit  r refresh  space expand"
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
