// Package intermute provides Intermute integration for Coldwine task orchestration.
// It enables task lifecycle events to be broadcast to other Autarch tools via Intermute messaging.
package intermute

import (
	"context"
	"encoding/json"
	"fmt"

	ic "github.com/mistakeknot/intermute/client"
	"github.com/mistakeknot/autarch/internal/coldwine/storage"
)

// MessageSender defines the interface for sending messages to Intermute.
type MessageSender interface {
	SendMessage(ctx context.Context, msg ic.Message) (ic.SendResponse, error)
}

// TaskBroadcaster broadcasts Coldwine task events to Intermute.
// It sends structured messages when tasks are created, assigned, or change status.
type TaskBroadcaster struct {
	sender     MessageSender
	project    string
	agentID    string
	recipients []string
}

// NewTaskBroadcaster creates a new broadcaster for task events.
// If sender is nil, broadcast operations become no-ops (graceful degradation).
func NewTaskBroadcaster(sender MessageSender, project, agentID string) *TaskBroadcaster {
	return &TaskBroadcaster{
		sender:  sender,
		project: project,
		agentID: agentID,
	}
}

// WithRecipients sets specific recipients for broadcast messages.
func (b *TaskBroadcaster) WithRecipients(recipients []string) *TaskBroadcaster {
	b.recipients = recipients
	return b
}

// TaskEventPayload is the structured payload for task events
type TaskEventPayload struct {
	EventType    string `json:"event_type"`
	TaskID       string `json:"task_id"`
	StoryID      string `json:"story_id,omitempty"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	PreviousStatus string `json:"previous_status,omitempty"`
	Assignee     string `json:"assignee,omitempty"`
	Priority     int    `json:"priority,omitempty"`
}

// BroadcastCreated sends a task.created event
func (b *TaskBroadcaster) BroadcastCreated(ctx context.Context, task storage.WorkTask) error {
	if b.sender == nil {
		return nil
	}

	payload := TaskEventPayload{
		EventType: "task.created",
		TaskID:    task.ID,
		StoryID:   task.StoryID,
		Title:     task.Title,
		Status:    string(task.Status),
		Assignee:  task.Assignee,
		Priority:  task.Priority,
	}

	return b.send(ctx, "task.created", payload)
}

// BroadcastStatusChange sends a task.status_changed event
func (b *TaskBroadcaster) BroadcastStatusChange(ctx context.Context, task storage.WorkTask, newStatus storage.TaskStatus) error {
	if b.sender == nil {
		return nil
	}

	payload := TaskEventPayload{
		EventType:      "task.status_changed",
		TaskID:         task.ID,
		StoryID:        task.StoryID,
		Title:          task.Title,
		Status:         string(newStatus),
		PreviousStatus: string(task.Status),
		Assignee:       task.Assignee,
		Priority:       task.Priority,
	}

	return b.send(ctx, "task.status_changed", payload)
}

// BroadcastAssigned sends a task.assigned event
func (b *TaskBroadcaster) BroadcastAssigned(ctx context.Context, task storage.WorkTask, assignee string) error {
	if b.sender == nil {
		return nil
	}

	payload := TaskEventPayload{
		EventType: "task.assigned",
		TaskID:    task.ID,
		StoryID:   task.StoryID,
		Title:     task.Title,
		Status:    string(task.Status),
		Assignee:  assignee,
		Priority:  task.Priority,
	}

	return b.send(ctx, "task.assigned", payload)
}

// BroadcastBlocked sends a task.blocked event with reason
func (b *TaskBroadcaster) BroadcastBlocked(ctx context.Context, task storage.WorkTask, reason string) error {
	if b.sender == nil {
		return nil
	}

	type BlockedPayload struct {
		TaskEventPayload
		Reason string `json:"reason"`
	}

	payload := BlockedPayload{
		TaskEventPayload: TaskEventPayload{
			EventType: "task.blocked",
			TaskID:    task.ID,
			StoryID:   task.StoryID,
			Title:     task.Title,
			Status:    "blocked",
			Assignee:  task.Assignee,
			Priority:  task.Priority,
		},
		Reason: reason,
	}

	return b.send(ctx, "task.blocked", payload)
}

// BroadcastCompleted sends a task.completed event
func (b *TaskBroadcaster) BroadcastCompleted(ctx context.Context, task storage.WorkTask) error {
	if b.sender == nil {
		return nil
	}

	payload := TaskEventPayload{
		EventType:      "task.completed",
		TaskID:         task.ID,
		StoryID:        task.StoryID,
		Title:          task.Title,
		Status:         "done",
		PreviousStatus: string(task.Status),
		Assignee:       task.Assignee,
		Priority:       task.Priority,
	}

	return b.send(ctx, "task.completed", payload)
}

func (b *TaskBroadcaster) send(ctx context.Context, eventType string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	msg := ic.Message{
		From:       b.agentID,
		To:         b.recipients,
		Subject:    fmt.Sprintf("[coldwine] %s", eventType),
		Body:       string(body),
		Importance: eventImportance(eventType),
	}

	if b.project != "" {
		msg.Project = b.project
	}

	_, err = b.sender.SendMessage(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to send %s event: %w", eventType, err)
	}

	return nil
}

// eventImportance returns "high" for blocked events, "normal" otherwise
func eventImportance(eventType string) string {
	if eventType == "task.blocked" {
		return "high"
	}
	return "normal"
}
