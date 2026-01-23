package epics

type Status string

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusReview     Status = "review"
	StatusBlocked    Status = "blocked"
	StatusDone       Status = "done"
)

type Priority string

const (
	PriorityP0 Priority = "p0"
	PriorityP1 Priority = "p1"
	PriorityP2 Priority = "p2"
	PriorityP3 Priority = "p3"
)

type Story struct {
	ID                 string   `yaml:"id"`
	Title              string   `yaml:"title"`
	Summary            string   `yaml:"summary,omitempty"`
	Status             Status   `yaml:"status"`
	Priority           Priority `yaml:"priority"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria,omitempty"`
	Risks              []string `yaml:"risks,omitempty"`
	Estimates          string   `yaml:"estimates,omitempty"`
}

type Epic struct {
	ID                 string   `yaml:"id"`
	Title              string   `yaml:"title"`
	Summary            string   `yaml:"summary,omitempty"`
	Status             Status   `yaml:"status"`
	Priority           Priority `yaml:"priority"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria,omitempty"`
	Risks              []string `yaml:"risks,omitempty"`
	Estimates          string   `yaml:"estimates,omitempty"`
	Stories            []Story  `yaml:"stories,omitempty"`
}

// ExistingMode controls how to handle pre-existing epic files.
type ExistingMode string

const (
	ExistingSkip      ExistingMode = "skip"
	ExistingOverwrite ExistingMode = "overwrite"
)

type WriteOptions struct {
	Existing ExistingMode
}
