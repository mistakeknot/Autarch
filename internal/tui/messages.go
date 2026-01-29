package tui

import (
	"github.com/mistakeknot/autarch/internal/coldwine/epics"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// View transition messages

// ProjectCreatedMsg is sent when a new project is created from kickoff
type ProjectCreatedMsg struct {
	ProjectID   string
	ProjectName string
	Description string
	// Pre-populated answers from codebase scan (optional)
	ScanResult *CodebaseScanResultMsg
}

// SpecCompletedMsg is sent when the interview/spec is complete
type SpecCompletedMsg struct {
	Spec *specs.PRD
}

// EpicsGeneratedMsg is sent when epics are auto-generated
type EpicsGeneratedMsg struct {
	Epics []epics.EpicProposal
}

// EpicsAcceptedMsg is sent when user accepts the epics
type EpicsAcceptedMsg struct {
	Epics []epics.EpicProposal
}

// TasksGeneratedMsg is sent when tasks are generated from epics
type TasksGeneratedMsg struct {
	Tasks []tasks.TaskProposal
}

// TasksAcceptedMsg is sent when user accepts the tasks
type TasksAcceptedMsg struct {
	Tasks []tasks.TaskProposal
}

// OnboardingCompleteMsg signals onboarding is done
type OnboardingCompleteMsg struct {
	ProjectID   string
	ProjectName string
}

// NavigateToTaskDetailMsg requests navigation to task detail view
type NavigateToTaskDetailMsg struct {
	Task tasks.TaskProposal
}

// NavigateBackMsg requests navigation back to previous view
type NavigateBackMsg struct{}

// NavigateToKickoffMsg requests navigation back to the kickoff screen
type NavigateToKickoffMsg struct{}

// StartAgentMsg requests starting an agent for a task
type StartAgentMsg struct {
	Task     tasks.TaskProposal
	Agent    string
	Worktree bool
}

// InterviewCompleteMsg is sent when the interview is complete
type InterviewCompleteMsg struct {
	Answers map[string]string
}

// SpecAcceptedMsg is sent when user accepts the spec summary
type SpecAcceptedMsg struct {
	Vision       string
	Users        string
	Problem      string
	Platform     string
	Language     string
	Requirements []string
}

// SuggestionsReadyMsg is sent when AI suggestions are ready for interview questions
type SuggestionsReadyMsg struct {
	Suggestions map[string]string
	Error       error
}

// GeneratingMsg indicates something is being generated
type GeneratingMsg struct {
	What string // "suggestions", "epics", "tasks"
}

// GenerationErrorMsg indicates generation failed
type GenerationErrorMsg struct {
	What  string
	Error error
}

// AgentNotFoundMsg indicates no coding agent was found
type AgentNotFoundMsg struct {
	Instructions string
}

// ScanCodebaseMsg requests scanning an existing codebase
type ScanCodebaseMsg struct {
	Path string
}

// ScanProgressMsg reports progress during codebase scanning
type ScanProgressMsg struct {
	Step      string   // Current step name
	Details   string   // What's happening
	Files     []string // Files found/being analyzed (optional)
	AgentLine string   // Live output line from agent (if streaming)
}

// AgentRunStartedMsg indicates an agent run has started.
type AgentRunStartedMsg struct {
	What string
}

// AgentStreamMsg reports a live output line from an agent run.
type AgentStreamMsg struct {
	Line string
}

// AgentRunFinishedMsg indicates an agent run has finished.
type AgentRunFinishedMsg struct {
	What string
	Err  error
	Diff []string
}

// AgentEditSummaryMsg reports a summary of edits after an agent run.
type AgentEditSummaryMsg struct {
	Summary string
}

// RevertLastRunMsg requests reverting the last agent run snapshot.
type RevertLastRunMsg struct {
	Snapshot string
}

// CodebaseScanResultMsg contains the results of a codebase scan
type CodebaseScanResultMsg struct {
	ProjectName  string
	Description  string
	Vision       string
	Users        string
	Problem      string
	Platform     string
	Language     string
	Requirements []string
	Error        error
}
