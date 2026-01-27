package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// ArbiterCompleteMsg is sent when the sprint finishes with a spec export.
type ArbiterCompleteMsg struct {
	State *arbiter.SprintState
	Spec  *specs.Spec
}

// ArbiterView is a reusable Bubble Tea component for the Arbiter spec sprint.
// It implements the pkgtui.View interface and uses shared ChatPanel + DocPanel + SplitLayout.
type ArbiterView struct {
	orchestrator *arbiter.Orchestrator
	state        *arbiter.SprintState
	coordinator  *research.Coordinator

	// UI components
	chatPanel   *pkgtui.ChatPanel
	docPanel    *pkgtui.DocPanel
	splitLayout *pkgtui.SplitLayout
	optionIndex int

	// Callbacks
	onComplete func(*arbiter.SprintState) tea.Cmd

	// Dimensions
	width  int
	height int

	// State
	focused      bool
	handoffMode  bool // showing handoff options
	handoffIndex int
	finished     bool
}

// NewArbiterView creates a new ArbiterView.
// If coordinator is non-nil, research findings will be integrated.
func NewArbiterView(projectPath string, coordinator *research.Coordinator) *ArbiterView {
	var orch *arbiter.Orchestrator
	if coordinator != nil {
		orch = arbiter.NewOrchestratorWithResearch(projectPath, nil)
	} else {
		orch = arbiter.NewOrchestrator(projectPath)
	}

	chatPanel := pkgtui.NewChatPanel()
	chatPanel.SetComposerHint("enter: send  a: accept  e: edit  1-3: alternatives")

	docPanel := pkgtui.NewDocPanel()

	splitLayout := pkgtui.NewSplitLayout(0.66)
	splitLayout.SetMinWidth(100)

	return &ArbiterView{
		orchestrator: orch,
		coordinator:  coordinator,
		chatPanel:    chatPanel,
		docPanel:     docPanel,
		splitLayout:  splitLayout,
		width:        120,
		height:       40,
	}
}

// SetOnComplete sets the callback for when the sprint finishes.
func (v *ArbiterView) SetOnComplete(cb func(*arbiter.SprintState) tea.Cmd) {
	v.onComplete = cb
}

// SetSuggestions populates the initial vision from scan results.
func (v *ArbiterView) SetSuggestions(suggestions map[string]string) {
	if vision, ok := suggestions["vision"]; ok && vision != "" {
		v.chatPanel.SetValue(vision)
	}
}

// SetCompleteCallback satisfies the InterviewViewSetter interface for unified app compatibility.
func (v *ArbiterView) SetCompleteCallback(cb func(answers map[string]string) tea.Cmd) {
	v.onComplete = func(state *arbiter.SprintState) tea.Cmd {
		// Convert sprint state to interview answers for backward compatibility
		answers := make(map[string]string)
		if s, ok := state.Sections[arbiter.PhaseVision]; ok {
			answers["vision"] = s.Content
		}
		if s, ok := state.Sections[arbiter.PhaseProblem]; ok {
			answers["problem"] = s.Content
		}
		if s, ok := state.Sections[arbiter.PhaseUsers]; ok {
			answers["users"] = s.Content
		}
		if s, ok := state.Sections[arbiter.PhaseRequirements]; ok {
			answers["requirements"] = s.Content
		}
		return cb(answers)
	}
}

// Init implements pkgtui.View.
func (v *ArbiterView) Init() tea.Cmd {
	return func() tea.Msg {
		state, err := v.orchestrator.Start(context.Background(), "")
		if err != nil {
			return nil
		}
		v.state = state
		v.updateDocPanel()
		return nil
	}
}

// StartWithInput initializes the sprint with user-provided input.
func (v *ArbiterView) StartWithInput(input string) tea.Cmd {
	return func() tea.Msg {
		state, err := v.orchestrator.Start(context.Background(), input)
		if err != nil {
			return nil
		}
		v.state = state
		v.updateDocPanel()
		return nil
	}
}

// Update implements pkgtui.View.
func (v *ArbiterView) Update(msg tea.Msg) (pkgtui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			v.width = msg.Width
		}
		if msg.Height > 0 {
			v.height = msg.Height
		}
		return v, nil

	case tea.KeyMsg:
		if v.state == nil {
			return v, nil
		}

		key := msg.String()

		if v.handoffMode {
			return v.handleHandoffKey(key)
		}

		switch key {
		case "a", "A":
			return v.acceptDraft()
		case "e", "E":
			// Start editing - put current content in composer
			if section := v.currentSection(); section != nil {
				v.chatPanel.SetValue(section.Content)
			}
			return v, nil
		case "enter":
			return v.submitComposerContent()
		case "1":
			v.selectOption(0)
		case "2":
			v.selectOption(1)
		case "3":
			v.selectOption(2)
		case "j", "down":
			if section := v.currentSection(); section != nil && v.optionIndex < len(section.Options)-1 {
				v.optionIndex++
			}
		case "k", "up":
			if v.optionIndex > 0 {
				v.optionIndex--
			}
		case "esc":
			// Cancel / go back
			return v, nil
		case "q", "ctrl+c":
			return v, tea.Quit
		}
		v.updateDocPanel()
	}
	return v, nil
}

func (v *ArbiterView) handleHandoffKey(key string) (pkgtui.View, tea.Cmd) {
	options := v.orchestrator.GetHandoffOptions(v.state)
	switch key {
	case "j", "down":
		if v.handoffIndex < len(options)-1 {
			v.handoffIndex++
		}
	case "k", "up":
		if v.handoffIndex > 0 {
			v.handoffIndex--
		}
	case "enter":
		if v.handoffIndex < len(options) {
			opt := options[v.handoffIndex]
			if opt.ID == "spec" {
				spec, err := v.orchestrator.ExportSpec(v.state)
				if err == nil && v.onComplete != nil {
					v.finished = true
					return v, func() tea.Msg {
						return ArbiterCompleteMsg{State: v.state, Spec: spec}
					}
				}
			}
			if v.onComplete != nil {
				v.finished = true
				return v, v.onComplete(v.state)
			}
		}
	case "esc":
		v.handoffMode = false
	}
	v.updateDocPanel()
	return v, nil
}

func (v *ArbiterView) acceptDraft() (pkgtui.View, tea.Cmd) {
	v.orchestrator.AcceptDraft(v.state)

	// Check if this is the last phase
	phases := arbiter.AllPhases()
	isLast := v.state.Phase == phases[len(phases)-1]

	if isLast {
		v.handoffMode = true
		v.updateDocPanel()
		return v, nil
	}

	// Advance to next phase
	newState, err := v.orchestrator.Advance(context.Background(), v.state)
	if err != nil {
		if arbiter.IsBlockerError(err) {
			v.chatPanel.AddMessage("system", "⚠️ Blocker: "+err.Error())
		}
		return v, nil
	}
	v.state = newState
	v.optionIndex = 0
	v.updateDocPanel()
	return v, nil
}

func (v *ArbiterView) submitComposerContent() (pkgtui.View, tea.Cmd) {
	content := v.chatPanel.Value()
	if strings.TrimSpace(content) == "" {
		return v, nil
	}
	v.orchestrator.ReviseDraft(v.state, content, "user edit")
	v.chatPanel.ClearComposer()
	v.updateDocPanel()
	return v, nil
}

func (v *ArbiterView) selectOption(idx int) {
	section := v.currentSection()
	if section == nil || idx >= len(section.Options) {
		return
	}
	section.Content = section.Options[idx]
	v.updateDocPanel()
}

func (v *ArbiterView) currentSection() *arbiter.SectionDraft {
	if v.state == nil {
		return nil
	}
	if section, ok := v.state.Sections[v.state.Phase]; ok {
		return section
	}
	return nil
}

func (v *ArbiterView) updateDocPanel() {
	if v.docPanel == nil || v.state == nil {
		return
	}
	v.docPanel.ClearSections()

	if v.handoffMode {
		v.docPanel.SetTitle("Sprint Complete")
		v.docPanel.SetSubtitle(fmt.Sprintf("Confidence: %.0f%%", v.state.Confidence.Total()*100))
		options := v.orchestrator.GetHandoffOptions(v.state)
		var content string
		for i, opt := range options {
			marker := "  "
			if i == v.handoffIndex {
				marker = "> "
			}
			rec := ""
			if opt.Recommended {
				rec = " ★"
			}
			content += fmt.Sprintf("%s%s — %s%s\n", marker, opt.Label, opt.Description, rec)
		}
		v.docPanel.AddSection(pkgtui.InfoSection("Next Steps", strings.TrimRight(content, "\n")))
		return
	}

	phase := v.state.Phase
	section := v.currentSection()

	v.docPanel.SetTitle(fmt.Sprintf("Phase: %s", phase.String()))
	v.docPanel.SetSubtitle(fmt.Sprintf("Confidence: %.0f%%", v.state.Confidence.Total()*100))

	// Draft content
	if section != nil {
		status := "⏳"
		switch section.Status {
		case arbiter.DraftProposed:
			status = "📝 Proposed"
		case arbiter.DraftAccepted:
			status = "✅ Accepted"
		case arbiter.DraftNeedsRevision:
			status = "✏️ Needs Revision"
		}
		v.docPanel.AddSection(pkgtui.InfoSection(status, section.Content))

		// Alternatives
		if len(section.Options) > 0 {
			var opts string
			for i, opt := range section.Options {
				prefix := fmt.Sprintf("[%d] ", i+1)
				if i == v.optionIndex {
					prefix = fmt.Sprintf("[%d]>", i+1)
				}
				// Truncate long options
				display := opt
				if len(display) > 80 {
					display = display[:77] + "..."
				}
				opts += prefix + display + "\n"
			}
			v.docPanel.AddSection(pkgtui.InfoSection("Alternatives", strings.TrimRight(opts, "\n")))
		}
	}

	// Conflicts
	if len(v.state.Conflicts) > 0 {
		var conflicts string
		for _, c := range v.state.Conflicts {
			icon := "🟡"
			if c.Severity == arbiter.SeverityBlocker {
				icon = "🔴"
			}
			conflicts += fmt.Sprintf("%s %s\n", icon, c.Message)
		}
		v.docPanel.AddSection(pkgtui.InfoSection("Conflicts", strings.TrimRight(conflicts, "\n")))
	}
}

// View implements pkgtui.View.
func (v *ArbiterView) View() string {
	if v.state == nil {
		return "Initializing sprint..."
	}

	left := v.docPanel.View()
	right := v.chatPanel.View()

	stacked := v.width < 100
	if stacked {
		return lipgloss.JoinVertical(lipgloss.Left, left, "", right)
	}

	leftWidth := v.width * 2 / 3
	rightWidth := v.width - leftWidth - 1

	leftStyle := lipgloss.NewStyle().Width(leftWidth)
	rightStyle := lipgloss.NewStyle().Width(rightWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		leftStyle.Render(left),
		" ",
		rightStyle.Render(right),
	)
}

// Focus implements pkgtui.View.
func (v *ArbiterView) Focus() tea.Cmd {
	v.focused = true
	return nil
}

// Blur implements pkgtui.View.
func (v *ArbiterView) Blur() {
	v.focused = false
}

// Name implements pkgtui.View.
func (v *ArbiterView) Name() string {
	return "Arbiter Sprint"
}

// ShortHelp implements pkgtui.View.
func (v *ArbiterView) ShortHelp() string {
	if v.handoffMode {
		return "j/k navigate  enter select  esc back"
	}
	return "a accept  e edit  1-3 alternatives  enter submit  esc cancel"
}
