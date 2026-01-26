package events

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	ic "github.com/mistakeknot/intermute/client"
)

// mockIntermuteMessenger implements MessageSender for testing
type mockIntermuteMessenger struct {
	messages []ic.Message
	sendErr  error
}

func (m *mockIntermuteMessenger) SendMessage(ctx context.Context, msg ic.Message) (ic.SendResponse, error) {
	if m.sendErr != nil {
		return ic.SendResponse{}, m.sendErr
	}
	m.messages = append(m.messages, msg)
	return ic.SendResponse{MessageID: "test-msg-id", Cursor: 1}, nil
}

func TestIntermuteBridge_Forward(t *testing.T) {
	mock := &mockIntermuteMessenger{messages: make([]ic.Message, 0)}
	bridge := NewIntermuteBridge(mock, "autarch", "test-agent")

	evt := &Event{
		EventType:   EventTaskCompleted,
		EntityType:  EntityTask,
		EntityID:    "task-123",
		SourceTool:  SourceColdwine,
		Payload:     []byte(`{"task_id":"task-123"}`),
		ProjectPath: "/test/project",
		CreatedAt:   time.Now(),
	}

	err := bridge.Forward(context.Background(), evt)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.messages))
	}

	msg := mock.messages[0]
	if !strings.Contains(msg.Subject, "task_completed") {
		t.Errorf("expected subject to contain 'task_completed', got %s", msg.Subject)
	}
	if !strings.Contains(msg.Subject, "coldwine") {
		t.Errorf("expected subject to contain 'coldwine', got %s", msg.Subject)
	}
	if msg.From != "test-agent" {
		t.Errorf("expected From 'test-agent', got %s", msg.From)
	}
	if msg.Importance != "normal" {
		t.Errorf("expected Importance 'normal', got %s", msg.Importance)
	}
}

func TestIntermuteBridge_ForwardWithRecipients(t *testing.T) {
	mock := &mockIntermuteMessenger{messages: make([]ic.Message, 0)}
	bridge := NewIntermuteBridge(mock, "autarch", "coldwine-agent").
		WithRecipients([]string{"bigend-agent", "gurgeh-agent"})

	evt := &Event{
		EventType:   EventTaskStarted,
		EntityType:  EntityTask,
		EntityID:    "task-456",
		SourceTool:  SourceColdwine,
		Payload:     []byte(`{}`),
		CreatedAt:   time.Now(),
	}

	err := bridge.Forward(context.Background(), evt)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	msg := mock.messages[0]
	if len(msg.To) != 2 {
		t.Errorf("expected 2 recipients, got %d", len(msg.To))
	}
	if msg.To[0] != "bigend-agent" || msg.To[1] != "gurgeh-agent" {
		t.Errorf("unexpected recipients: %v", msg.To)
	}
}

func TestIntermuteBridge_HighImportanceForErrorEvents(t *testing.T) {
	testCases := []struct {
		eventType         EventType
		expectedImportance string
	}{
		{EventTaskBlocked, "high"},
		{EventRunFailed, "high"},
		{EventTaskCompleted, "normal"},
		{EventTaskCreated, "normal"},
		{EventRunStarted, "normal"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.eventType), func(t *testing.T) {
			mock := &mockIntermuteMessenger{messages: make([]ic.Message, 0)}
			bridge := NewIntermuteBridge(mock, "autarch", "test-agent")

			evt := &Event{
				EventType:  tc.eventType,
				EntityType: EntityTask,
				EntityID:   "test-id",
				SourceTool: SourceColdwine,
				Payload:    []byte(`{}`),
				CreatedAt:  time.Now(),
			}

			err := bridge.Forward(context.Background(), evt)
			if err != nil {
				t.Fatalf("Forward failed: %v", err)
			}

			msg := mock.messages[0]
			if msg.Importance != tc.expectedImportance {
				t.Errorf("for %s: expected importance %s, got %s",
					tc.eventType, tc.expectedImportance, msg.Importance)
			}
		})
	}
}

func TestIntermuteBridge_MessageBodyContainsEventData(t *testing.T) {
	mock := &mockIntermuteMessenger{messages: make([]ic.Message, 0)}
	bridge := NewIntermuteBridge(mock, "autarch", "test-agent")

	evt := &Event{
		ID:          42,
		EventType:   EventTaskCreated,
		EntityType:  EntityTask,
		EntityID:    "task-999",
		SourceTool:  SourceColdwine,
		Payload:     []byte(`{"title":"Test Task","assignee":"claude"}`),
		ProjectPath: "/my/project",
		CreatedAt:   time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	err := bridge.Forward(context.Background(), evt)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	// Verify body is valid JSON containing event data
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(mock.messages[0].Body), &body); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}

	if body["event_type"] != "task_created" {
		t.Errorf("expected event_type 'task_created', got %v", body["event_type"])
	}
	if body["entity_id"] != "task-999" {
		t.Errorf("expected entity_id 'task-999', got %v", body["entity_id"])
	}
	if body["source_tool"] != "coldwine" {
		t.Errorf("expected source_tool 'coldwine', got %v", body["source_tool"])
	}
}

func TestIntermuteBridge_ForwardNilEvent(t *testing.T) {
	mock := &mockIntermuteMessenger{messages: make([]ic.Message, 0)}
	bridge := NewIntermuteBridge(mock, "autarch", "test-agent")

	err := bridge.Forward(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil event")
	}
}
