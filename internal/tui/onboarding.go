package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// OnboardingState tracks where we are in the onboarding flow
type OnboardingState int

const (
	OnboardingKickoff OnboardingState = iota
	OnboardingInterview
	OnboardingSpecSummary
	OnboardingEpicReview
	OnboardingTaskReview
	OnboardingComplete
)

// AllOnboardingStates returns all onboarding states in order.
func AllOnboardingStates() []OnboardingState {
	return []OnboardingState{
		OnboardingKickoff,
		OnboardingInterview,
		OnboardingSpecSummary,
		OnboardingEpicReview,
		OnboardingTaskReview,
		OnboardingComplete,
	}
}

// ID returns a stable identifier for the state.
func (s OnboardingState) ID() string {
	switch s {
	case OnboardingKickoff:
		return "kickoff"
	case OnboardingInterview:
		return "interview"
	case OnboardingSpecSummary:
		return "spec"
	case OnboardingEpicReview:
		return "epics"
	case OnboardingTaskReview:
		return "tasks"
	case OnboardingComplete:
		return "dashboard"
	default:
		return "unknown"
	}
}

// Label returns the display label for the state.
func (s OnboardingState) Label() string {
	switch s {
	case OnboardingKickoff:
		return "Project"
	case OnboardingInterview:
		return "Interview"
	case OnboardingSpecSummary:
		return "Spec"
	case OnboardingEpicReview:
		return "Epics"
	case OnboardingTaskReview:
		return "Tasks"
	case OnboardingComplete:
		return "Dashboard"
	default:
		return "Unknown"
	}
}

// OnboardingOrchestrator manages the new project onboarding flow
type OnboardingOrchestrator struct {
	state       OnboardingState
	currentView View
	projectID   string
	projectName string
	width       int
	height      int

	// Context for cancelling research on project switch
	ctx    context.Context
	cancel context.CancelFunc

	// Callbacks to notify parent
	onComplete func(projectID, projectName string) tea.Cmd
	onCancel   func() tea.Cmd
	keys       pkgtui.CommonKeys
}

// NewOnboardingOrchestrator creates a new onboarding orchestrator
func NewOnboardingOrchestrator() *OnboardingOrchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	return &OnboardingOrchestrator{
		state:  OnboardingKickoff,
		ctx:    ctx,
		cancel: cancel,
		keys:   pkgtui.NewCommonKeys(),
	}
}

// SetCallbacks sets completion callbacks
func (o *OnboardingOrchestrator) SetCallbacks(
	onComplete func(projectID, projectName string) tea.Cmd,
	onCancel func() tea.Cmd,
) {
	o.onComplete = onComplete
	o.onCancel = onCancel
}

// SetView sets the current view (called by parent to inject views)
func (o *OnboardingOrchestrator) SetView(v View) {
	o.currentView = v
}

// State returns the current onboarding state
func (o *OnboardingOrchestrator) State() OnboardingState {
	return o.state
}

// SetState advances to a new state
func (o *OnboardingOrchestrator) SetState(s OnboardingState) {
	o.state = s
}

// ProjectInfo returns the current project info
func (o *OnboardingOrchestrator) ProjectInfo() (string, string) {
	return o.projectID, o.projectName
}

// SetProjectInfo sets the project info
func (o *OnboardingOrchestrator) SetProjectInfo(id, name string) {
	o.projectID = id
	o.projectName = name
}

// Init implements tea.Model
func (o *OnboardingOrchestrator) Init() tea.Cmd {
	if o.currentView != nil {
		return o.currentView.Init()
	}
	return nil
}

// Update implements tea.Model
func (o *OnboardingOrchestrator) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		o.width = msg.Width
		o.height = msg.Height
		if o.currentView != nil {
			var cmd tea.Cmd
			o.currentView, cmd = o.currentView.Update(msg)
			return o, cmd
		}
		return o, nil

	case tea.KeyMsg:
		// Global escape to cancel onboarding
		if key.Matches(msg, o.keys.Quit) {
			o.cancel()
			if o.onCancel != nil {
				return o, o.onCancel()
			}
			return o, tea.Quit
		}
	}

	// Pass to current view
	if o.currentView != nil {
		var cmd tea.Cmd
		o.currentView, cmd = o.currentView.Update(msg)
		return o, cmd
	}

	return o, nil
}

// View implements tea.Model
func (o *OnboardingOrchestrator) View() string {
	if o.currentView != nil {
		return o.currentView.View()
	}

	// Fallback loading state
	return lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted).
		Render("Loading...")
}

// Complete marks onboarding as complete and notifies parent
func (o *OnboardingOrchestrator) Complete() tea.Cmd {
	o.state = OnboardingComplete
	if o.onComplete != nil {
		return o.onComplete(o.projectID, o.projectName)
	}
	return nil
}

// Context returns the orchestrator's context for research coordination
func (o *OnboardingOrchestrator) Context() context.Context {
	return o.ctx
}

// Cancel cancels any running operations
func (o *OnboardingOrchestrator) Cancel() {
	o.cancel()
}
