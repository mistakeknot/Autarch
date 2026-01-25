package contract

import (
	"errors"
	"fmt"
	"strings"
)

// ValidationError represents a validation failure
type ValidationError struct {
	EntityType string
	EntityID   string
	Field      string
	Message    string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s %s: %s - %s", e.EntityType, e.EntityID, e.Field, e.Message)
}

// ValidationResult collects multiple validation errors
type ValidationResult struct {
	Errors []ValidationError
}

// IsValid returns true if there are no validation errors
func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// Add appends a validation error
func (r *ValidationResult) Add(entityType, entityID, field, message string) {
	r.Errors = append(r.Errors, ValidationError{
		EntityType: entityType,
		EntityID:   entityID,
		Field:      field,
		Message:    message,
	})
}

// Error returns a combined error message or nil if valid
func (r *ValidationResult) Error() error {
	if r.IsValid() {
		return nil
	}
	var msgs []string
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return errors.New(strings.Join(msgs, "; "))
}

// ValidateInitiative validates an Initiative entity
func ValidateInitiative(i *Initiative) *ValidationResult {
	result := &ValidationResult{}

	if strings.TrimSpace(i.ID) == "" {
		result.Add("Initiative", i.ID, "ID", "cannot be empty")
	}
	if strings.TrimSpace(i.Title) == "" {
		result.Add("Initiative", i.ID, "Title", "cannot be empty")
	}
	if !isValidStatus(i.Status) {
		result.Add("Initiative", i.ID, "Status", fmt.Sprintf("invalid status: %s", i.Status))
	}
	if !isValidSourceTool(i.SourceTool) {
		result.Add("Initiative", i.ID, "SourceTool", fmt.Sprintf("invalid source tool: %s", i.SourceTool))
	}
	if i.CreatedAt.IsZero() {
		result.Add("Initiative", i.ID, "CreatedAt", "cannot be zero")
	}

	return result
}

// ValidateEpic validates an Epic entity
func ValidateEpic(e *Epic) *ValidationResult {
	result := &ValidationResult{}

	if strings.TrimSpace(e.ID) == "" {
		result.Add("Epic", e.ID, "ID", "cannot be empty")
	}
	if strings.TrimSpace(e.Title) == "" {
		result.Add("Epic", e.ID, "Title", "cannot be empty")
	}
	if !isValidStatus(e.Status) {
		result.Add("Epic", e.ID, "Status", fmt.Sprintf("invalid status: %s", e.Status))
	}
	if !isValidSourceTool(e.SourceTool) {
		result.Add("Epic", e.ID, "SourceTool", fmt.Sprintf("invalid source tool: %s", e.SourceTool))
	}
	if e.CreatedAt.IsZero() {
		result.Add("Epic", e.ID, "CreatedAt", "cannot be zero")
	}

	return result
}

// ValidateStory validates a Story entity
func ValidateStory(s *Story) *ValidationResult {
	result := &ValidationResult{}

	if strings.TrimSpace(s.ID) == "" {
		result.Add("Story", s.ID, "ID", "cannot be empty")
	}
	if strings.TrimSpace(s.EpicID) == "" {
		result.Add("Story", s.ID, "EpicID", "cannot be empty")
	}
	if strings.TrimSpace(s.Title) == "" {
		result.Add("Story", s.ID, "Title", "cannot be empty")
	}
	if !isValidStatus(s.Status) {
		result.Add("Story", s.ID, "Status", fmt.Sprintf("invalid status: %s", s.Status))
	}
	if s.Complexity != "" && !isValidComplexity(s.Complexity) {
		result.Add("Story", s.ID, "Complexity", fmt.Sprintf("invalid complexity: %s", s.Complexity))
	}
	if !isValidSourceTool(s.SourceTool) {
		result.Add("Story", s.ID, "SourceTool", fmt.Sprintf("invalid source tool: %s", s.SourceTool))
	}
	if s.CreatedAt.IsZero() {
		result.Add("Story", s.ID, "CreatedAt", "cannot be zero")
	}

	return result
}

// ValidateTask validates a Task entity
func ValidateTask(t *Task) *ValidationResult {
	result := &ValidationResult{}

	if strings.TrimSpace(t.ID) == "" {
		result.Add("Task", t.ID, "ID", "cannot be empty")
	}
	if strings.TrimSpace(t.StoryID) == "" {
		result.Add("Task", t.ID, "StoryID", "cannot be empty")
	}
	if strings.TrimSpace(t.Title) == "" {
		result.Add("Task", t.ID, "Title", "cannot be empty")
	}
	if !isValidTaskStatus(t.Status) {
		result.Add("Task", t.ID, "Status", fmt.Sprintf("invalid task status: %s", t.Status))
	}
	if !isValidSourceTool(t.SourceTool) {
		result.Add("Task", t.ID, "SourceTool", fmt.Sprintf("invalid source tool: %s", t.SourceTool))
	}
	if t.CreatedAt.IsZero() {
		result.Add("Task", t.ID, "CreatedAt", "cannot be zero")
	}

	return result
}

// ValidateRun validates a Run entity
func ValidateRun(r *Run) *ValidationResult {
	result := &ValidationResult{}

	if strings.TrimSpace(r.ID) == "" {
		result.Add("Run", r.ID, "ID", "cannot be empty")
	}
	if strings.TrimSpace(r.TaskID) == "" {
		result.Add("Run", r.ID, "TaskID", "cannot be empty")
	}
	if strings.TrimSpace(r.AgentName) == "" {
		result.Add("Run", r.ID, "AgentName", "cannot be empty")
	}
	if strings.TrimSpace(r.AgentProgram) == "" {
		result.Add("Run", r.ID, "AgentProgram", "cannot be empty")
	}
	if !isValidRunState(r.State) {
		result.Add("Run", r.ID, "State", fmt.Sprintf("invalid run state: %s", r.State))
	}
	if !isValidSourceTool(r.SourceTool) {
		result.Add("Run", r.ID, "SourceTool", fmt.Sprintf("invalid source tool: %s", r.SourceTool))
	}
	if r.StartedAt.IsZero() {
		result.Add("Run", r.ID, "StartedAt", "cannot be zero")
	}

	return result
}

// ValidateOutcome validates an Outcome entity
func ValidateOutcome(o *Outcome) *ValidationResult {
	result := &ValidationResult{}

	if strings.TrimSpace(o.ID) == "" {
		result.Add("Outcome", o.ID, "ID", "cannot be empty")
	}
	if strings.TrimSpace(o.RunID) == "" {
		result.Add("Outcome", o.ID, "RunID", "cannot be empty")
	}
	if strings.TrimSpace(o.Summary) == "" {
		result.Add("Outcome", o.ID, "Summary", "cannot be empty")
	}
	if !isValidSourceTool(o.SourceTool) {
		result.Add("Outcome", o.ID, "SourceTool", fmt.Sprintf("invalid source tool: %s", o.SourceTool))
	}
	if o.CreatedAt.IsZero() {
		result.Add("Outcome", o.ID, "CreatedAt", "cannot be zero")
	}

	return result
}

// Helper validation functions
func isValidStatus(s Status) bool {
	switch s {
	case StatusDraft, StatusOpen, StatusInProgress, StatusDone, StatusClosed:
		return true
	}
	return false
}

func isValidTaskStatus(s TaskStatus) bool {
	switch s {
	case TaskStatusTodo, TaskStatusInProgress, TaskStatusBlocked, TaskStatusDone:
		return true
	}
	return false
}

func isValidRunState(s RunState) bool {
	switch s {
	case RunStateWorking, RunStateWaiting, RunStateBlocked, RunStateDone:
		return true
	}
	return false
}

func isValidComplexity(c Complexity) bool {
	switch c {
	case ComplexityXS, ComplexityS, ComplexityM, ComplexityL, ComplexityXL:
		return true
	}
	return false
}

func isValidSourceTool(s SourceTool) bool {
	switch s {
	case SourceGurgeh, SourceColdwine, SourcePollard, SourceBigend:
		return true
	}
	return false
}

// CrossToolValidator provides validation for cross-tool references
type CrossToolValidator struct {
	initiatives map[string]*Initiative
	epics       map[string]*Epic
	stories     map[string]*Story
	tasks       map[string]*Task
	runs        map[string]*Run
}

// NewCrossToolValidator creates a new validator with the given entities
func NewCrossToolValidator() *CrossToolValidator {
	return &CrossToolValidator{
		initiatives: make(map[string]*Initiative),
		epics:       make(map[string]*Epic),
		stories:     make(map[string]*Story),
		tasks:       make(map[string]*Task),
		runs:        make(map[string]*Run),
	}
}

// RegisterInitiative adds an initiative for reference validation
func (v *CrossToolValidator) RegisterInitiative(i *Initiative) {
	v.initiatives[i.ID] = i
}

// RegisterEpic adds an epic for reference validation
func (v *CrossToolValidator) RegisterEpic(e *Epic) {
	v.epics[e.ID] = e
}

// RegisterStory adds a story for reference validation
func (v *CrossToolValidator) RegisterStory(s *Story) {
	v.stories[s.ID] = s
}

// RegisterTask adds a task for reference validation
func (v *CrossToolValidator) RegisterTask(t *Task) {
	v.tasks[t.ID] = t
}

// RegisterRun adds a run for reference validation
func (v *CrossToolValidator) RegisterRun(r *Run) {
	v.runs[r.ID] = r
}

// ValidateReferences checks all cross-tool references
func (v *CrossToolValidator) ValidateReferences() *ValidationResult {
	result := &ValidationResult{}

	// Validate Epic -> Initiative references
	for _, e := range v.epics {
		if e.InitiativeID != "" {
			if _, ok := v.initiatives[e.InitiativeID]; !ok {
				result.Add("Epic", e.ID, "InitiativeID", fmt.Sprintf("references non-existent initiative: %s", e.InitiativeID))
			}
		}
	}

	// Validate Story -> Epic references
	for _, s := range v.stories {
		if _, ok := v.epics[s.EpicID]; !ok {
			result.Add("Story", s.ID, "EpicID", fmt.Sprintf("references non-existent epic: %s", s.EpicID))
		}
	}

	// Validate Task -> Story references
	for _, t := range v.tasks {
		if _, ok := v.stories[t.StoryID]; !ok {
			result.Add("Task", t.ID, "StoryID", fmt.Sprintf("references non-existent story: %s", t.StoryID))
		}
	}

	// Validate Run -> Task references
	for _, r := range v.runs {
		if _, ok := v.tasks[r.TaskID]; !ok {
			result.Add("Run", r.ID, "TaskID", fmt.Sprintf("references non-existent task: %s", r.TaskID))
		}
	}

	return result
}
