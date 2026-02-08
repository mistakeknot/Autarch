package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/coldwine/prd"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/gurgeh/brief"
	gproject "github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	pollardquick "github.com/mistakeknot/autarch/internal/pollard/quick"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	apptui "github.com/mistakeknot/autarch/internal/tui"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
	"gopkg.in/yaml.v3"
)

// ArbiterCompleteMsg is sent when the sprint finishes with a spec export.
type ArbiterCompleteMsg struct {
	State *arbiter.SprintState
	Spec  *specs.Spec
}

type arbiterStartedMsg struct {
	err error
}

type handoffActionCompletedMsg struct {
	actionID string
	summary  string
	err      error
}

// ArbiterView is a reusable Bubble Tea component for the Arbiter spec sprint.
// It implements the pkgtui.View interface and uses shared ChatPanel + DocPanel + SplitLayout.
type ArbiterView struct {
	projectPath  string
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
	chatPanel.SetViewCommands(pkgtui.SprintCommands())

	docPanel := pkgtui.NewDocPanel()

	shell := pkgtui.NewShellLayout()

	return &ArbiterView{
		projectPath:  projectPath,
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
		_, err := v.orchestrator.Start(context.TODO(), "")
		return arbiterStartedMsg{err: err}
	}
}

// StartWithInput initializes the sprint with user-provided input.
func (v *ArbiterView) StartWithInput(input string) tea.Cmd {
	return func() tea.Msg {
		_, err := v.orchestrator.Start(context.TODO(), input)
		return arbiterStartedMsg{err: err}
	}
}

// Update implements pkgtui.View.
func (v *ArbiterView) Update(msg tea.Msg) (pkgtui.View, tea.Cmd) {
	v.syncStateSnapshot()

	switch msg := msg.(type) {
	case arbiterStartedMsg:
		if msg.err != nil {
			v.chatPanel.AddMessage("system", "Failed to start sprint: "+msg.err.Error())
			return v, nil
		}
		v.syncStateSnapshot()
		v.updateDocPanel()
		return v, nil

	case handoffActionCompletedMsg:
		if msg.err != nil {
			v.chatPanel.AddMessage("system", fmt.Sprintf("Handoff failed (%s): %v", msg.actionID, msg.err))
			v.updateDocPanel()
			return v, nil
		}
		if strings.TrimSpace(msg.summary) != "" {
			v.chatPanel.AddMessage("system", msg.summary)
		}
		v.updateDocPanel()
		if v.onComplete != nil {
			v.finished = true
			return v, v.onComplete(v.state)
		}
		return v, nil

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
		key := msg.String()
		if key == "ctrl+c" {
			return v, tea.Quit
		}
		if v.state == nil {
			return v, nil
		}

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
			if opt.ID == "tasks" || opt.ID == "research" {
				v.chatPanel.AddMessage("system", fmt.Sprintf("Running handoff: %s...", opt.Label))
				return v, v.runHandoffAction(opt.ID)
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

func (v *ArbiterView) runHandoffAction(actionID string) tea.Cmd {
	return func() tea.Msg {
		switch actionID {
		case "tasks":
			summary, err := v.runTasksHandoff()
			return handoffActionCompletedMsg{actionID: actionID, summary: summary, err: err}
		case "research":
			summary, err := v.runResearchHandoff()
			return handoffActionCompletedMsg{actionID: actionID, summary: summary, err: err}
		default:
			return handoffActionCompletedMsg{actionID: actionID, summary: "No handoff action executed."}
		}
	}
}

func (v *ArbiterView) runTasksHandoff() (string, error) {
	if v.state == nil {
		return "", fmt.Errorf("no sprint state available")
	}

	spec, err := v.orchestrator.ExportSpec(v.state)
	if err != nil {
		return "", fmt.Errorf("export spec: %w", err)
	}
	if err := persistExportedSpec(v.projectPath, spec); err != nil {
		return "", fmt.Errorf("persist spec: %w", err)
	}

	briefs, err := brief.Decompose(context.TODO(), spec, v.projectPath)
	if err != nil {
		return "", fmt.Errorf("decompose briefs: %w", err)
	}
	if err := brief.SaveBriefsAt(v.projectPath, spec.ID, briefs); err != nil {
		return "", fmt.Errorf("save briefs: %w", err)
	}

	imported, err := prd.ImportFromBriefs(prd.BriefImportOptions{
		Root:   v.projectPath,
		SpecID: spec.ID,
	})
	if err != nil {
		return "", fmt.Errorf("import briefs: %w", err)
	}
	persisted, err := prd.PersistBriefTasks(v.projectPath, imported.Tasks)
	if err != nil {
		return "", fmt.Errorf("persist imported tasks: %w", err)
	}

	return fmt.Sprintf(
		"Generated %d briefs and imported %d tasks (%s).",
		len(briefs),
		persisted.TaskCount,
		persisted.StateDBPath,
	), nil
}

func (v *ArbiterView) runResearchHandoff() (string, error) {
	if v.state == nil {
		return "", fmt.Errorf("no sprint state available")
	}

	if err := v.orchestrator.StartDeepScan(context.TODO(), v.state); err == nil {
		return "Deep research scan started via Intermute provider.", nil
	}

	if v.coordinator == nil {
		return "", fmt.Errorf("deep research unavailable and no research coordinator configured")
	}

	query := deriveResearchQuery(v.state)
	run, err := v.coordinator.StartRun(context.TODO(), v.state.ID,
		[]string{"github-scout", "hackernews-trendwatcher", "competitor-tracker"},
		[]research.TopicConfig{{Key: "handoff", Queries: []string{query}}},
	)
	if err != nil {
		return "", fmt.Errorf("start coordinator run: %w", err)
	}
	return fmt.Sprintf("Research run started (run %s).", run.RunID), nil
}

func deriveResearchQuery(state *arbiter.SprintState) string {
	if state == nil {
		return "product roadmap and competitor analysis"
	}
	if section, ok := state.Sections[arbiter.PhaseVision]; ok && strings.TrimSpace(section.Content) != "" {
		return strings.TrimSpace(section.Content)
	}
	if section, ok := state.Sections[arbiter.PhaseProblem]; ok && strings.TrimSpace(section.Content) != "" {
		return strings.TrimSpace(section.Content)
	}
	return "product roadmap and competitor analysis"
}

func persistExportedSpec(projectPath string, spec *specs.Spec) error {
	if spec == nil {
		return fmt.Errorf("spec is nil")
	}
	if projectPath == "" {
		return fmt.Errorf("project path is required")
	}
	if err := gproject.Init(projectPath); err != nil {
		return err
	}
	specsDir := gproject.SpecsDir(projectPath)
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(spec)
	if err != nil {
		return err
	}
	path := filepath.Join(specsDir, spec.ID+".yaml")
	return os.WriteFile(path, data, 0o644)
}

func (v *ArbiterView) acceptDraft() (pkgtui.View, tea.Cmd) {
	v.chatPanel.AddMessage("user", fmt.Sprintf("✓ Accepted %s", v.state.Phase.String()))
	newState, err := v.orchestrator.AcceptAndAdvance(context.TODO())
	if newState != nil {
		v.state = newState
	}

	if errors.Is(err, arbiter.ErrFinalPhaseAccepted) {
		v.handoffMode = true
		v.chatPanel.AddMessage("system", "Sprint complete — choose a handoff option")
		v.updateDocPanel()
		return v, nil
	}

	if err != nil {
		if arbiter.IsBlockerError(err) {
			v.chatPanel.AddMessage("system", "⚠️ Blocker: "+err.Error())
		}
		v.updateDocPanel()
		return v, nil
	}
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
	v.persistStateSnapshot()
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
	v.persistStateSnapshot()
	v.updateDocPanel()
}

func (v *ArbiterView) syncStateSnapshot() {
	state, ok := v.orchestrator.State()
	if !ok {
		return
	}
	snapshot := state
	v.state = &snapshot
}

func (v *ArbiterView) persistStateSnapshot() {
	if v.state == nil {
		return
	}
	v.orchestrator.SetState(v.state)
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
	return "/vision /problem etc. navigate  ctrl+a accept  ctrl+e edit  /1 /2 /3 alternatives"
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
	// Phase navigation
	case "vision", "vis":
		return v.jumpToPhase(arbiter.PhaseVision)
	case "problem", "prob":
		return v.jumpToPhase(arbiter.PhaseProblem)
	case "users", "usr":
		return v.jumpToPhase(arbiter.PhaseUsers)
	case "features", "feat":
		return v.jumpToPhase(arbiter.PhaseFeaturesGoals)
	case "cujs", "cuj":
		return v.jumpToPhase(arbiter.PhaseCUJs)
	case "reqs", "req":
		return v.jumpToPhase(arbiter.PhaseRequirements)
	case "scope", "scp":
		return v.jumpToPhase(arbiter.PhaseScopeAssumptions)
	case "acceptance", "ac":
		return v.jumpToPhase(arbiter.PhaseAcceptanceCriteria)
	}
	return nil
}

// jumpToPhase switches the sprint view to the specified phase.
func (v *ArbiterView) jumpToPhase(phase arbiter.Phase) tea.Cmd {
	if v.state == nil || v.handoffMode {
		return nil
	}
	v.state.Phase = phase
	v.persistStateSnapshot()
	v.optionIndex = 0
	v.updateDocPanel()
	v.chatPanel.AddMessage("system", fmt.Sprintf("Jumped to %s", phase.String()))
	return nil
}
