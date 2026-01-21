package tandemonium

import "time"

// TasksFile represents the root structure of tasks.yml
type TasksFile struct {
	Version   int       `yaml:"version"`
	Rev       int       `yaml:"rev"`
	UpdatedAt time.Time `yaml:"updated_at"`
	Data      TasksData `yaml:"data"`
}

// TasksData contains the tasks array
type TasksData struct {
	Tasks []Task `yaml:"tasks"`
}

// Task represents a Tandemonium task
type Task struct {
	ID                 string              `yaml:"id" json:"id"`
	Slug               string              `yaml:"slug" json:"slug"`
	Title              string              `yaml:"title" json:"title"`
	Description        string              `yaml:"description" json:"description"`
	Status             string              `yaml:"status" json:"status"`
	AcceptanceCriteria []AcceptanceCrit    `yaml:"acceptance_criteria" json:"acceptance_criteria"`
	Progress           Progress            `yaml:"progress" json:"progress"`
	Tests              []string            `yaml:"tests" json:"tests"`
	DependsOn          []string            `yaml:"depends_on" json:"depends_on"`
	ParentID           string              `yaml:"parent_id,omitempty" json:"parent_id,omitempty"`
	Subtasks           []string            `yaml:"subtasks" json:"subtasks"`
	CreatedAt          time.Time           `yaml:"created_at" json:"created_at"`
	UpdatedAt          time.Time           `yaml:"updated_at" json:"updated_at"`
	TaskmasterMetadata *TaskmasterMetadata `yaml:"taskmaster_metadata,omitempty" json:"taskmaster_metadata,omitempty"`
}

// AcceptanceCrit represents an acceptance criterion
type AcceptanceCrit struct {
	Text      string `yaml:"text" json:"text"`
	Completed bool   `yaml:"completed" json:"completed"`
}

// Progress represents task progress tracking
type Progress struct {
	Mode  string `yaml:"mode" json:"mode"`
	Value int    `yaml:"value" json:"value"`
}

// TaskmasterMetadata contains legacy taskmaster info
type TaskmasterMetadata struct {
	OriginalID string `yaml:"originalId" json:"original_id"`
}

// ConfigFile represents the config.yml structure
type ConfigFile struct {
	Project      ProjectConfig      `yaml:"project"`
	Workflow     WorkflowConfig     `yaml:"workflow"`
	Network      NetworkConfig      `yaml:"network"`
	Observability ObservabilityConfig `yaml:"observability"`
	Features     FeaturesConfig     `yaml:"features"`
}

// ProjectConfig contains project settings
type ProjectConfig struct {
	Name          string `yaml:"name"`
	GitRoot       string `yaml:"gitRoot"`
	ProjectRoot   string `yaml:"projectRoot"`
	DefaultBranch string `yaml:"defaultBranch"`
}

// WorkflowConfig contains workflow settings
type WorkflowConfig struct {
	BranchPrefix     string         `yaml:"branchPrefix"`
	AutoCreateBranch bool           `yaml:"autoCreateBranch"`
	AutoCreatePR     bool           `yaml:"autoCreatePr"`
	PackageManager   string         `yaml:"packageManager"`
	AgentTemplates   AgentTemplates `yaml:"agentTemplates"`
}

// AgentTemplates contains agent template configurations
type AgentTemplates struct {
	DefaultTemplateID string          `yaml:"defaultTemplateId"`
	Templates         []AgentTemplate `yaml:"templates"`
}

// AgentTemplate defines how to launch an agent
type AgentTemplate struct {
	ID                   string     `yaml:"id"`
	Name                 string     `yaml:"name"`
	Description          string     `yaml:"description"`
	Command              string     `yaml:"command"`
	Args                 []string   `yaml:"args,omitempty"`
	WorkingDirectory     string     `yaml:"workingDirectory,omitempty"`
	Env                  []EnvVar   `yaml:"env,omitempty"`
	MetadataMode         string     `yaml:"metadataMode,omitempty"`
	MetadataFilename     string     `yaml:"metadataFilename,omitempty"`
	RequiresConfirmation bool       `yaml:"requiresConfirmation"`
}

// EnvVar represents an environment variable
type EnvVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// NetworkConfig contains network allowlist
type NetworkConfig struct {
	Allowlist []string `yaml:"allowlist"`
}

// ObservabilityConfig contains logging settings
type ObservabilityConfig struct {
	LogLevel          string `yaml:"logLevel"`
	LogRotationMB     int    `yaml:"logRotationMb"`
	LogRetentionCount int    `yaml:"logRetentionCount"`
}

// FeaturesConfig contains feature flags
type FeaturesConfig struct {
	EnableFileScope bool `yaml:"enableFileScope"`
}

// TaskStatus constants
const (
	StatusTodo       = "todo"
	StatusInProgress = "in_progress"
	StatusReview     = "review"
	StatusDone       = "done"
	StatusBlocked    = "blocked"
)

// IsTerminal returns true if the status is a terminal state
func (t *Task) IsTerminal() bool {
	return t.Status == StatusDone
}

// IsActive returns true if the task is being worked on
func (t *Task) IsActive() bool {
	return t.Status == StatusInProgress || t.Status == StatusReview
}

// CompletedCriteriaCount returns how many acceptance criteria are done
func (t *Task) CompletedCriteriaCount() int {
	count := 0
	for _, ac := range t.AcceptanceCriteria {
		if ac.Completed {
			count++
		}
	}
	return count
}
