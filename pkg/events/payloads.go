// Package events provides typed payload structures for all event types.
// These structs ensure type safety when emitting and consuming events.
package events

import (
	"time"

	"github.com/mistakeknot/autarch/pkg/contract"
)

// InitiativeCreatedPayload contains data for initiative_created events
type InitiativeCreatedPayload struct {
	Initiative contract.Initiative `json:"initiative"`
}

// InitiativeUpdatedPayload contains data for initiative_updated events
type InitiativeUpdatedPayload struct {
	Initiative contract.Initiative `json:"initiative"`
}

// InitiativeClosedPayload contains data for initiative_closed events
type InitiativeClosedPayload struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// EpicCreatedPayload contains data for epic_created events
type EpicCreatedPayload struct {
	Epic contract.Epic `json:"epic"`
}

// EpicUpdatedPayload contains data for epic_updated events
type EpicUpdatedPayload struct {
	Epic contract.Epic `json:"epic"`
}

// EpicClosedPayload contains data for epic_closed events
type EpicClosedPayload struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// StoryCreatedPayload contains data for story_created events
type StoryCreatedPayload struct {
	Story contract.Story `json:"story"`
}

// StoryUpdatedPayload contains data for story_updated events
type StoryUpdatedPayload struct {
	Story contract.Story `json:"story"`
}

// StoryClosedPayload contains data for story_closed events
type StoryClosedPayload struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// TaskCreatedPayload contains data for task_created events
type TaskCreatedPayload struct {
	Task contract.Task `json:"task"`
}

// TaskAssignedPayload contains data for task_assigned events
type TaskAssignedPayload struct {
	TaskID   string `json:"task_id"`
	Assignee string `json:"assignee"`
}

// TaskStartedPayload contains data for task_started events
type TaskStartedPayload struct {
	TaskID    string    `json:"task_id"`
	StartedAt time.Time `json:"started_at,omitempty"`
}

// TaskBlockedPayload contains data for task_blocked events
type TaskBlockedPayload struct {
	TaskID    string `json:"task_id"`
	Reason    string `json:"reason"`
	BlockedBy string `json:"blocked_by,omitempty"` // Optional: ID of blocking task/issue
}

// TaskCompletedPayload contains data for task_completed events
type TaskCompletedPayload struct {
	TaskID      string    `json:"task_id"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// RunStartedPayload contains data for run_started events
type RunStartedPayload struct {
	Run contract.Run `json:"run"`
}

// RunWaitingPayload contains data for run_waiting events
type RunWaitingPayload struct {
	RunID  string `json:"run_id"`
	Reason string `json:"reason"`
}

// RunCompletedPayload contains data for run_completed events
type RunCompletedPayload struct {
	RunID       string    `json:"run_id"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// RunFailedPayload contains data for run_failed events
type RunFailedPayload struct {
	RunID  string `json:"run_id"`
	Reason string `json:"reason"`
}

// OutcomeRecordedPayload contains data for outcome_recorded events
type OutcomeRecordedPayload struct {
	Outcome contract.Outcome `json:"outcome"`
}

// RunArtifactAddedPayload contains data for run_artifact_added events.
type RunArtifactAddedPayload struct {
	Artifact contract.RunArtifact `json:"artifact"`
}

// SpecSnapshot captures essential spec metadata for reconciliation events.
type SpecSnapshot struct {
	ID        string    `json:"id"`
	Title     string    `json:"title,omitempty"`
	Status    string    `json:"status,omitempty"`
	Type      string    `json:"type,omitempty"`
	Version   int       `json:"version,omitempty"`
	Path      string    `json:"path,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// SpecRevisedPayload contains data for spec_revised events.
type SpecRevisedPayload struct {
	Spec SpecSnapshot `json:"spec"`
}

// Note: InsightLinkedPayload is already defined in writer.go
// It's kept there for backward compatibility since it's part of the public API.
