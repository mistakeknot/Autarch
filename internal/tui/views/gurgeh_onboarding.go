package views

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/autarch/agent"
	"github.com/mistakeknot/autarch/internal/coldwine/epics"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/scan"
	"github.com/mistakeknot/autarch/internal/gurgeh/exploration"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/autarch"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// Local interface copies for unexported interfaces from internal/tui.
// These are needed because the original interfaces are unexported (lowercase).

type onboardingAgentSelectorSetter interface {
	SetAgentSelector(*pkgtui.AgentSelector)
}

type onboardingAgentNameSetter interface {
	SetAgentName(string)
}

type onboardingChatSettingsSetter interface {
	SetChatSettings(pkgtui.ChatSettings)
}

// GurgehOnboardingView encapsulates the full Gurgeh onboarding state machine,
// including view transitions, scan/generation goroutines, and agent streaming.
// It implements the pkgtui.View interface so it can be embedded in a container.
type GurgehOnboardingView struct {
	// Dependencies (from GurgehConfig)
	researchCoord *research.Coordinator
	codingAgent   *agent.Agent
	client        *autarch.Client
	agentSelector *pkgtui.AgentSelector
	selectedAgent string

	// Onboarding state
	state            tui.OnboardingState
	breadcrumb       *tui.Breadcrumb
	currentView      tui.View
	projectID        string
	projectName      string
	projectDesc      string
	interviewAnswers map[string]string
	generatedEpics   []epics.EpicProposal
	generatedTasks   []tasks.TaskProposal
	acceptedVision   string
	acceptedUsers    string
	acceptedProblem  string

	// Loading state
	generating     bool
	generatingWhat string

	// Agent run state
	lastRunLabel    string
	lastRunSnapshot string

	// Context
	ctx    context.Context
	cancel context.CancelFunc

	// Layout
	width  int
	height int

	// View factories
	createKickoffView     func() tui.View
	createArbiterView     func(*research.Coordinator) tui.View
	createSpecSummaryView func(*tui.SpecSummary, *research.Coordinator) tui.View
	createEpicReviewView  func([]epics.EpicProposal) tui.View
	createTaskReviewView  func([]tasks.TaskProposal) tui.View
	createTaskDetailView  func(tasks.TaskProposal, *research.Coordinator) tui.View
	createSprintView      func(string) tui.View

	// Chat settings (received from parent)
	chatSettings pkgtui.ChatSettings
}

// NewGurgehOnboardingView creates a new onboarding view from a GurgehConfig.
func NewGurgehOnboardingView(cfg tui.GurgehConfig) *GurgehOnboardingView {
	ctx, cancel := context.WithCancel(context.TODO())

	breadcrumb := tui.NewBreadcrumb()
	breadcrumb.SetCurrent(tui.OnboardingKickoff)

	return &GurgehOnboardingView{
		researchCoord: cfg.ResearchCoord,
		codingAgent:   cfg.CodingAgent,
		client:        cfg.Client,
		agentSelector: cfg.AgentSelector,
		selectedAgent: cfg.SelectedAgent,

		state:      tui.OnboardingKickoff,
		breadcrumb: breadcrumb,
		ctx:        ctx,
		cancel:     cancel,

		createKickoffView:     cfg.CreateKickoffView,
		createArbiterView:     cfg.CreateArbiterView,
		createSpecSummaryView: cfg.CreateSpecSummaryView,
		createEpicReviewView:  cfg.CreateEpicReviewView,
		createTaskReviewView:  cfg.CreateTaskReviewView,
		createTaskDetailView:  cfg.CreateTaskDetailView,
		createSprintView:      cfg.CreateSprintView,
	}
}

// --- View interface implementation ---

// Init implements pkgtui.View.
func (v *GurgehOnboardingView) Init() tea.Cmd {
	if v.createKickoffView != nil {
		v.currentView = v.createKickoffView()
		v.attachAgentSelector(v.currentView)
		return tea.Batch(
			v.currentView.Init(),
			v.currentView.Focus(),
		)
	}
	return nil
}

// Update implements pkgtui.View.
func (v *GurgehOnboardingView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		if v.currentView != nil {
			var cmd tea.Cmd
			v.currentView, cmd = v.currentView.Update(msg)
			return v, cmd
		}
		return v, nil

	// --- Onboarding transition messages ---

	case tui.ProjectCreatedMsg:
		return v, v.handleProjectCreated(msg)

	case tui.InterviewCompleteMsg:
		return v, v.handleInterviewComplete(msg)

	case tui.SuggestionsReadyMsg:
		return v, v.handleSuggestionsReady(msg)

	case tui.SpecAcceptedMsg:
		return v, v.handleSpecAccepted(msg)

	case tui.EpicsGeneratedMsg:
		v.generating = false
		return v, v.handleEpicsGenerated(msg)

	case tui.EpicsAcceptedMsg:
		return v, v.handleEpicsAccepted(msg)

	case tui.TasksGeneratedMsg:
		v.generating = false
		return v, v.handleTasksGenerated(msg)

	case tui.TasksAcceptedMsg:
		return v, v.handleTasksAccepted(msg)

	case tui.GeneratingMsg:
		v.generating = true
		v.generatingWhat = msg.What
		return v, nil

	case tui.GenerationErrorMsg:
		v.generating = false
		// Fall through to pass to currentView so SprintView can show errors in chat

	case tui.AgentNotFoundMsg:
		v.generating = false
		// Convert to GenerationErrorMsg so existing error handling in child views works
		errMsg := tui.GenerationErrorMsg{
			What:  v.generatingWhat,
			Error: fmt.Errorf("no coding agent found: %s", msg.Instructions),
		}
		if v.currentView != nil {
			var cmd tea.Cmd
			v.currentView, cmd = v.currentView.Update(errMsg)
			return v, cmd
		}
		return v, nil

	case tui.NavigateToTaskDetailMsg:
		return v, v.showTaskDetail(msg.Task)

	case tui.NavigateBackMsg:
		return v, v.navigateBack()

	case tui.NavigateToKickoffMsg:
		return v, v.navigateToKickoff()

	case tui.NavigateToStepMsg:
		return v, v.navigateToStep(msg.State)

	case tui.SprintCompleteMsg:
		return v, v.handleSprintComplete()

	case tui.ScanCodebaseMsg:
		// Auto-show log pane during scan, then start scan
		return v, tea.Batch(
			func() tea.Msg { return tui.LogPaneAutoShowMsg{} },
			v.scanCodebase(msg.Path),
		)

	case tui.CodebaseScanResultMsg:
		// Update onboarding state on successful scan
		if len(msg.ValidationErrors) == 0 {
			v.state = tui.OnboardingInterview
			v.breadcrumb.SetCurrent(tui.OnboardingInterview)
		}
		// Schedule auto-hide of log pane now that scan is done, then fall through to currentView
		var cmds []tea.Cmd
		cmds = append(cmds, func() tea.Msg { return tui.LogPaneScheduleAutoHideMsg{} })
		if v.currentView != nil {
			var viewCmd tea.Cmd
			v.currentView, viewCmd = v.currentView.Update(msg)
			cmds = append(cmds, viewCmd)
		}
		return v, tea.Batch(cmds...)

	case scanProgressWithContinuation:
		// Forward progress to current view and schedule next read
		if v.currentView != nil {
			var cmd tea.Cmd
			v.currentView, cmd = v.currentView.Update(msg.ScanProgressMsg)
			return v, tea.Batch(cmd, msg.nextCmd)
		}
		return v, msg.nextCmd

	case agentStreamWithContinuation:
		if setter, ok := v.currentView.(tui.ChatStreamSetter); ok {
			setter.AppendChatLine(msg.Line)
		}
		return v, msg.nextCmd

	case tui.AgentStreamMsg:
		if setter, ok := v.currentView.(tui.ChatStreamSetter); ok {
			setter.AppendChatLine(msg.Line)
		}
		return v, nil

	case tui.AgentRunStartedMsg:
		// Pass through to currentView
	}

	// Pass to current view
	if v.currentView != nil {
		var cmd tea.Cmd
		v.currentView, cmd = v.currentView.Update(msg)
		return v, cmd
	}

	return v, nil
}

// View implements pkgtui.View.
func (v *GurgehOnboardingView) View() string {
	if v.currentView != nil {
		return v.currentView.View()
	}
	return pkgtui.LabelStyle.Render("Loading Gurgeh onboarding...")
}

// Focus implements pkgtui.View.
func (v *GurgehOnboardingView) Focus() tea.Cmd {
	if v.currentView != nil {
		return v.currentView.Focus()
	}
	return nil
}

// Blur implements pkgtui.View.
func (v *GurgehOnboardingView) Blur() {
	if v.currentView != nil {
		v.currentView.Blur()
	}
}

// Name implements pkgtui.View.
func (v *GurgehOnboardingView) Name() string {
	return "Gurgeh"
}

// ShortHelp implements pkgtui.View.
func (v *GurgehOnboardingView) ShortHelp() string {
	if v.currentView != nil {
		return v.currentView.ShortHelp()
	}
	return ""
}

// Breadcrumb returns the onboarding breadcrumb for the parent to render.
func (v *GurgehOnboardingView) Breadcrumb() *tui.Breadcrumb {
	return v.breadcrumb
}

// State returns the current onboarding state.
func (v *GurgehOnboardingView) State() tui.OnboardingState {
	return v.state
}

// SetAgentSelector sets the shared agent selector.
func (v *GurgehOnboardingView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.agentSelector = selector
	v.attachAgentSelector(v.currentView)
}

// SetAgentName sets the selected agent name.
func (v *GurgehOnboardingView) SetAgentName(name string) {
	v.selectedAgent = name
	v.attachAgentName(v.currentView)
}

// SetChatSettings sets the chat settings to propagate to child views.
func (v *GurgehOnboardingView) SetChatSettings(settings pkgtui.ChatSettings) {
	v.chatSettings = settings
	v.attachChatSettings(v.currentView)
}

// --- Handler methods (moved from UnifiedApp) ---

func (v *GurgehOnboardingView) handleProjectCreated(msg tui.ProjectCreatedMsg) tea.Cmd {
	v.projectID = msg.ProjectID
	v.projectName = msg.ProjectName
	v.projectDesc = msg.Description
	v.interviewAnswers = make(map[string]string)

	// Transition to interview/arbiter
	v.state = tui.OnboardingInterview
	v.breadcrumb.SetCurrent(tui.OnboardingInterview)

	// Prefer unified SprintView (chat-driven 8-phase flow)
	if v.createSprintView != nil {
		projectPath := ""
		if cwd, err := os.Getwd(); err == nil {
			projectPath = cwd
		}
		v.currentView = v.createSprintView(projectPath)
		v.attachAgentSelector(v.currentView)

		// Set up back callback
		if cb, ok := v.currentView.(interface{ SetCallbacks(func() tea.Cmd) }); ok {
			cb.SetCallbacks(func() tea.Cmd {
				return func() tea.Msg { return tui.NavigateBackMsg{} }
			})
		}

		// Check for existing sprints to resume
		var startCmd tea.Cmd
		if lister, ok := v.currentView.(interface{ ListSprints() ([]string, error) }); ok {
			if ids, err := lister.ListSprints(); err == nil && len(ids) > 0 {
				// Resume the most recent sprint (IDs are sorted by list order)
				mostRecent := ids[len(ids)-1]
				if resumer, ok := v.currentView.(interface{ ResumeSprint(string) tea.Cmd }); ok {
					startCmd = resumer.ResumeSprint(mostRecent)
					slog.Info("resuming sprint", "id", mostRecent)
				}
			}
		}

		// If no existing sprint to resume, start a new one
		if startCmd == nil {
			if msg.ScanResult != nil {
				// Prefer the full method with exploration results for instant phase transitions
				if starter, ok := v.currentView.(interface {
					StartSprintWithExploration(string, *scan.Artifacts, map[string]any, string) tea.Cmd
				}); ok {
					startCmd = starter.StartSprintWithExploration(
						msg.Description,
						ScanResultToArtifacts(msg.ScanResult),
						msg.ScanResult.ExplorationResult,
						msg.ScanResult.ExplorationSessionID,
					)
				} else if starter, ok := v.currentView.(interface {
					StartSprintWithScan(string, *scan.Artifacts) tea.Cmd
				}); ok {
					startCmd = starter.StartSprintWithScan(msg.Description, ScanResultToArtifacts(msg.ScanResult))
				}
			}
			if startCmd == nil {
				if starter, ok := v.currentView.(tui.SprintStarter); ok {
					startCmd = starter.StartSprint(msg.Description)
				}
			}
		}

		cmds := []tea.Cmd{
			v.currentView.Init(),
			v.currentView.Focus(),
			v.sendWindowSize(),
		}
		if startCmd != nil {
			cmds = append(cmds, startCmd)
		}
		return tea.Batch(cmds...)
	}

	// Fall back to Arbiter view if available
	if v.createArbiterView != nil {
		v.currentView = v.createArbiterView(v.researchCoord)
		v.attachAgentSelector(v.currentView)

		// Set up callback for when sprint completes (backward-compatible)
		if iv, ok := v.currentView.(tui.InterviewViewSetter); ok {
			iv.SetCompleteCallback(func(answers map[string]string) tea.Cmd {
				return func() tea.Msg {
					return tui.InterviewCompleteMsg{Answers: answers}
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
			v.currentView.Init(),
			v.currentView.Focus(),
			v.sendWindowSize(),
		}
		return tea.Batch(cmds...)
	}

	// No arbiter view -- skip interview and go directly to spec summary
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
		return tui.InterviewCompleteMsg{Answers: answers}
	}
}

func (v *GurgehOnboardingView) handleInterviewComplete(msg tui.InterviewCompleteMsg) tea.Cmd {
	v.interviewAnswers = msg.Answers
	v.state = tui.OnboardingSpecSummary
	v.breadcrumb.SetCurrent(tui.OnboardingSpecSummary)

	// Create spec summary from answers
	spec := tui.CreateSpecSummaryFromAnswers(v.projectID, msg.Answers, nil)

	if v.createSpecSummaryView != nil {
		v.currentView = v.createSpecSummaryView(spec, v.researchCoord)
		v.attachAgentSelector(v.currentView)

		// Set up callbacks
		if sv, ok := v.currentView.(tui.SpecSummaryViewSetter); ok {
			sv.SetCallbacks(
				// onGenerateEpics
				func(s *tui.SpecSummary) tea.Cmd {
					return func() tea.Msg {
						return tui.SpecAcceptedMsg{
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
				func(s *tui.SpecSummary) tea.Cmd {
					return func() tea.Msg {
						return tui.NavigateBackMsg{}
					}
				},
				// onWaitResearch
				nil,
			)
		}

		return tea.Batch(
			v.currentView.Init(),
			v.currentView.Focus(),
			v.sendWindowSize(),
		)
	}
	return nil
}

func (v *GurgehOnboardingView) handleSuggestionsReady(msg tui.SuggestionsReadyMsg) tea.Cmd {
	if msg.Error != nil {
		// Suggestions failed, user will type manually - this is not fatal
		return nil
	}

	// Pass suggestions to the interview view
	if iv, ok := v.currentView.(tui.InterviewViewSetter); ok {
		iv.SetSuggestions(msg.Suggestions)
	}
	return nil
}

func (v *GurgehOnboardingView) handleSpecAccepted(msg tui.SpecAcceptedMsg) tea.Cmd {
	v.acceptedVision = msg.Vision
	v.acceptedUsers = msg.Users
	v.acceptedProblem = msg.Problem
	v.state = tui.OnboardingEpicReview
	v.generating = true
	v.generatingWhat = "epics"

	// Generate epics using the agent
	return v.generateEpicsWithAgent(msg)
}

func (v *GurgehOnboardingView) handleEpicsGenerated(msg tui.EpicsGeneratedMsg) tea.Cmd {
	v.finalizeAgentRun("epics")
	v.generatedEpics = msg.Epics
	v.breadcrumb.SetCurrent(tui.OnboardingEpicReview)

	// Show epic review view
	if v.createEpicReviewView != nil {
		v.currentView = v.createEpicReviewView(msg.Epics)
		v.attachAgentSelector(v.currentView)
		return tea.Batch(
			v.currentView.Init(),
			v.currentView.Focus(),
			v.sendWindowSize(),
		)
	}
	return nil
}

func (v *GurgehOnboardingView) handleEpicsAccepted(msg tui.EpicsAcceptedMsg) tea.Cmd {
	v.generatedEpics = msg.Epics
	v.state = tui.OnboardingTaskReview
	v.generating = true
	v.generatingWhat = "tasks"

	// Generate tasks from epics using the agent
	return v.generateTasksWithAgent()
}

func (v *GurgehOnboardingView) handleTasksGenerated(msg tui.TasksGeneratedMsg) tea.Cmd {
	v.finalizeAgentRun("tasks")
	v.generatedTasks = msg.Tasks
	v.breadcrumb.SetCurrent(tui.OnboardingTaskReview)

	// Show task review view
	if v.createTaskReviewView != nil {
		v.currentView = v.createTaskReviewView(msg.Tasks)
		v.attachAgentSelector(v.currentView)
		return tea.Batch(
			v.currentView.Init(),
			v.currentView.Focus(),
			v.sendWindowSize(),
		)
	}
	return nil
}

func (v *GurgehOnboardingView) handleTasksAccepted(msg tui.TasksAcceptedMsg) tea.Cmd {
	v.generatedTasks = msg.Tasks
	v.state = tui.OnboardingComplete
	v.breadcrumb.SetCurrent(tui.OnboardingComplete)

	return v.persistAndComplete()
}

func (v *GurgehOnboardingView) persistAndComplete() tea.Cmd {
	return func() tea.Msg {
		specID := ""
		if v.client != nil {
			spec := autarch.Spec{
				Title:   v.projectName,
				Project: v.projectID,
				Vision:  v.acceptedVision,
				Users:   v.acceptedUsers,
				Problem: v.acceptedProblem,
				Status:  autarch.SpecStatusDraft,
			}
			created, err := v.client.CreateSpec(spec)
			if err == nil {
				specID = created.ID
			}
		}
		return tui.OnboardingCompleteMsg{
			ProjectID:   v.projectID,
			ProjectName: v.projectName,
			SpecID:      specID,
		}
	}
}

func (v *GurgehOnboardingView) handleSprintComplete() tea.Cmd {
	// Handle at parent level: transition to SpecSummaryView with sprint results.
	if provider, ok := v.currentView.(tui.SprintStateProvider); ok {
		state, stateOK := provider.Orchestrator().State()
		if stateOK && v.createSpecSummaryView != nil {
			spec := CreateSpecSummaryFromSprintState(&state)
			v.state = tui.OnboardingSpecSummary
			v.breadcrumb.SetCurrent(tui.OnboardingSpecSummary)
			v.currentView = v.createSpecSummaryView(spec, v.researchCoord)
			v.attachAgentSelector(v.currentView)

			// Set up callbacks (same as handleInterviewComplete)
			if sv, ok := v.currentView.(tui.SpecSummaryViewSetter); ok {
				sv.SetCallbacks(
					func(s *tui.SpecSummary) tea.Cmd {
						return func() tea.Msg {
							return tui.SpecAcceptedMsg{
								Vision:       s.Vision,
								Users:        s.Users,
								Problem:      s.Problem,
								Platform:     s.Platform,
								Language:     s.Language,
								Requirements: s.Requirements,
							}
						}
					},
					func(s *tui.SpecSummary) tea.Cmd {
						return func() tea.Msg { return tui.NavigateBackMsg{} }
					},
					nil,
				)
			}

			return tea.Batch(
				v.currentView.Init(),
				v.currentView.Focus(),
				v.sendWindowSize(),
			)
		}
	}
	// Fallback: proceed to dashboard if view transition fails
	return func() tea.Msg {
		return tui.OnboardingCompleteMsg{
			ProjectID:   v.projectID,
			ProjectName: v.projectName,
		}
	}
}

// --- Navigation methods ---

func (v *GurgehOnboardingView) showTaskDetail(task tasks.TaskProposal) tea.Cmd {
	if v.createTaskDetailView != nil {
		v.currentView = v.createTaskDetailView(task, v.researchCoord)
		v.attachAgentSelector(v.currentView)
		return tea.Batch(
			v.currentView.Init(),
			v.currentView.Focus(),
			v.sendWindowSize(),
		)
	}
	return nil
}

func (v *GurgehOnboardingView) navigateBack() tea.Cmd {
	v.blurCurrentView()
	// Return to appropriate view based on state
	switch v.state {
	case tui.OnboardingEpicReview:
		return v.navigateToKickoff()
	case tui.OnboardingTaskReview:
		// Go back to epic review
		v.state = tui.OnboardingEpicReview
		v.breadcrumb.SetCurrent(tui.OnboardingEpicReview)
		if v.createEpicReviewView != nil {
			v.currentView = v.createEpicReviewView(v.generatedEpics)
			v.attachAgentSelector(v.currentView)
			return tea.Batch(v.currentView.Init(), v.currentView.Focus(), v.sendWindowSize())
		}
	}
	return nil
}

func (v *GurgehOnboardingView) navigateToKickoff() tea.Cmd {
	v.blurCurrentView()
	v.state = tui.OnboardingKickoff
	v.breadcrumb.SetCurrent(tui.OnboardingKickoff)
	// Clear any generated data
	v.generatedEpics = nil
	v.generatedTasks = nil
	v.projectID = ""
	v.projectName = ""
	v.projectDesc = ""

	if v.createKickoffView != nil {
		v.currentView = v.createKickoffView()
		v.attachAgentSelector(v.currentView)
		return tea.Batch(
			v.currentView.Init(),
			v.currentView.Focus(),
			v.sendWindowSize(),
		)
	}
	return nil
}

func (v *GurgehOnboardingView) navigateToStep(state tui.OnboardingState) tea.Cmd {
	// Only allow navigation to unlocked steps
	switch state {
	case tui.OnboardingKickoff:
		return v.navigateToKickoff()

	case tui.OnboardingEpicReview:
		// Only if we have generated epics
		if len(v.generatedEpics) > 0 {
			v.state = tui.OnboardingEpicReview
			v.breadcrumb.SetCurrent(tui.OnboardingEpicReview)
			if v.createEpicReviewView != nil {
				v.currentView = v.createEpicReviewView(v.generatedEpics)
				v.attachAgentSelector(v.currentView)
				return tea.Batch(
					v.currentView.Init(),
					v.currentView.Focus(),
					v.sendWindowSize(),
				)
			}
		}

	case tui.OnboardingTaskReview:
		// Only if we have generated tasks
		if len(v.generatedTasks) > 0 {
			v.state = tui.OnboardingTaskReview
			v.breadcrumb.SetCurrent(tui.OnboardingTaskReview)
			if v.createTaskReviewView != nil {
				v.currentView = v.createTaskReviewView(v.generatedTasks)
				v.attachAgentSelector(v.currentView)
				return tea.Batch(
					v.currentView.Init(),
					v.currentView.Focus(),
					v.sendWindowSize(),
				)
			}
		}

	case tui.OnboardingComplete:
		return func() tea.Msg {
			return tui.OnboardingCompleteMsg{
				ProjectID:   v.projectID,
				ProjectName: v.projectName,
			}
		}
	}

	return nil
}

// --- Generation methods ---

func (v *GurgehOnboardingView) generateSuggestions() tea.Cmd {
	if v.codingAgent == nil {
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

		suggestions, err := agent.SuggestAnswers(v.ctx, v.codingAgent, v.projectDesc, questions)
		return tui.SuggestionsReadyMsg{Suggestions: suggestions, Error: err}
	}
}

func (v *GurgehOnboardingView) generateEpicsWithAgent(spec tui.SpecAcceptedMsg) tea.Cmd {
	if v.codingAgent == nil {
		// No agent - show error with instructions
		return func() tea.Msg {
			return tui.AgentNotFoundMsg{
				Instructions: (&agent.NoAgentError{}).Instructions(),
			}
		}
	}

	v.captureRunSnapshot()
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

		proposals, err := agent.GenerateEpicsWithOutput(v.ctx, v.codingAgent, input, outputCallback)
		if err != nil {
			stream <- agentStreamEvent{err: err}
			return
		}
		stream <- agentStreamEvent{epics: proposals}
	}()

	return tea.Batch(
		func() tea.Msg { return tui.AgentRunStartedMsg{What: "epics"} },
		v.waitForAgentStream(stream, "epics"),
	)
}

func (v *GurgehOnboardingView) generateTasksWithAgent() tea.Cmd {
	if v.codingAgent == nil {
		// No agent - show error with instructions
		return func() tea.Msg {
			return tui.AgentNotFoundMsg{
				Instructions: (&agent.NoAgentError{}).Instructions(),
			}
		}
	}

	v.captureRunSnapshot()
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

		taskList, err := agent.GenerateTasksWithOutput(v.ctx, v.codingAgent, v.generatedEpics, outputCallback)
		if err != nil {
			stream <- agentStreamEvent{err: err}
			return
		}
		stream <- agentStreamEvent{tasks: taskList}
	}()

	return tea.Batch(
		func() tea.Msg { return tui.AgentRunStartedMsg{What: "tasks"} },
		v.waitForAgentStream(stream, "tasks"),
	)
}

// --- Scan methods ---

func (v *GurgehOnboardingView) scanCodebase(path string) tea.Cmd {
	// Create a channel for progress updates
	progressChan := make(chan agent.ScanProgress, 100)

	// Start exploration in a goroutine (Claude Code is the primary scan method)
	// Progress is reported via slog, which appears in the log pane
	go func() {
		defer close(progressChan)

		// Send initial progress to TUI
		progressChan <- agent.ScanProgress{Step: "Exploring", Details: "Running Claude Code..."}

		// Run Claude Code exploration (progress logged via slog)
		exploreResult, sessionID, err := exploration.Explore(v.ctx, path)

		if err != nil {
			progressChan <- agent.ScanProgress{Step: "_error", Details: err.Error()}
			return
		}

		// Convert exploration results to ScanResult format
		artifacts := exploration.MergeIntoArtifacts(exploreResult, nil)

		// Extract basic info from exploration results for backward compatibility
		projectName := ExtractString(exploreResult, "project_name")
		if projectName == "" {
			// Fallback to directory name
			projectName = filepath.Base(path)
		}

		result := &agent.ScanResult{
			ProjectName:    projectName,
			Description:    ExtractString(exploreResult, "description"),
			Vision:         ExtractVisionSummary(artifacts),
			Users:          ExtractUsersSummary(artifacts, exploreResult),
			Problem:        ExtractProblemSummary(artifacts),
			Platform:       ExtractString(exploreResult, "platform"),
			Language:       ExtractString(exploreResult, "language"),
			PhaseArtifacts: artifacts,
		}

		// Encode result in progress for simplicity
		progressChan <- agent.ScanProgress{
			Step:                 "_complete",
			Details:              result.ProjectName,
			Files:                []string{result.Description, result.Vision, result.Users, result.Problem, result.Platform, result.Language, strings.Join(result.Requirements, "|||")},
			PhaseArtifacts:       result.PhaseArtifacts,
			ExplorationResult:    exploreResult,
			ExplorationSessionID: sessionID,
		}
	}()

	// Return a command that reads from the progress channel
	return v.waitForScanProgress(progressChan)
}

// waitForScanProgress reads one progress update from the channel and returns it as a message.
func (v *GurgehOnboardingView) waitForScanProgress(ch <-chan agent.ScanProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			// Channel closed unexpectedly
			return tui.CodebaseScanResultMsg{Error: fmt.Errorf("scan interrupted")}
		}

		// Check for special completion messages
		if p.Step == "_error" {
			return tui.CodebaseScanResultMsg{Error: fmt.Errorf("%s", p.Details)}
		}
		if p.Step == "_complete" {
			// Decode result from Files array
			var requirements []string
			if len(p.Files) >= 7 && p.Files[6] != "" {
				requirements = strings.Split(p.Files[6], "|||")
			}
			return tui.CodebaseScanResultMsg{
				ProjectName:          p.Details,
				Description:          SafeIndex(p.Files, 0),
				Vision:               SafeIndex(p.Files, 1),
				Users:                SafeIndex(p.Files, 2),
				Problem:              SafeIndex(p.Files, 3),
				Platform:             SafeIndex(p.Files, 4),
				Language:             SafeIndex(p.Files, 5),
				Requirements:         requirements,
				ValidationErrors:     ToValidationErrors(p.ValidationErrors),
				PhaseArtifacts:       ToPhaseArtifacts(p.PhaseArtifacts),
				ExplorationResult:    p.ExplorationResult,
				ExplorationSessionID: p.ExplorationSessionID,
			}
		}

		// Return progress message and schedule next read
		return scanProgressWithContinuation{
			ScanProgressMsg: tui.ScanProgressMsg{
				Step:      p.Step,
				Details:   p.Details,
				Files:     p.Files,
				AgentLine: p.AgentLine,
			},
			nextCmd: v.waitForScanProgress(ch),
		}
	}
}

// --- Agent stream methods ---

func (v *GurgehOnboardingView) waitForAgentStream(ch <-chan agentStreamEvent, what string) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return tui.GenerationErrorMsg{What: what, Error: fmt.Errorf("%s generation interrupted", what)}
		}

		if ev.line != "" {
			return agentStreamWithContinuation{
				AgentStreamMsg: tui.AgentStreamMsg{Line: ev.line},
				nextCmd:        v.waitForAgentStream(ch, what),
			}
		}

		if ev.err != nil {
			return tui.GenerationErrorMsg{What: what, Error: ev.err}
		}

		if len(ev.epics) > 0 {
			return tui.EpicsGeneratedMsg{Epics: ev.epics}
		}

		if len(ev.tasks) > 0 {
			return tui.TasksGeneratedMsg{Tasks: ev.tasks}
		}

		return tui.GenerationErrorMsg{What: what, Error: fmt.Errorf("%s generation interrupted", what)}
	}
}

// --- Snapshot methods ---

func (v *GurgehOnboardingView) captureRunSnapshot() {
	v.lastRunLabel = ""
	v.lastRunSnapshot = ""
	if snapper, ok := v.currentView.(tui.DocumentSnapshotter); ok {
		label, content := snapper.DocumentSnapshot()
		v.lastRunLabel = label
		v.lastRunSnapshot = content
	}
}

func (v *GurgehOnboardingView) finalizeAgentRun(what string) {
	var diff []string
	var diffErr error

	if v.lastRunSnapshot != "" {
		if snapper, ok := v.currentView.(tui.DocumentSnapshotter); ok {
			label, after := snapper.DocumentSnapshot()
			if label != "" {
				v.lastRunLabel = label
			}
			diff, diffErr = pkgtui.UnifiedDiff(v.lastRunSnapshot, after, v.lastRunLabel)
		}
	}

	v.sendToCurrentView(tui.AgentRunFinishedMsg{
		What: what,
		Err:  diffErr,
		Diff: diff,
	})

	summary := SummarizeDiff(diff, diffErr)
	if summary != "" {
		v.sendToCurrentView(tui.AgentEditSummaryMsg{Summary: summary})
	}
}

// BUG(phase2c): sendToCurrentView discards the tea.Cmd returned by Update().
// Any commands the view returns (timers, IO, focus requests) are silently lost.
// This is called from goroutines (handleAgentRun) which cannot return commands
// to the Bubble Tea runtime. Fix in Phase 2c by converting to p.Send() pattern.
func (v *GurgehOnboardingView) sendToCurrentView(msg tea.Msg) {
	if v.currentView == nil {
		return
	}
	v.currentView, _ = v.currentView.Update(msg)
}

func (v *GurgehOnboardingView) blurCurrentView() {
	if v.currentView != nil {
		v.currentView.Blur()
	}
}

// --- Attach helpers ---

func (v *GurgehOnboardingView) attachAgentSelector(view tui.View) {
	if view == nil {
		return
	}
	if v.agentSelector != nil {
		if setter, ok := view.(onboardingAgentSelectorSetter); ok {
			setter.SetAgentSelector(v.agentSelector)
		}
	}
	v.attachAgentName(view)
	v.attachChatSettings(view)
}

func (v *GurgehOnboardingView) attachAgentName(view tui.View) {
	if view == nil || v.selectedAgent == "" {
		return
	}
	if setter, ok := view.(onboardingAgentNameSetter); ok {
		setter.SetAgentName(v.selectedAgent)
	}
}

func (v *GurgehOnboardingView) attachChatSettings(view tui.View) {
	if view == nil {
		return
	}
	if setter, ok := view.(onboardingChatSettingsSetter); ok {
		setter.SetChatSettings(v.chatSettings)
	}
}

func (v *GurgehOnboardingView) sendWindowSize() tea.Cmd {
	return func() tea.Msg {
		return tea.WindowSizeMsg{Width: v.width, Height: v.height}
	}
}

// Compile-time assertion: GurgehOnboardingView implements View.
var _ tui.View = (*GurgehOnboardingView)(nil)
