package intermute

import (
	"context"
	"testing"
	"time"

	ic "github.com/mistakeknot/intermute/client"
	"github.com/mistakeknot/autarch/internal/coldwine/storage"
)

// mockMessenger implements events.MessageSender for testing
type mockMessenger struct {
	messages []mockMessage
	sendErr  error
}

type mockMessage struct {
	subject string
	body    string
}

func (m *mockMessenger) SendMessage(ctx context.Context, msg ic.Message) (ic.SendResponse, error) {
	if m.sendErr != nil {
		return ic.SendResponse{}, m.sendErr
	}
	m.messages = append(m.messages, mockMessage{subject: msg.Subject, body: msg.Body})
	return ic.SendResponse{MessageID: "test-id", Cursor: 1}, nil
}

func TestTaskBroadcaster_BroadcastStatusChange(t *testing.T) {
	mock := &mockMessenger{messages: make([]mockMessage, 0)}
	broadcaster := NewTaskBroadcaster(mock, "autarch", "coldwine-agent")

	task := storage.WorkTask{
		ID:       "TASK-001",
		Title:    "Test Task",
		Status:   storage.TaskStatusTodo,
		StoryID:  "STORY-001",
		Assignee: "claude",
	}

	err := broadcaster.BroadcastStatusChange(context.Background(), task, storage.TaskStatusInProgress)
	if err != nil {
		t.Fatalf("BroadcastStatusChange failed: %v", err)
	}

	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.messages))
	}

	msg := mock.messages[0]
	if msg.subject == "" {
		t.Error("expected non-empty subject")
	}
}

func TestTaskBroadcaster_BroadcastCreated(t *testing.T) {
	mock := &mockMessenger{messages: make([]mockMessage, 0)}
	broadcaster := NewTaskBroadcaster(mock, "autarch", "coldwine-agent")

	task := storage.WorkTask{
		ID:        "TASK-002",
		Title:     "New Task",
		Status:    storage.TaskStatusTodo,
		StoryID:   "STORY-001",
		CreatedAt: time.Now(),
	}

	err := broadcaster.BroadcastCreated(context.Background(), task)
	if err != nil {
		t.Fatalf("BroadcastCreated failed: %v", err)
	}

	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.messages))
	}
}

func TestTaskBroadcaster_BroadcastAssigned(t *testing.T) {
	mock := &mockMessenger{messages: make([]mockMessage, 0)}
	broadcaster := NewTaskBroadcaster(mock, "autarch", "coldwine-agent")

	task := storage.WorkTask{
		ID:       "TASK-003",
		Title:    "Assigned Task",
		Status:   storage.TaskStatusTodo,
		Assignee: "claude",
	}

	err := broadcaster.BroadcastAssigned(context.Background(), task, "claude")
	if err != nil {
		t.Fatalf("BroadcastAssigned failed: %v", err)
	}

	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.messages))
	}
}

func TestTaskBroadcaster_WithRecipients(t *testing.T) {
	mock := &mockMessenger{messages: make([]mockMessage, 0)}
	broadcaster := NewTaskBroadcaster(mock, "autarch", "coldwine-agent").
		WithRecipients([]string{"bigend-agent"})

	task := storage.WorkTask{
		ID:     "TASK-004",
		Title:  "Task with recipients",
		Status: storage.TaskStatusDone,
	}

	err := broadcaster.BroadcastStatusChange(context.Background(), task, storage.TaskStatusDone)
	if err != nil {
		t.Fatalf("BroadcastStatusChange failed: %v", err)
	}

	// Message was sent (recipients handling is in the bridge)
	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.messages))
	}
}

func TestTaskBroadcaster_NilMessengerGracefulDegradation(t *testing.T) {
	broadcaster := NewTaskBroadcaster(nil, "autarch", "coldwine-agent")

	task := storage.WorkTask{
		ID:     "TASK-005",
		Title:  "Task with nil messenger",
		Status: storage.TaskStatusTodo,
	}

	// Should not panic or error when messenger is nil
	err := broadcaster.BroadcastCreated(context.Background(), task)
	if err != nil {
		t.Fatalf("expected nil error for nil messenger, got: %v", err)
	}
}
