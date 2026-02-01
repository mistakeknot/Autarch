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
	ProjectName      string
	Description      string
	Vision           string
	Users            string
	Problem          string
	Platform         string
	Language         string
	Requirements     []string
	ValidationErrors []ValidationError
	PhaseArtifacts   *PhaseArtifacts
	Error            error
}

// OpenQuestionsRequest asks the agent to resolve open questions for a scan phase.
type OpenQuestionsRequest struct {
	Phase         string
	Summary       string
	Evidence      []EvidenceItem
	OpenQuestions []string
	UserAnswer    string
	Vision        string
	Problem       string
	Users         string
	Platform      string
	Language      string
	Requirements  []string
}

// OpenQuestionsResolvedMsg returns resolved and remaining questions for a phase.
type OpenQuestionsResolvedMsg struct {
	Phase     string
	Resolved  []ResolvedQuestion
	Remaining []string
	Err       error
}

// ValidationError represents a scan validation issue surfaced to the UI.
type ValidationError struct {
	Code    string
	Field   string
	Message string
}

type PhaseArtifacts struct {
	Vision  *VisionArtifact
	Problem *ProblemArtifact
	Users   *UsersArtifact
}

type EvidenceItem struct {
	Type       string
	Path       string
	Quote      string
	Confidence float64
}

type QualityScores struct {
	Clarity      float64
	Completeness float64
	Grounding    float64
	Consistency  float64
}

type VisionArtifact struct {
	Phase             string
	Version           string
	Summary           string
	Goals             []string
	NonGoals          []string
	Evidence          []EvidenceItem
	ResolvedQuestions []ResolvedQuestion
	OpenQuestions     []string
	Quality           QualityScores
}

type ProblemArtifact struct {
	Phase             string
	Version           string
	Summary           string
	PainPoints        []string
	Impact            string
	Evidence          []EvidenceItem
	ResolvedQuestions []ResolvedQuestion
	OpenQuestions     []string
	Quality           QualityScores
}

type UsersArtifact struct {
	Phase             string
	Version           string
	Personas          []Persona
	Evidence          []EvidenceItem
	ResolvedQuestions []ResolvedQuestion
	OpenQuestions     []string
	Quality           QualityScores
}

type Persona struct {
	Name    string
	Needs   []string
	Context string
}

type ResolvedQuestion struct {
	Question string
	Answer   string
}

// ScanSignoffCompleteMsg signals scan signoff completion.
type ScanSignoffCompleteMsg struct {
	Answers map[string]string
}

// SprintStreamLineMsg carries a streaming agent response chunk.
type SprintStreamLineMsg struct {
	Content string
}

// SprintStreamDoneMsg signals the agent response stream is complete.
type SprintStreamDoneMsg struct{}

// SprintDraftUpdatedMsg signals the current phase draft has been updated.
type SprintDraftUpdatedMsg struct {
	Phase   string
	Content string
}

// SprintPhaseAdvancedMsg signals the sprint has advanced to a new phase.
type SprintPhaseAdvancedMsg struct {
	Phase string
}

// SprintConflictMsg signals consistency conflicts were found.
type SprintConflictMsg struct {
	Phase    string
	Messages []string
}
