package intermute

import (
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/coldwine/storage"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

func TestMapColdwineTaskToIntermute(t *testing.T) {
	now := time.Now()
	coldwineTask := storage.WorkTask{
		ID:          "TASK-001",
		StoryID:     "STORY-001",
		Title:       "Implement feature X",
		Description: "Detailed description here",
		Status:      storage.TaskStatusInProgress,
		Priority:    2,
		Assignee:    "claude",
		SessionRef:  "session-123",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	intermuteTask := MapColdwineTaskToIntermute(coldwineTask, "autarch")

	if intermuteTask.ID != "" {
		t.Error("expected empty ID (Intermute assigns IDs)")
	}
	if intermuteTask.Project != "autarch" {
		t.Errorf("expected project 'autarch', got %s", intermuteTask.Project)
	}
	if intermuteTask.Title != "Implement feature X" {
		t.Errorf("expected title 'Implement feature X', got %s", intermuteTask.Title)
	}
	if intermuteTask.Agent != "claude" {
		t.Errorf("expected agent 'claude', got %s", intermuteTask.Agent)
	}
	if intermuteTask.SessionID != "session-123" {
		t.Errorf("expected session 'session-123', got %s", intermuteTask.SessionID)
	}
	if intermuteTask.Status != intermute.TaskStatusRunning {
		t.Errorf("expected status 'running', got %s", intermuteTask.Status)
	}
}

func TestMapStatusColdwineToIntermute(t *testing.T) {
	testCases := []struct {
		coldwine storage.TaskStatus
		expected intermute.TaskStatus
	}{
		{storage.TaskStatusTodo, intermute.TaskStatusPending},
		{storage.TaskStatusInProgress, intermute.TaskStatusRunning},
		{storage.TaskStatusBlocked, intermute.TaskStatusBlocked},
		{storage.TaskStatusDone, intermute.TaskStatusDone},
	}

	for _, tc := range testCases {
		t.Run(string(tc.coldwine), func(t *testing.T) {
			result := MapStatusColdwineToIntermute(tc.coldwine)
			if result != tc.expected {
				t.Errorf("MapStatusColdwineToIntermute(%s) = %s, want %s",
					tc.coldwine, result, tc.expected)
			}
		})
	}
}

func TestMapStatusIntermuteToColdwine(t *testing.T) {
	testCases := []struct {
		intermute intermute.TaskStatus
		expected  storage.TaskStatus
	}{
		{intermute.TaskStatusPending, storage.TaskStatusTodo},
		{intermute.TaskStatusRunning, storage.TaskStatusInProgress},
		{intermute.TaskStatusBlocked, storage.TaskStatusBlocked},
		{intermute.TaskStatusDone, storage.TaskStatusDone},
	}

	for _, tc := range testCases {
		t.Run(string(tc.intermute), func(t *testing.T) {
			result := MapStatusIntermuteToColdwine(tc.intermute)
			if result != tc.expected {
				t.Errorf("MapStatusIntermuteToColdwine(%s) = %s, want %s",
					tc.intermute, result, tc.expected)
			}
		})
	}
}

func TestMapIntermuteTaskToColdwine(t *testing.T) {
	now := time.Now()
	intermuteTask := intermute.Task{
		ID:        "int-task-001",
		Project:   "autarch",
		StoryID:   "STORY-002",
		Title:     "Fix bug Y",
		Agent:     "codex",
		SessionID: "session-456",
		Status:    intermute.TaskStatusBlocked,
		CreatedAt: now,
		UpdatedAt: now,
	}

	coldwineTask := MapIntermuteTaskToColdwine(intermuteTask)

	if coldwineTask.ID != "int-task-001" {
		t.Errorf("expected ID 'int-task-001', got %s", coldwineTask.ID)
	}
	if coldwineTask.StoryID != "STORY-002" {
		t.Errorf("expected story ID 'STORY-002', got %s", coldwineTask.StoryID)
	}
	if coldwineTask.Title != "Fix bug Y" {
		t.Errorf("expected title 'Fix bug Y', got %s", coldwineTask.Title)
	}
	if coldwineTask.Assignee != "codex" {
		t.Errorf("expected assignee 'codex', got %s", coldwineTask.Assignee)
	}
	if coldwineTask.SessionRef != "session-456" {
		t.Errorf("expected session ref 'session-456', got %s", coldwineTask.SessionRef)
	}
	if coldwineTask.Status != storage.TaskStatusBlocked {
		t.Errorf("expected status 'blocked', got %s", coldwineTask.Status)
	}
}

func TestMapColdwineTaskWithExternalID(t *testing.T) {
	coldwineTask := storage.WorkTask{
		ID:     "TASK-003",
		Title:  "Task with external ID",
		Status: storage.TaskStatusTodo,
	}

	// When we have an existing Intermute ID, use it
	intermuteTask := MapColdwineTaskToIntermuteWithID(coldwineTask, "autarch", "existing-int-id")

	if intermuteTask.ID != "existing-int-id" {
		t.Errorf("expected ID 'existing-int-id', got %s", intermuteTask.ID)
	}
}
