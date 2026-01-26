package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	ic "github.com/mistakeknot/intermute/client"
)

// MessageSender defines the interface for sending messages to Intermute.
// This allows for easy mocking in tests.
type MessageSender interface {
	SendMessage(ctx context.Context, msg ic.Message) (ic.SendResponse, error)
}

// IntermuteBridge forwards local events to Intermute messaging.
// This enables cross-tool visibility where Autarch tools can broadcast
// their events to other tools via Intermute's coordination layer.
type IntermuteBridge struct {
	sender     MessageSender
	project    string
	agentID    string
	recipients []string
}

// NewIntermuteBridge creates a new bridge that forwards events to Intermute.
// The sender can be an *intermute.Client or any implementation of MessageSender.
// The project identifies the Intermute project scope.
// The agentID identifies the sending agent in message metadata.
func NewIntermuteBridge(sender MessageSender, project, agentID string) *IntermuteBridge {
	return &IntermuteBridge{
		sender:  sender,
		project: project,
		agentID: agentID,
	}
}

// WithRecipients sets specific recipients for forwarded messages.
// If not set, messages are broadcast (empty To field allows Intermute to handle routing).
func (b *IntermuteBridge) WithRecipients(recipients []string) *IntermuteBridge {
	b.recipients = recipients
	return b
}

// bridgeEventPayload is the JSON structure sent in message bodies
type bridgeEventPayload struct {
	EventID     int64      `json:"event_id,omitempty"`
	EventType   EventType  `json:"event_type"`
	EntityType  EntityType `json:"entity_type"`
	EntityID    string     `json:"entity_id"`
	SourceTool  SourceTool `json:"source_tool"`
	Payload     any        `json:"payload,omitempty"`
	ProjectPath string     `json:"project_path,omitempty"`
	CreatedAt   string     `json:"created_at"`
}

// Forward sends an event to Intermute as a message.
// The event is serialized to JSON and sent with appropriate metadata.
// Returns an error if the event is nil or if sending fails.
func (b *IntermuteBridge) Forward(ctx context.Context, evt *Event) error {
	if evt == nil {
		return fmt.Errorf("cannot forward nil event")
	}

	// Parse the event payload if present
	var payloadData any
	if len(evt.Payload) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(evt.Payload, &parsed); err == nil {
			payloadData = parsed
		} else {
			// If not valid JSON, include as string
			payloadData = string(evt.Payload)
		}
	}

	// Build the message body
	body := bridgeEventPayload{
		EventID:     evt.ID,
		EventType:   evt.EventType,
		EntityType:  evt.EntityType,
		EntityID:    evt.EntityID,
		SourceTool:  evt.SourceTool,
		Payload:     payloadData,
		ProjectPath: evt.ProjectPath,
		CreatedAt:   evt.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal event body: %w", err)
	}

	// Build the message
	msg := ic.Message{
		From:       b.agentID,
		To:         b.recipients,
		Subject:    fmt.Sprintf("[%s] %s", evt.SourceTool, evt.EventType),
		Body:       string(bodyJSON),
		Importance: eventImportance(evt.EventType),
	}

	// Set project if configured
	if b.project != "" {
		msg.Project = b.project
	}

	_, err = b.sender.SendMessage(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to send event to Intermute: %w", err)
	}

	return nil
}

// eventImportance determines message importance based on event type.
// Error, failure, and blocked events are high importance.
// All other events are normal importance.
func eventImportance(eventType EventType) string {
	et := string(eventType)
	if strings.Contains(et, "failed") ||
		strings.Contains(et, "blocked") ||
		strings.Contains(et, "error") {
		return "high"
	}
	return "normal"
}
