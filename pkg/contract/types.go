// Package contract provides shared entity types for cross-tool communication in Autarch.
// These types form the unified data contract between Bigend, Gurgeh, Coldwine, and Pollard.
package contract

import "time"

// Status represents the lifecycle status of an initiative or epic
type Status string

const (
	StatusDraft      Status = "draft"
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusClosed     Status = "closed"
)

// TaskStatus represents the status of a task
type TaskStatus string

const (
	TaskStatusTodo       TaskStatus = "todo"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusDone       TaskStatus = "done"
)

// RunState represents the state of an agent run
type RunState string

const (
	RunStateWorking RunState = "working"
	RunStateWaiting RunState = "waiting"
	RunStateBlocked RunState = "blocked"
	RunStateDone    RunState = "done"
)

// Complexity represents t-shirt sizing for work items
type Complexity string

const (
	ComplexityXS Complexity = "xs"
	ComplexityS  Complexity = "s"
	ComplexityM  Complexity = "m"
	ComplexityL  Complexity = "l"
	ComplexityXL Complexity = "xl"
)

// SourceTool identifies which tool created or owns an entity
type SourceTool string

const (
	SourceGurgeh  SourceTool = "gurgeh"
	SourceColdwine SourceTool = "coldwine"
	SourcePollard  SourceTool = "pollard"
	SourceBigend   SourceTool = "bigend"
)

// Initiative represents a high-level product or feature initiative.
// In Gurgeh, this maps to a Spec. In Coldwine, this may be a top-level grouping.
type Initiative struct {
	ID          string     `json:"id" yaml:"id"`
	Title       string     `json:"title" yaml:"title"`
	Description string     `json:"description,omitempty" yaml:"description,omitempty"`
	Status      Status     `json:"status" yaml:"status"`
	Priority    int        `json:"priority" yaml:"priority"`
	SourceTool  SourceTool `json:"source_tool" yaml:"source_tool"`
	ProjectPath string     `json:"project_path,omitempty" yaml:"project_path,omitempty"`
	CreatedAt   time.Time  `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" yaml:"updated_at"`
}

// Epic represents a large body of work, broken into stories.
// Links to an Initiative via InitiativeID and to Gurgeh specs via FeatureRef.
type Epic struct {
	ID           string     `json:"id" yaml:"id"`
	InitiativeID string     `json:"initiative_id,omitempty" yaml:"initiative_id,omitempty"`
	FeatureRef   string     `json:"feature_ref,omitempty" yaml:"feature_ref,omitempty"` // Links to Gurgeh spec ID
	Title        string     `json:"title" yaml:"title"`
	Description  string     `json:"description,omitempty" yaml:"description,omitempty"`
	Status       Status     `json:"status" yaml:"status"`
	Priority     int        `json:"priority" yaml:"priority"`
	SourceTool   SourceTool `json:"source_tool" yaml:"source_tool"`
	CreatedAt    time.Time  `json:"created_at" yaml:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" yaml:"updated_at"`
}

// Story represents a user story within an epic.
type Story struct {
	ID          string     `json:"id" yaml:"id"`
	EpicID      string     `json:"epic_id" yaml:"epic_id"`
	Title       string     `json:"title" yaml:"title"`
	Description string     `json:"description,omitempty" yaml:"description,omitempty"`
	Status      Status     `json:"status" yaml:"status"`
	Priority    int        `json:"priority" yaml:"priority"`
	Complexity  Complexity `json:"complexity" yaml:"complexity"`
	Assignee    string     `json:"assignee,omitempty" yaml:"assignee,omitempty"`
	SourceTool  SourceTool `json:"source_tool" yaml:"source_tool"`
	CreatedAt   time.Time  `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" yaml:"updated_at"`
}

// Task represents an implementable unit of work within a story.
type Task struct {
	ID          string     `json:"id" yaml:"id"`
	StoryID     string     `json:"story_id" yaml:"story_id"`
	Title       string     `json:"title" yaml:"title"`
	Description string     `json:"description,omitempty" yaml:"description,omitempty"`
	Status      TaskStatus `json:"status" yaml:"status"`
	Priority    int        `json:"priority" yaml:"priority"`
	Assignee    string     `json:"assignee,omitempty" yaml:"assignee,omitempty"`
	WorktreeRef string     `json:"worktree_ref,omitempty" yaml:"worktree_ref,omitempty"` // Git worktree path
	SessionRef  string     `json:"session_ref,omitempty" yaml:"session_ref,omitempty"`  // Agent session ID
	SourceTool  SourceTool `json:"source_tool" yaml:"source_tool"`
	CreatedAt   time.Time  `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" yaml:"updated_at"`
}

// Run represents an agent working on a task.
type Run struct {
	ID           string     `json:"id" yaml:"id"`
	TaskID       string     `json:"task_id" yaml:"task_id"`
	AgentName    string     `json:"agent_name" yaml:"agent_name"`
	AgentProgram string     `json:"agent_program" yaml:"agent_program"` // claude, codex, aider
	State        RunState   `json:"state" yaml:"state"`
	WorktreePath string     `json:"worktree_path,omitempty" yaml:"worktree_path,omitempty"`
	SourceTool   SourceTool `json:"source_tool" yaml:"source_tool"`
	StartedAt    time.Time  `json:"started_at" yaml:"started_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty" yaml:"ended_at,omitempty"`
}

// Outcome represents the result of an agent run.
type Outcome struct {
	ID         string     `json:"id" yaml:"id"`
	RunID      string     `json:"run_id" yaml:"run_id"`
	Success    bool       `json:"success" yaml:"success"`
	Summary    string     `json:"summary" yaml:"summary"`
	Artifacts  []string   `json:"artifacts,omitempty" yaml:"artifacts,omitempty"` // File paths, commit SHAs
	SourceTool SourceTool `json:"source_tool" yaml:"source_tool"`
	CreatedAt  time.Time  `json:"created_at" yaml:"created_at"`
}

// InsightLink represents a connection between a Pollard insight and an initiative/feature.
type InsightLink struct {
	InsightID     string    `json:"insight_id" yaml:"insight_id"`
	InitiativeID  string    `json:"initiative_id,omitempty" yaml:"initiative_id,omitempty"`
	FeatureRef    string    `json:"feature_ref,omitempty" yaml:"feature_ref,omitempty"` // Gurgeh spec ID
	LinkedAt      time.Time `json:"linked_at" yaml:"linked_at"`
	LinkedBy      string    `json:"linked_by,omitempty" yaml:"linked_by,omitempty"` // Agent or user
}
