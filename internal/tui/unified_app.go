package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/autarch/agent"
	"github.com/mistakeknot/autarch/internal/coldwine/epics"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/pkg/autarch"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// AppMode represents the current mode of the app
type AppMode int

const (
	ModeOnboarding AppMode = iota
	ModeDashboard
)

// UnifiedApp is the main application that handles both onboarding and dashboard modes
type UnifiedApp struct {
	client *autarch.Client
	mode   AppMode

	// Onboarding state
	onboardingState  OnboardingState
	breadcrumb       *Breadcrumb
	currentView      View
	projectID        string
	projectName      string
	projectDesc      string
	interviewAnswers map[string]string
	generatedEpics   []epics.EpicProposal
	generatedTasks   []tasks.TaskProposal
	researchCoord    *research.Coordinator
	ctx              context.Context
	cancel           context.CancelFunc

	// Agent for AI generation
	codingAgent   *agent.Agent
	agentSelector *pkgtui.AgentSelector
	selectedAgent string

	// Loading state
	generating     bool
	generatingWhat string

	// Last agent run snapshot
	lastRunLabel    string
	lastRunSnapshot string

	// Dashboard state
	tabs      *TabBar
	dashViews []View
	palette   *Palette

	// UI state
	width    int
	height   int
	err      error
	showHelp bool // Help overlay visible
	keys     pkgtui.CommonKeys
	// Chat settings
	chatSettings     pkgtui.ChatSettings
	chatSettingsOpen bool
	chatSettingsView *pkgtui.ChatSettingsPanel

	// View factories (injected from main.go)
	createKickoffView     func() View
	createArbiterView     func(*research.Coordinator) View
	createSpecSummaryView func(*SpecSummary, *research.Coordinator) View
	createEpicReviewView  func([]epics.EpicProposal) View
	createTaskReviewView  func([]tasks.TaskProposal) View
	createTaskDetailView  func(tasks.TaskProposal, *research.Coordinator) View
	createDashboardViews  func(*autarch.Client) []View
	createSprintView      func(string) View // projectPath → SprintView
}

// NewUnifiedApp creates a new unified application
func NewUnifiedApp(client *autarch.Client) *UnifiedApp {
	ctx, cancel := context.WithCancel(context.Background())

	breadcrumb := NewBreadcrumb()
	breadcrumb.SetCurrent(OnboardingKickoff)

	tabNames := []string{"Bigend", "Signals", "Gurgeh", "Coldwine", "Pollard"}
	app := &UnifiedApp{
		client:          client,
		mode:            ModeOnboarding,
		onboardingState: OnboardingKickoff,
		breadcrumb:      breadcrumb,
		tabs:            NewTabBar(tabNames),
		palette:         NewPalette(),
		researchCoord:   research.NewCoordinator(nil),
		ctx:             ctx,
		cancel:          cancel,
		keys:            pkgtui.NewCommonKeys(),
		chatSettings:    pkgtui.DefaultChatSettings(),
	}

	return app
}

// SetArbiterViewFactory sets the factory for the Arbiter sprint view (replaces interview).
func (a *UnifiedApp) SetArbiterViewFactory(factory func(*research.Coordinator) View) {
	a.createArbiterView = factory
}

// SetSprintViewFactory sets the factory for the unified chat-driven sprint view.
func (a *UnifiedApp) SetSprintViewFactory(factory func(string) View) {
	a.createSprintView = factory
}

// SetViewFactories sets the factory functions for creating views
func (a *UnifiedApp) SetViewFactories(
	kickoff func() View,
	specSummary func(*SpecSummary, *research.Coordinator) View,
	epicReview func([]epics.EpicProposal) View,
	taskReview func([]tasks.TaskProposal) View,
	taskDetail func(tasks.TaskProposal, *research.Coordinator) View,
	dashViews func(*autarch.Client) []View,
) {
	a.createKickoffView = kickoff
	a.createSpecSummaryView = specSummary
	a.createEpicReviewView = epicReview
	a.createTaskReviewView = taskReview
	a.createTaskDetailView = taskDetail
	a.createDashboardViews = dashViews
}

type agentSelectorSetter interface {
	SetAgentSelector(*pkgtui.AgentSelector)
}

type agentNameSetter interface {
	SetAgentName(string)
}

type chatSettingsSetter interface {
	SetChatSettings(pkgtui.ChatSettings)
}

func (a *UnifiedApp) initAgentSelector() {
	if a.agentSelector != nil {
		return
	}

	projectRoot := ""
	if cwd, err := os.Getwd(); err == nil {
		projectRoot = cwd
	}

	options, err := LoadAgentOptions(projectRoot)
	if err != nil {
		return
	}

	options = filterSupportedAgentOptions(options)
	if len(options) == 0 {
		return
	}

	a.agentSelector = pkgtui.NewAgentSelector(options)
	if a.selectedAgent != "" {
		a.setSelectorIndex(a.selectedAgent)
		return
	}
	if a.codingAgent != nil {
		a.selectedAgent = string(a.codingAgent.Type)
		a.setSelectorIndex(a.selectedAgent)
		return
	}

	a.selectedAgent = options[0].Name
	if resolved, err := agent.DetectAgentByName(a.selectedAgent, exec.LookPath); err == nil {
		a.codingAgent = resolved
	}
}

func (a *UnifiedApp) setSelectorIndex(name string) {
	if a.agentSelector == nil {
		return
	}
	for i, opt := range a.agentSelector.Options {
		if strings.EqualFold(opt.Name, name) {
			a.agentSelector.Index = i
			return
		}
	}
}

func (a *UnifiedApp) attachAgentSelector(view View) {
	if view == nil {
		return
	}
	if a.agentSelector != nil {
		if setter, ok := view.(agentSelectorSetter); ok {
			setter.SetAgentSelector(a.agentSelector)
		}
	}
	a.attachAgentName(view)
	a.attachChatSettings(view)
}

func (a *UnifiedApp) attachAgentName(view View) {
	if view == nil || a.selectedAgent == "" {
		return
	}
	if setter, ok := view.(agentNameSetter); ok {
		setter.SetAgentName(a.selectedAgent)
	}
}

func (a *UnifiedApp) attachChatSettings(view View) {
	if view == nil {
		return
	}
	if setter, ok := view.(chatSettingsSetter); ok {
		setter.SetChatSettings(a.chatSettings)
	}
}

func filterSupportedAgentOptions(options []pkgtui.AgentOption) []pkgtui.AgentOption {
	if len(options) == 0 {
		return options
	}
	out := make([]pkgtui.AgentOption, 0, len(options))
	for _, opt := range options {
		switch strings.ToLower(opt.Name) {
		case "codex", "claude":
			out = append(out, opt)
		}
	}
	return out
}

// Init implements tea.Model
func (a *UnifiedApp) Init() tea.Cmd {
	if settings, err := LoadChatSettings(); err == nil {
		a.chatSettings = settings
	}
	a.chatSettingsView = pkgtui.NewChatSettingsPanel(a.chatSettings)

	// Detect coding agent
	detectedAgent, err := agent.DetectAgent()
	if err == nil {
		a.codingAgent = detectedAgent
		a.selectedAgent = string(detectedAgent.Type)
	}
	// Note: We don't error here - we'll handle missing agent when we need it
	a.initAgentSelector()

	// Start with kickoff view
	if a.createKickoffView != nil {
		a.currentView = a.createKickoffView()
		a.attachAgentSelector(a.currentView)
		return tea.Batch(
			a.currentView.Init(),
			a.currentView.Focus(),
		)
	}
	return nil
}

// Update implements tea.Model
func (a *UnifiedApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.tabs.SetWidth(msg.Width)
		a.palette.SetSize(msg.Width, msg.Height)

		// Pass reduced size to current view (account for header + footer)
		if a.currentView != nil {
			headerHeight := 3
			footerHeight := 3
			contentMsg := tea.WindowSizeMsg{
				Width:  msg.Width,
				Height: msg.Height - headerHeight - footerHeight,
			}
			var cmd tea.Cmd
			a.currentView, cmd = a.currentView.Update(contentMsg)
			return a, cmd
		}
		return a, nil

	case tea.KeyMsg:
		if key.Matches(msg, a.keys.Quit) {
			if a.cancel != nil {
				a.cancel()
			}
			return a, tea.Quit
		}
		// Handle help overlay first
		if a.showHelp {
			switch {
			case key.Matches(msg, a.keys.Help), key.Matches(msg, a.keys.Back):
				a.showHelp = false
			}
			return a, nil
		}

		// Handle palette if visible
		if a.palette.Visible() {
			var cmd tea.Cmd
			a.palette, cmd = a.palette.Update(msg)
			return a, cmd
		}
		if a.chatSettingsOpen {
			if msg.String() == "esc" {
				a.chatSettingsOpen = false
				return a, nil
			}
			if a.chatSettingsView != nil {
				if a.chatSettingsView.Update(msg) {
					a.chatSettings = a.chatSettingsView.Settings
					_ = SaveChatSettings(a.chatSettings)
					a.attachChatSettings(a.currentView)
				}
			}
			return a, nil
		}

		// Handle breadcrumb navigation in onboarding mode
		if a.mode == ModeOnboarding && a.breadcrumb.IsNavigating() {
			var cmd tea.Cmd
			a.breadcrumb, cmd = a.breadcrumb.Update(msg)
			return a, cmd
		}

		if key.Matches(msg, a.keys.Help) {
			a.showHelp = true
			return a, nil
		}

		switch msg.String() {

		case "ctrl+p":
			if a.mode == ModeDashboard {
				return a, a.palette.Show()
			}

		case "ctrl+b":
			// Toggle breadcrumb navigation in onboarding mode
			if a.mode == ModeOnboarding {
				if a.breadcrumb.IsNavigating() {
					a.breadcrumb.StopNavigation()
				} else {
					a.breadcrumb.StartNavigation()
				}
				return a, nil
			}
		case "ctrl+,":
			a.openChatSettings()
			return a, nil
		case "ctrl+u":
			return a, a.revertLastRun()
		}

		// In dashboard mode, handle tab switching
		if a.mode == ModeDashboard {
			switch {
			case msg.String() == "ctrl+left" || msg.String() == "ctrl+pgup":
				return a, a.switchDashboardTab((a.tabs.Active() - 1 + len(a.dashViews)) % len(a.dashViews))
			case msg.String() == "ctrl+right" || msg.String() == "ctrl+pgdown":
				return a, a.switchDashboardTab((a.tabs.Active() + 1) % len(a.dashViews))
			}
		}

		// Pass unhandled keys to current view
		if a.currentView != nil {
			var cmd tea.Cmd
			a.currentView, cmd = a.currentView.Update(msg)
			return a, cmd
		}

	case pkgtui.AgentSelectedMsg:
		a.selectedAgent = msg.Name
		a.setSelectorIndex(msg.Name)
		if resolved, err := agent.DetectAgentByName(msg.Name, exec.LookPath); err == nil {
			a.codingAgent = resolved
		}
		a.attachAgentName(a.currentView)
		return a, nil

	// Handle view transition messages
	case ProjectCreatedMsg:
		return a, a.handleProjectCreated(msg)

	case InterviewCompleteMsg:
		return a, a.handleInterviewComplete(msg)

	case ScanSignoffCompleteMsg:
		return a, a.handleInterviewComplete(InterviewCompleteMsg{Answers: msg.Answers})

	case OpenQuestionsRequest:
		return a, a.resolveOpenQuestionsWithAgent(msg)

	case SuggestionsReadyMsg:
		return a, a.handleSuggestionsReady(msg)

	case SpecAcceptedMsg:
		return a, a.handleSpecAccepted(msg)

	case EpicsGeneratedMsg:
		a.generating = false
		return a, a.handleEpicsGenerated(msg)

	case EpicsAcceptedMsg:
		return a, a.handleEpicsAccepted(msg)

	case TasksGeneratedMsg:
		a.generating = false
		return a, a.handleTasksGenerated(msg)

	case TasksAcceptedMsg:
		return a, a.handleTasksAccepted(msg)

	case GeneratingMsg:
		a.generating = true
		a.generatingWhat = msg.What
		return a, nil

	case GenerationErrorMsg:
		a.generating = false
		a.err = msg.Error
		return a, nil

	case AgentNotFoundMsg:
		a.err = &agent.NoAgentError{}
		return a, nil

	case NavigateToTaskDetailMsg:
		return a, a.showTaskDetail(msg.Task)

	case NavigateBackMsg:
		return a, a.navigateBack()

	case NavigateToKickoffMsg:
		return a, a.navigateToKickoff()

	case NavigateToStepMsg:
		return a, a.navigateToStep(msg.State)

	case OnboardingCompleteMsg:
		return a, a.enterDashboard()

	case ScanCodebaseMsg:
		return a, a.scanCodebase(msg.Path)

	case CodebaseScanResultMsg:
		// Pass to kickoff view - it will handle the result
		if a.mode == ModeOnboarding {
			if len(msg.ValidationErrors) == 0 {
				a.onboardingState = OnboardingInterview
				a.breadcrumb.SetCurrent(OnboardingInterview)
			}
		}

	case scanProgressWithContinuation:
		// Forward progress to current view and schedule next read
		if a.currentView != nil {
			var cmd tea.Cmd
			a.currentView, cmd = a.currentView.Update(msg.ScanProgressMsg)
			return a, tea.Batch(cmd, msg.nextCmd)
		}
		return a, msg.nextCmd
	case agentStreamWithContinuation:
		if setter, ok := a.currentView.(ChatStreamSetter); ok {
			setter.AppendChatLine(msg.Line)
		}
		return a, msg.nextCmd

	case AgentStreamMsg:
		if setter, ok := a.currentView.(ChatStreamSetter); ok {
			setter.AppendChatLine(msg.Line)
		}
		return a, nil
	}

	// Pass to current view
	if a.currentView != nil {
		var cmd tea.Cmd
		a.currentView, cmd = a.currentView.Update(msg)
		return a, cmd
	}

	return a, nil
}

func (a *UnifiedApp) handleProjectCreated(msg ProjectCreatedMsg) tea.Cmd {
	a.projectID = msg.ProjectID
	a.projectName = msg.ProjectName
	a.projectDesc = msg.Description
	a.interviewAnswers = make(map[string]string)

	// Transition to interview/arbiter
	a.onboardingState = OnboardingInterview
	a.breadcrumb.SetCurrent(OnboardingInterview)

	// Prefer unified SprintView (chat-driven 8-phase flow)
	if a.createSprintView != nil {
		projectPath := ""
		if cwd, err := os.Getwd(); err == nil {
			projectPath = cwd
		}
		a.currentView = a.createSprintView(projectPath)
		a.attachAgentSelector(a.currentView)

		// Set up back callback
		if cb, ok := a.currentView.(interface{ SetCallbacks(func() tea.Cmd) }); ok {
			cb.SetCallbacks(func() tea.Cmd {
				return func() tea.Msg { return NavigateBackMsg{} }
			})
		}

		// Start the sprint with the project description
		var startCmd tea.Cmd
		if starter, ok := a.currentView.(SprintStarter); ok {
			startCmd = starter.StartSprint(msg.Description)
		}

		cmds := []tea.Cmd{
			a.currentView.Init(),
			a.currentView.Focus(),
			a.sendWindowSize(),
		}
		if startCmd != nil {
			cmds = append(cmds, startCmd)
		}
		return tea.Batch(cmds...)
	}

	// Fall back to Arbiter view if available
	if a.createArbiterView != nil {
		a.currentView = a.createArbiterView(a.researchCoord)
		a.attachAgentSelector(a.currentView)

		// Set up callback for when sprint completes (backward-compatible)
		if iv, ok := a.currentView.(InterviewViewSetter); ok {
			iv.SetCompleteCallback(func(answers map[string]string) tea.Cmd {
				return func() tea.Msg {
					return InterviewCompleteMsg{Answers: answers}
				}
			})

			// If we have scan results, use them as suggestions
			if msg.ScanResult != nil {
				suggestions := make(map[string]string)
				if msg.ScanResult.Vision != "" {
					suggestions["vision"] = msg.ScanResult.Vision
				}
				if msg.ScanResult.Users != "" {
					suggestions["users"] = msg.ScanResult.Users
				}
				if msg.ScanResult.Problem != "" {
					suggestions["problem"] = msg.ScanResult.Problem
				}
				iv.SetSuggestions(suggestions)
			}
		}

		cmds := []tea.Cmd{
			a.currentView.Init(),
			a.currentView.Focus(),
			a.sendWindowSize(),
		}
		return tea.Batch(cmds...)
	}

	// No arbiter view — skip interview and go directly to spec summary
	// using the project description and scan results as answers.
	answers := map[string]string{
		"vision": msg.Description,
	}
	if msg.ScanResult != nil {
		if msg.ScanResult.Vision != "" {
			answers["vision"] = msg.ScanResult.Vision
		}
		if msg.ScanResult.Users != "" {
			answers["users"] = msg.ScanResult.Users
		}
		if msg.ScanResult.Problem != "" {
			answers["problem"] = msg.ScanResult.Problem
		}
		if msg.ScanResult.Platform != "" {
			answers["platform"] = msg.ScanResult.Platform
		}
		if msg.ScanResult.Language != "" {
			answers["language"] = msg.ScanResult.Language
		}
		if len(msg.ScanResult.Requirements) > 0 {
			answers["requirements"] = strings.Join(msg.ScanResult.Requirements, "\n")
		}
	}
	return func() tea.Msg {
		return InterviewCompleteMsg{Answers: answers}
	}
}

func (a *UnifiedApp) generateSuggestions() tea.Cmd {
	if a.codingAgent == nil {
		// No agent available, user will type manually
		return nil
	}

	return func() tea.Msg {
		questions := []string{
			"What is your project vision? Describe what you want to build.",
			"Who are the primary users of this project?",
			"What problem are you solving?",
			"What platform(s) will this run on? (Web, CLI, Desktop, Mobile, API/Backend)",
			"What programming language(s) will you use? (Go, TypeScript, Python, Rust, Other)",
			"List the key requirements (one per line).",
		}

		suggestions, err := agent.SuggestAnswers(context.Background(), a.codingAgent, a.projectDesc, questions)
		return SuggestionsReadyMsg{Suggestions: suggestions, Error: err}
	}
}

func (a *UnifiedApp) scanCodebase(path string) tea.Cmd {
	if a.codingAgent == nil {
		// No agent - show error with instructions
		return func() tea.Msg {
			return CodebaseScanResultMsg{
				Error: &agent.NoAgentError{},
			}
		}
	}

	// Create a channel for progress updates
	progressChan := make(chan agent.ScanProgress, 100)

	// Start the scan in a goroutine
	go func() {
		defer close(progressChan)

		result, err := agent.ScanCodebaseWithProgress(
			context.Background(),
			a.codingAgent,
			path,
			func(p agent.ScanProgress) {
				// Non-blocking send to avoid deadlock
				select {
				case progressChan <- p:
				default:
				}
			},
		)

		// Send final result through the channel as a special progress message
		if err != nil {
			progressChan <- agent.ScanProgress{Step: "_error", Details: err.Error()}
		} else {
			// Encode result in progress for simplicity
			progressChan <- agent.ScanProgress{
				Step:    "_complete",
				Details: result.ProjectName,
				Files: []string{
					result.Description,
					result.Vision,
					result.Users,
					result.Problem,
					result.Platform,
					result.Language,
					strings.Join(result.Requirements, "|||"),
				},
				ValidationErrors: result.ValidationErrors,
				PhaseArtifacts:   result.PhaseArtifacts,
			}
		}
	}()

	// Return a command that reads from the progress channel
	return a.waitForScanProgress(progressChan)
}

// waitForScanProgress reads one progress update from the channel and returns it as a message.
func (a *UnifiedApp) waitForScanProgress(ch <-chan agent.ScanProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			// Channel closed unexpectedly
			return CodebaseScanResultMsg{Error: fmt.Errorf("scan interrupted")}
		}

		// Check for special completion messages
		if p.Step == "_error" {
			return CodebaseScanResultMsg{Error: fmt.Errorf("%s", p.Details)}
		}
		if p.Step == "_complete" {
			// Decode result from Files array
			var requirements []string
			if len(p.Files) >= 7 && p.Files[6] != "" {
				requirements = strings.Split(p.Files[6], "|||")
			}
			return CodebaseScanResultMsg{
				ProjectName:      p.Details,
				Description:      safeIndex(p.Files, 0),
				Vision:           safeIndex(p.Files, 1),
				Users:            safeIndex(p.Files, 2),
				Problem:          safeIndex(p.Files, 3),
				Platform:         safeIndex(p.Files, 4),
				Language:         safeIndex(p.Files, 5),
				Requirements:     requirements,
				ValidationErrors: toValidationErrors(p.ValidationErrors),
				PhaseArtifacts:   toPhaseArtifacts(p.PhaseArtifacts),
			}
		}

		// Return progress message and schedule next read
		return scanProgressWithContinuation{
			ScanProgressMsg: ScanProgressMsg{
				Step:      p.Step,
				Details:   p.Details,
				Files:     p.Files,
				AgentLine: p.AgentLine,
			},
			nextCmd: a.waitForScanProgress(ch),
		}
	}
}

func safeIndex(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

func toValidationErrors(errs []agent.ValidationError) []ValidationError {
	if len(errs) == 0 {
		return nil
	}
	out := make([]ValidationError, 0, len(errs))
	for _, err := range errs {
		out = append(out, ValidationError{
			Code:    err.Code,
			Field:   err.Field,
			Message: err.Message,
		})
	}
	return out
}

func toPhaseArtifacts(artifacts *agent.PhaseArtifacts) *PhaseArtifacts {
	if artifacts == nil {
		return nil
	}
	return &PhaseArtifacts{
		Vision:  toVisionArtifact(artifacts.Vision),
		Problem: toProblemArtifact(artifacts.Problem),
		Users:   toUsersArtifact(artifacts.Users),
	}
}

func toVisionArtifact(artifact *agent.VisionArtifact) *VisionArtifact {
	if artifact == nil {
		return nil
	}
	return &VisionArtifact{
		Phase:         artifact.Phase,
		Version:       artifact.Version,
		Summary:       artifact.Summary,
		Goals:         append([]string{}, artifact.Goals...),
		NonGoals:      append([]string{}, artifact.NonGoals...),
		Evidence:      toEvidenceItems(artifact.Evidence),
		OpenQuestions: append([]string{}, artifact.OpenQuestions...),
		Quality:       toQualityScores(artifact.Quality),
	}
}

func toProblemArtifact(artifact *agent.ProblemArtifact) *ProblemArtifact {
	if artifact == nil {
		return nil
	}
	return &ProblemArtifact{
		Phase:         artifact.Phase,
		Version:       artifact.Version,
		Summary:       artifact.Summary,
		PainPoints:    append([]string{}, artifact.PainPoints...),
		Impact:        artifact.Impact,
		Evidence:      toEvidenceItems(artifact.Evidence),
		OpenQuestions: append([]string{}, artifact.OpenQuestions...),
		Quality:       toQualityScores(artifact.Quality),
	}
}

func toUsersArtifact(artifact *agent.UsersArtifact) *UsersArtifact {
	if artifact == nil {
		return nil
	}
	return &UsersArtifact{
		Phase:         artifact.Phase,
		Version:       artifact.Version,
		Personas:      toPersonas(artifact.Personas),
		Evidence:      toEvidenceItems(artifact.Evidence),
		OpenQuestions: append([]string{}, artifact.OpenQuestions...),
		Quality:       toQualityScores(artifact.Quality),
	}
}

func toEvidenceItems(items []agent.EvidenceItem) []EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]EvidenceItem, 0, len(items))
	for _, item := range items {
		out = append(out, EvidenceItem{
			Type:       item.Type,
			Path:       item.Path,
			Quote:      item.Quote,
			Confidence: item.Confidence,
		})
	}
	return out
}

func toAgentEvidenceItems(items []EvidenceItem) []agent.EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]agent.EvidenceItem, 0, len(items))
	for _, item := range items {
		out = append(out, agent.EvidenceItem{
			Type:       item.Type,
			Path:       item.Path,
			Quote:      item.Quote,
			Confidence: item.Confidence,
		})
	}
	return out
}

func toPersonas(items []agent.Persona) []Persona {
	if len(items) == 0 {
		return nil
	}
	out := make([]Persona, 0, len(items))
	for _, item := range items {
		out = append(out, Persona{
			Name:    item.Name,
			Needs:   append([]string{}, item.Needs...),
			Context: item.Context,
		})
	}
	return out
}

func toResolvedQuestions(items []agent.ResolvedQuestion) []ResolvedQuestion {
	if len(items) == 0 {
		return nil
	}
	out := make([]ResolvedQuestion, 0, len(items))
	for _, item := range items {
		out = append(out, ResolvedQuestion{
			Question: item.Question,
			Answer:   item.Answer,
		})
	}
	return out
}

func toQualityScores(scores agent.QualityScores) QualityScores {
	return QualityScores{
		Clarity:      scores.Clarity,
		Completeness: scores.Completeness,
		Grounding:    scores.Grounding,
		Consistency:  scores.Consistency,
	}
}

// scanProgressWithContinuation wraps a progress message with a continuation command.
type scanProgressWithContinuation struct {
	ScanProgressMsg
	nextCmd tea.Cmd
}

func (a *UnifiedApp) captureRunSnapshot() {
	a.lastRunLabel = ""
	a.lastRunSnapshot = ""
	if snapper, ok := a.currentView.(DocumentSnapshotter); ok {
		label, content := snapper.DocumentSnapshot()
		a.lastRunLabel = label
		a.lastRunSnapshot = content
	}
}

func (a *UnifiedApp) finalizeAgentRun(what string) {
	var diff []string
	var diffErr error

	if a.lastRunSnapshot != "" {
		if snapper, ok := a.currentView.(DocumentSnapshotter); ok {
			label, after := snapper.DocumentSnapshot()
			if label != "" {
				a.lastRunLabel = label
			}
			diff, diffErr = pkgtui.UnifiedDiff(a.lastRunSnapshot, after, a.lastRunLabel)
		}
	}

	a.sendToCurrentView(AgentRunFinishedMsg{
		What: what,
		Err:  diffErr,
		Diff: diff,
	})

	summary := summarizeDiff(diff, diffErr)
	if summary != "" {
		a.sendToCurrentView(AgentEditSummaryMsg{Summary: summary})
	}
}

func summarizeDiff(diff []string, err error) string {
	if err != nil {
		return "Agent run complete. Diff unavailable."
	}
	if len(diff) == 0 {
		return "Agent run complete. No document changes detected."
	}

	adds := 0
	dels := 0
	for _, line := range diff {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			adds++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			dels++
		}
	}

	return fmt.Sprintf("Agent run complete. +%d -%d lines.", adds, dels)
}

func (a *UnifiedApp) sendToCurrentView(msg tea.Msg) {
	if a.currentView == nil {
		return
	}
	var cmd tea.Cmd
	a.currentView, cmd = a.currentView.Update(msg)
	_ = cmd
}

// agentStreamEvent represents a single streaming output or final result.
type agentStreamEvent struct {
	line          string
	epics         []epics.EpicProposal
	tasks         []tasks.TaskProposal
	err           error
	openQuestions *OpenQuestionsResolvedMsg
}

// agentStreamWithContinuation wraps a stream message with a continuation command.
type agentStreamWithContinuation struct {
	AgentStreamMsg
	nextCmd tea.Cmd
}

func (a *UnifiedApp) waitForAgentStream(ch <-chan agentStreamEvent, what string) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return GenerationErrorMsg{What: what, Error: fmt.Errorf("%s generation interrupted", what)}
		}

		if ev.line != "" {
			return agentStreamWithContinuation{
				AgentStreamMsg: AgentStreamMsg{Line: ev.line},
				nextCmd:        a.waitForAgentStream(ch, what),
			}
		}

		if ev.openQuestions != nil {
			return *ev.openQuestions
		}

		if ev.err != nil {
			return GenerationErrorMsg{What: what, Error: ev.err}
		}

		if len(ev.epics) > 0 {
			return EpicsGeneratedMsg{Epics: ev.epics}
		}

		if len(ev.tasks) > 0 {
			return TasksGeneratedMsg{Tasks: ev.tasks}
		}

		return GenerationErrorMsg{What: what, Error: fmt.Errorf("%s generation interrupted", what)}
	}
}

func (a *UnifiedApp) handleSuggestionsReady(msg SuggestionsReadyMsg) tea.Cmd {
	if msg.Error != nil {
		// Suggestions failed, user will type manually - this is not fatal
		return nil
	}

	// Pass suggestions to the interview view
	if iv, ok := a.currentView.(InterviewViewSetter); ok {
		iv.SetSuggestions(msg.Suggestions)
	}
	return nil
}

func (a *UnifiedApp) handleInterviewComplete(msg InterviewCompleteMsg) tea.Cmd {
	a.interviewAnswers = msg.Answers
	a.onboardingState = OnboardingSpecSummary
	a.breadcrumb.SetCurrent(OnboardingSpecSummary)

	// Create spec summary from answers
	spec := CreateSpecSummaryFromAnswers(a.projectID, msg.Answers, nil)

	if a.createSpecSummaryView != nil {
		a.currentView = a.createSpecSummaryView(spec, a.researchCoord)
		a.attachAgentSelector(a.currentView)

		// Set up callbacks
		if sv, ok := a.currentView.(SpecSummaryViewSetter); ok {
			sv.SetCallbacks(
				// onGenerateEpics
				func(s *SpecSummary) tea.Cmd {
					return func() tea.Msg {
						return SpecAcceptedMsg{
							Vision:       s.Vision,
							Users:        s.Users,
							Problem:      s.Problem,
							Platform:     s.Platform,
							Language:     s.Language,
							Requirements: s.Requirements,
						}
					}
				},
				// onEditSpec - go back to interview
				func(s *SpecSummary) tea.Cmd {
					return func() tea.Msg {
						return NavigateBackMsg{}
					}
				},
				// onWaitResearch
				nil,
			)
		}

		return tea.Batch(
			a.currentView.Init(),
			a.currentView.Focus(),
			a.sendWindowSize(),
		)
	}
	return nil
}

func (a *UnifiedApp) handleSpecAccepted(msg SpecAcceptedMsg) tea.Cmd {
	a.onboardingState = OnboardingEpicReview
	a.generating = true
	a.generatingWhat = "epics"

	// Generate epics using the agent
	return a.generateEpicsWithAgent(msg)
}

func (a *UnifiedApp) generateEpicsWithAgent(spec SpecAcceptedMsg) tea.Cmd {
	if a.codingAgent == nil {
		// No agent - show error with instructions
		return func() tea.Msg {
			return AgentNotFoundMsg{
				Instructions: (&agent.NoAgentError{}).Instructions(),
			}
		}
	}

	a.captureRunSnapshot()
	input := agent.SpecInput{
		Vision:       spec.Vision,
		Users:        spec.Users,
		Problem:      spec.Problem,
		Platform:     spec.Platform,
		Language:     spec.Language,
		Requirements: spec.Requirements,
	}

	stream := make(chan agentStreamEvent, 100)
	go func() {
		defer close(stream)
		outputCallback := func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			select {
			case stream <- agentStreamEvent{line: line}:
			default:
			}
		}

		proposals, err := agent.GenerateEpicsWithOutput(context.Background(), a.codingAgent, input, outputCallback)
		if err != nil {
			stream <- agentStreamEvent{err: err}
			return
		}
		stream <- agentStreamEvent{epics: proposals}
	}()

	return tea.Batch(
		func() tea.Msg { return AgentRunStartedMsg{What: "epics"} },
		a.waitForAgentStream(stream, "epics"),
	)
}

func (a *UnifiedApp) handleEpicsGenerated(msg EpicsGeneratedMsg) tea.Cmd {
	a.finalizeAgentRun("epics")
	a.generatedEpics = msg.Epics
	a.breadcrumb.SetCurrent(OnboardingEpicReview)

	// Show epic review view
	if a.createEpicReviewView != nil {
		a.currentView = a.createEpicReviewView(msg.Epics)
		a.attachAgentSelector(a.currentView)
		return tea.Batch(
			a.currentView.Init(),
			a.currentView.Focus(),
			a.sendWindowSize(),
		)
	}
	return nil
}

func (a *UnifiedApp) handleEpicsAccepted(msg EpicsAcceptedMsg) tea.Cmd {
	a.generatedEpics = msg.Epics
	a.onboardingState = OnboardingTaskReview
	a.generating = true
	a.generatingWhat = "tasks"

	// Generate tasks from epics using the agent
	return a.generateTasksWithAgent()
}

func (a *UnifiedApp) generateTasksWithAgent() tea.Cmd {
	if a.codingAgent == nil {
		// No agent - show error with instructions
		return func() tea.Msg {
			return AgentNotFoundMsg{
				Instructions: (&agent.NoAgentError{}).Instructions(),
			}
		}
	}

	a.captureRunSnapshot()
	stream := make(chan agentStreamEvent, 100)
	go func() {
		defer close(stream)
		outputCallback := func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			select {
			case stream <- agentStreamEvent{line: line}:
			default:
			}
		}

		taskList, err := agent.GenerateTasksWithOutput(context.Background(), a.codingAgent, a.generatedEpics, outputCallback)
		if err != nil {
			stream <- agentStreamEvent{err: err}
			return
		}
		stream <- agentStreamEvent{tasks: taskList}
	}()

	return tea.Batch(
		func() tea.Msg { return AgentRunStartedMsg{What: "tasks"} },
		a.waitForAgentStream(stream, "tasks"),
	)
}

func (a *UnifiedApp) resolveOpenQuestionsWithAgent(req OpenQuestionsRequest) tea.Cmd {
	if a.codingAgent == nil {
		return func() tea.Msg {
			return AgentNotFoundMsg{
				Instructions: (&agent.NoAgentError{}).Instructions(),
			}
		}
	}

	input := agent.ResolveOpenQuestionsInput{
		Phase:         req.Phase,
		Summary:       req.Summary,
		Evidence:      toAgentEvidenceItems(req.Evidence),
		OpenQuestions: append([]string{}, req.OpenQuestions...),
		UserAnswer:    req.UserAnswer,
		Vision:        req.Vision,
		Problem:       req.Problem,
		Users:         req.Users,
		Platform:      req.Platform,
		Language:      req.Language,
		Requirements:  append([]string{}, req.Requirements...),
	}

	stream := make(chan agentStreamEvent, 100)
	go func() {
		defer close(stream)
		outputCallback := func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			select {
			case stream <- agentStreamEvent{line: line}:
			default:
			}
		}

		resolution, err := agent.ResolveOpenQuestionsWithOutput(context.Background(), a.codingAgent, input, outputCallback)
		if err != nil {
			stream <- agentStreamEvent{openQuestions: &OpenQuestionsResolvedMsg{Phase: req.Phase, Err: err}}
			return
		}
		stream <- agentStreamEvent{openQuestions: &OpenQuestionsResolvedMsg{
			Phase:     req.Phase,
			Resolved:  toResolvedQuestions(resolution.Resolved),
			Remaining: append([]string{}, resolution.Remaining...),
		}}
	}()

	return a.waitForAgentStream(stream, "open-questions")
}

func (a *UnifiedApp) handleTasksGenerated(msg TasksGeneratedMsg) tea.Cmd {
	a.finalizeAgentRun("tasks")
	a.generatedTasks = msg.Tasks
	a.breadcrumb.SetCurrent(OnboardingTaskReview)

	// Show task review view
	if a.createTaskReviewView != nil {
		a.currentView = a.createTaskReviewView(msg.Tasks)
		a.attachAgentSelector(a.currentView)
		return tea.Batch(
			a.currentView.Init(),
			a.currentView.Focus(),
			a.sendWindowSize(),
		)
	}
	return nil
}

func (a *UnifiedApp) handleTasksAccepted(msg TasksAcceptedMsg) tea.Cmd {
	a.generatedTasks = msg.Tasks
	a.onboardingState = OnboardingComplete
	a.breadcrumb.SetCurrent(OnboardingComplete)

	// Transition to dashboard
	return func() tea.Msg {
		return OnboardingCompleteMsg{
			ProjectID:   a.projectID,
			ProjectName: a.projectName,
		}
	}
}

func (a *UnifiedApp) showTaskDetail(task tasks.TaskProposal) tea.Cmd {
	if a.createTaskDetailView != nil {
		a.currentView = a.createTaskDetailView(task, a.researchCoord)
		a.attachAgentSelector(a.currentView)
		return tea.Batch(
			a.currentView.Init(),
			a.currentView.Focus(),
			a.sendWindowSize(),
		)
	}
	return nil
}

func (a *UnifiedApp) navigateBack() tea.Cmd {
	// Return to appropriate view based on state
	switch a.onboardingState {
	case OnboardingEpicReview:
		return a.navigateToKickoff()
	case OnboardingTaskReview:
		// Go back to epic review
		a.onboardingState = OnboardingEpicReview
		a.breadcrumb.SetCurrent(OnboardingEpicReview)
		if a.createEpicReviewView != nil {
			a.currentView = a.createEpicReviewView(a.generatedEpics)
			a.attachAgentSelector(a.currentView)
			return tea.Batch(a.currentView.Init(), a.currentView.Focus(), a.sendWindowSize())
		}
	case OnboardingComplete:
		// In dashboard mode, go back to Bigend
		if len(a.dashViews) > 0 {
			a.currentView = a.dashViews[0]
			return a.currentView.Focus()
		}
	}
	return nil
}

func (a *UnifiedApp) navigateToKickoff() tea.Cmd {
	a.onboardingState = OnboardingKickoff
	a.breadcrumb.SetCurrent(OnboardingKickoff)
	// Clear any generated data
	a.generatedEpics = nil
	a.generatedTasks = nil
	a.projectID = ""
	a.projectName = ""
	a.projectDesc = ""

	if a.createKickoffView != nil {
		a.currentView = a.createKickoffView()
		a.attachAgentSelector(a.currentView)
		return tea.Batch(
			a.currentView.Init(),
			a.currentView.Focus(),
			a.sendWindowSize(),
		)
	}
	return nil
}

func (a *UnifiedApp) navigateToStep(state OnboardingState) tea.Cmd {
	// Only allow navigation to unlocked steps
	switch state {
	case OnboardingKickoff:
		return a.navigateToKickoff()

	case OnboardingScanVision, OnboardingScanProblem, OnboardingScanUsers:
		a.onboardingState = state
		a.breadcrumb.SetCurrent(state)
		return nil

	case OnboardingEpicReview:
		// Only if we have generated epics
		if len(a.generatedEpics) > 0 {
			a.onboardingState = OnboardingEpicReview
			a.breadcrumb.SetCurrent(OnboardingEpicReview)
			if a.createEpicReviewView != nil {
				a.currentView = a.createEpicReviewView(a.generatedEpics)
				a.attachAgentSelector(a.currentView)
				return tea.Batch(
					a.currentView.Init(),
					a.currentView.Focus(),
					a.sendWindowSize(),
				)
			}
		}

	case OnboardingTaskReview:
		// Only if we have generated tasks
		if len(a.generatedTasks) > 0 {
			a.onboardingState = OnboardingTaskReview
			a.breadcrumb.SetCurrent(OnboardingTaskReview)
			if a.createTaskReviewView != nil {
				a.currentView = a.createTaskReviewView(a.generatedTasks)
				a.attachAgentSelector(a.currentView)
				return tea.Batch(
					a.currentView.Init(),
					a.currentView.Focus(),
					a.sendWindowSize(),
				)
			}
		}

	case OnboardingComplete:
		return a.enterDashboard()
	}

	return nil
}

func (a *UnifiedApp) enterDashboard() tea.Cmd {
	a.mode = ModeDashboard

	// Create dashboard views
	if a.createDashboardViews != nil {
		a.dashViews = a.createDashboardViews(a.client)
		if len(a.dashViews) > 0 {
			for _, v := range a.dashViews {
				a.attachAgentSelector(v)
			}
			a.updateCommands()
			a.currentView = a.dashViews[0]

			// Initialize all views
			var cmds []tea.Cmd
			for _, v := range a.dashViews {
				cmds = append(cmds, v.Init())
			}
			cmds = append(cmds, a.currentView.Focus())
			cmds = append(cmds, a.sendWindowSize())
			return tea.Batch(cmds...)
		}
	}
	return nil
}

func (a *UnifiedApp) updateCommands() {
	var cmds []Command

	cmds = append(cmds, Command{
		Name:        "Switch model",
		Description: "Toggle model selector",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return tea.KeyMsg{Type: tea.KeyF2}
			}
		},
	})
	cmds = append(cmds, Command{
		Name:        "Chat settings",
		Description: "Configure chat panel",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}}
			}
		},
	})
	cmds = append(cmds, Command{
		Name:        "Revert last run",
		Description: "Restore last agent snapshot",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return tea.KeyMsg{Type: tea.KeyCtrlU}
			}
		},
	})

	for i, v := range a.dashViews {
		idx := i
		name := v.Name()
		desc := fmt.Sprintf("View %s", strings.ToLower(name))
		cmds = append(cmds, Command{
			Name:        "Switch to " + name,
			Description: desc,
			Action:      func() tea.Cmd { return a.switchDashboardTab(idx) },
		})
		if provider, ok := v.(CommandProvider); ok {
			cmds = append(cmds, provider.Commands()...)
		}
	}

	a.palette.SetCommands(cmds)
}

func (a *UnifiedApp) openChatSettings() {
	a.chatSettingsOpen = true
	if a.chatSettingsView == nil {
		a.chatSettingsView = pkgtui.NewChatSettingsPanel(a.chatSettings)
	} else {
		a.chatSettingsView.Settings = a.chatSettings
	}
}

func (a *UnifiedApp) revertLastRun() tea.Cmd {
	if a.lastRunSnapshot == "" {
		return nil
	}
	a.sendToCurrentView(RevertLastRunMsg{Snapshot: a.lastRunSnapshot})
	return nil
}

func (a *UnifiedApp) switchDashboardTab(idx int) tea.Cmd {
	if idx < 0 || idx >= len(a.dashViews) {
		return nil
	}

	oldActive := a.tabs.Active()
	if oldActive == idx {
		return nil
	}

	// Blur old view
	if oldActive < len(a.dashViews) {
		a.dashViews[oldActive].Blur()
	}

	a.tabs.SetActive(idx)
	a.currentView = a.dashViews[idx]
	return a.currentView.Focus()
}

func (a *UnifiedApp) sendWindowSize() tea.Cmd {
	return func() tea.Msg {
		return tea.WindowSizeMsg{Width: a.width, Height: a.height}
	}
}

// View implements tea.Model
func (a *UnifiedApp) View() string {
	if a.width == 0 || a.height == 0 {
		return "Loading..."
	}

	// Calculate heights
	headerHeight := 3 // Header with padding
	footerHeight := 3 // Footer with padding
	contentHeight := a.height - headerHeight - footerHeight

	// Header area
	var header string
	if a.mode == ModeDashboard {
		header = a.tabs.View()
	} else {
		// Onboarding: show breadcrumb
		a.breadcrumb.SetWidth(a.width - 6) // Account for padding
		header = a.breadcrumb.View()
	}
	headerStyle := pkgtui.HeaderStyle.
		Width(a.width).
		Height(headerHeight)
	headerRendered := headerStyle.Render(header)

	// Content area
	var content string
	if a.currentView != nil {
		content = a.currentView.View()
	}

	// Apply content styling with padding
	contentStyle := lipgloss.NewStyle().
		Background(pkgtui.ColorBg).
		Foreground(pkgtui.ColorFg).
		Padding(1, 3).
		Width(a.width).
		Height(contentHeight)

	contentRendered := contentStyle.Render(content)

	// Footer
	footerStyle := pkgtui.FooterStyle.
		Width(a.width).
		Height(footerHeight)
	footerRendered := footerStyle.Render(a.renderFooterContent())

	// Join all sections vertically
	result := lipgloss.JoinVertical(lipgloss.Left,
		headerRendered,
		contentRendered,
		footerRendered,
	)

	// Overlay palette if visible
	if a.palette.Visible() {
		return a.overlay(result, a.palette.View())
	}

	if a.chatSettingsOpen && a.chatSettingsView != nil {
		return a.overlay(result, a.chatSettingsView.View())
	}

	// Overlay help if visible
	if a.showHelp {
		return a.overlay(result, a.renderHelpOverlay())
	}

	return result
}

func (a *UnifiedApp) onboardingHeader() string {
	switch a.onboardingState {
	case OnboardingKickoff:
		return "New Project"
	case OnboardingScanVision, OnboardingScanProblem, OnboardingScanUsers, OnboardingInterview:
		return "Project Setup"
	case OnboardingSpecSummary:
		return "Review Spec"
	case OnboardingEpicReview:
		return "Review Epics"
	case OnboardingTaskReview:
		return "Review Tasks"
	default:
		return "Autarch"
	}
}

func (a *UnifiedApp) renderFooterContent() string {
	help := ""
	if a.currentView != nil {
		help = a.currentView.ShortHelp()
	}

	if a.mode == ModeDashboard {
		help += "  │  ctrl+left/right tabs  ctrl+pgup/pgdn tabs  ctrl+p palette  ctrl+, settings  F1 help  F2 model  ctrl+c quit"
	} else {
		if a.breadcrumb.IsNavigating() {
			help = "←/→ navigate  enter select  esc cancel  ctrl+, settings  F1 help  F2 model"
		} else {
			help += "  │  ctrl+b jump  ctrl+, settings  F1 help  F2 model  ctrl+c quit"
		}
	}

	return help
}

// renderFooter is deprecated, use renderFooterContent
func (a *UnifiedApp) renderFooter() string {
	return a.renderFooterContent()
}

// renderHelpOverlay renders the full keybinding help overlay
func (a *UnifiedApp) renderHelpOverlay() string {
	var lines []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true).
		MarginBottom(1)

	viewName := "Help"
	if a.currentView != nil {
		viewName = a.currentView.Name() + " Help"
	}
	lines = append(lines, titleStyle.Render(viewName))
	lines = append(lines, "")

	// Get full help from view if it supports it
	var bindings []HelpBinding
	if provider, ok := a.currentView.(FullHelpProvider); ok {
		bindings = provider.FullHelp()
	} else {
		// Fall back to generic help from ShortHelp
		bindings = a.defaultHelpBindings()
	}

	// Render bindings
	keyStyle := pkgtui.HelpKeyStyle.Width(12)
	descStyle := pkgtui.HelpDescStyle

	for _, b := range bindings {
		line := keyStyle.Render(b.Key) + " " + descStyle.Render(b.Description)
		lines = append(lines, line)
	}

	// Global keys section
	lines = append(lines, "")
	lines = append(lines, titleStyle.Render("Global"))

	globalBindings := []HelpBinding{
		{Key: "F1", Description: "Show this help"},
		{Key: "ctrl+c", Description: "Quit"},
		{Key: "F2", Description: "Agent selector"},
	}

	if a.mode == ModeDashboard {
		globalBindings = append(globalBindings,
			HelpBinding{Key: "ctrl+left/right", Description: "Switch tabs"},
			HelpBinding{Key: "ctrl+pgup/pgdn", Description: "Switch tabs (fallback)"},
			HelpBinding{Key: "ctrl+p", Description: "Command palette"},
		)
	} else {
		globalBindings = append(globalBindings,
			HelpBinding{Key: "ctrl+b", Description: "Jump to step"},
		)
	}

	for _, b := range globalBindings {
		line := keyStyle.Render(b.Key) + " " + descStyle.Render(b.Description)
		lines = append(lines, line)
	}

	lines = append(lines, "")
	lines = append(lines, pkgtui.LabelStyle.Render("Press F1 or Esc to close"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	// Wrap in a box
	boxStyle := lipgloss.NewStyle().
		Background(pkgtui.ColorBgLight).
		Foreground(pkgtui.ColorFg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(pkgtui.ColorPrimary).
		Padding(1, 3).
		Width(50)

	return boxStyle.Render(content)
}

// defaultHelpBindings returns generic navigation help
func (a *UnifiedApp) defaultHelpBindings() []HelpBinding {
	return []HelpBinding{
		{Key: "up/down", Description: "Navigate"},
		{Key: "enter", Description: "Select/expand"},
		{Key: "esc", Description: "Back/cancel"},
	}
}

func (a *UnifiedApp) overlay(base, overlay string) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	startRow := (a.height - len(overlayLines)) / 4
	startCol := (a.width - lipgloss.Width(overlayLines[0])) / 2

	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	for i, line := range overlayLines {
		row := startRow + i
		if row >= len(baseLines) {
			break
		}
		baseLines[row] = insertAt(baseLines[row], startCol, line)
	}

	return strings.Join(baseLines, "\n")
}

func insertAt(base string, col int, overlay string) string {
	baseRunes := []rune(base)
	for len(baseRunes) < col {
		baseRunes = append(baseRunes, ' ')
	}

	overlayWidth := lipgloss.Width(overlay)
	var result strings.Builder

	if col > 0 && col < len(baseRunes) {
		result.WriteString(string(baseRunes[:col]))
	}
	result.WriteString(overlay)

	end := col + overlayWidth
	if end < len(baseRunes) {
		result.WriteString(string(baseRunes[end:]))
	}

	return result.String()
}

// RunUnified starts the unified TUI application
func RunUnified(client *autarch.Client, app *UnifiedApp) error {
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
