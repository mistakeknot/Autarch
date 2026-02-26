package tui

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

// InterviewStep represents a sub-step within the interview flow.
type InterviewStep struct {
	ID    string
	Label string
}

// InterviewSteps returns the shared Arbiter phase list for onboarding sidebars.
func InterviewSteps() []InterviewStep {
	return []InterviewStep{
		{ID: "vision", Label: "Vision"},
		{ID: "problem", Label: "Problem"},
		{ID: "users", Label: "Users"},
		{ID: "features", Label: "Features + Goals"},
		{ID: "requirements", Label: "Requirements"},
		{ID: "scope", Label: "Scope + Assumptions"},
		{ID: "cujs", Label: "Critical User Journeys"},
		{ID: "acceptance", Label: "Acceptance Criteria"},
	}
}
