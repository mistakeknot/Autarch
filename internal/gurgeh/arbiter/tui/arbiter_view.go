package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	pollardquick "github.com/mistakeknot/autarch/internal/pollard/quick"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	apptui "github.com/mistakeknot/autarch/internal/tui"
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
	shell       *pkgtui.ShellLayout
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
		if url := os.Getenv("INTERMUTE_URL"); url != "" {
			if bridge, err := arbiter.NewResearchBridge(url, projectPath); err == nil {
				orch = arbiter.NewOrchestratorWithResearch(projectPath, bridge)
			}
		}
	}
	if orch == nil {
		orch = arbiter.NewOrchestrator(projectPath)
	}
	orch.SetScanner(pollardquick.NewScanner())

	chatPanel := pkgtui.NewChatPanel()
	chatPanel.SetComposerTitle("Chat")
	chatPanel.SetComposerHint("enter send · ctrl+a accept · ctrl+e edit · /1 /2 /3 alternatives")
	chatPanel.SetComposerPlaceholder("Type to revise the draft...")

	docPanel := pkgtui.NewDocPanel()

	shell := pkgtui.NewShellLayout()

	return &ArbiterView{
		orchestrator: orch,
		coordinator:  coordinator,
		chatPanel:    chatPanel,
		docPanel:     docPanel,
		shell:        shell,
		width:        120,
		height:       40,
	}
}

// SetAgentSelector sets the shared agent selector.
func (v *ArbiterView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.chatPanel.SetAgentSelector(selector)
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

// SidebarItems provides the shared interview steps for the left nav.
func (v *ArbiterView) SidebarItems() []pkgtui.SidebarItem {
	steps := apptui.InterviewSteps()
	items := make([]pkgtui.SidebarItem, 0, len(steps))
	for _, step := range steps {
		items = append(items, pkgtui.SidebarItem{
			ID:    step.ID,
			Label: step.Label,
			Icon:  "○",
		})
	}
	return items
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
		v.resizePanels()
		return v, nil

	case tea.KeyMsg:
		if v.state == nil {
			return v, nil
		}

		key := msg.String()

		if v.handoffMode {
			return v.handleHandoffKey(key)
		}

		// Use ctrl+ combinations to avoid conflict with typing in the chat composer
		switch key {
		case "ctrl+a":
			return v.acceptDraft()
		case "ctrl+e":
			// Start editing - put current content in composer
			if section := v.currentSection(); section != nil {
				v.chatPanel.SetValue(section.Content)
			}
			return v, nil
		case "enter":
			return v.submitComposerContent()
		case "down":
			if section := v.currentSection(); section != nil && v.optionIndex < len(section.Options)-1 {
				v.optionIndex++
			}
		case "up":
			if v.optionIndex > 0 {
				v.optionIndex--
			}
		case "esc":
			// Cancel / go back
			return v, nil
		case "ctrl+c":
			return v, tea.Quit
		}
		v.updateDocPanel()
	}
	return v, nil
}

func (v *ArbiterView) handleHandoffKey(key string) (pkgtui.View, tea.Cmd) {
	options := v.orchestrator.GetHandoffOptions(v.state)
	switch key {
	case "down":
		if v.handoffIndex < len(options)-1 {
			v.handoffIndex++
		}
	case "up":
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
	v.chatPanel.AddMessage("user", fmt.Sprintf("✓ Accepted %s", v.state.Phase.String()))
	v.orchestrator.AcceptDraft(v.state)

	// Check if this is the last phase
	phases := arbiter.AllPhases()
	isLast := v.state.Phase == phases[len(phases)-1]

	if isLast {
		v.handoffMode = true
		v.chatPanel.AddMessage("system", "Sprint complete — choose a handoff option")
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
	v.chatPanel.AddMessage("agent", fmt.Sprintf("Proposing %s draft...", v.state.Phase.String()))
	v.updateDocPanel()
	return v, nil
}

func (v *ArbiterView) submitComposerContent() (pkgtui.View, tea.Cmd) {
	// Check for slash command first
	if slashCmd := v.chatPanel.SubmitInput(); slashCmd != nil {
		return v, slashCmd
	}
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

// resizePanels updates panel dimensions from the split layout.
func (v *ArbiterView) resizePanels() {
	v.shell.SetSize(v.width, v.height)
	split := v.shell.SplitLayout()
	v.docPanel.SetSize(split.LeftWidth(), split.LeftHeight())
	v.chatPanel.SetSize(split.RightWidth(), split.RightHeight())
}

// View implements pkgtui.View.
func (v *ArbiterView) View() string {
	if v.state == nil {
		return "Initializing sprint..."
	}

	v.resizePanels()
	return v.shell.Render(v.SidebarItems(), v.docPanel.View(), v.chatPanel.View())
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
		return "↑/↓ navigate  enter select  esc back"
	}
	return "ctrl+a accept  ctrl+e edit  /1 /2 /3 alternatives  enter submit  esc cancel"
}

// ClearInput clears the chat composer (for ctrl+c soft cancel)
func (v *ArbiterView) ClearInput() {
	v.chatPanel.ClearComposer()
}

// HandleSlashCommand handles view-specific slash commands
func (v *ArbiterView) HandleSlashCommand(command string, args []string) tea.Cmd {
	switch command {
	case "accept", "a":
		_, cmd := v.acceptDraft()
		return cmd
	case "edit", "e":
		// Put current content in composer for editing
		if section := v.currentSection(); section != nil {
			v.chatPanel.SetValue(section.Content)
		}
	case "1":
		v.selectOption(0)
	case "2":
		v.selectOption(1)
	case "3":
		v.selectOption(2)
	}
	return nil
}
