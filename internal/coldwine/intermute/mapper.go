package intermute

import (
	"github.com/mistakeknot/autarch/internal/coldwine/storage"
	"github.com/mistakeknot/autarch/pkg/intermute"
)

// MapStatusColdwineToIntermute converts Coldwine task status to Intermute task status.
// Mapping:
//   - todo        -> pending
//   - in_progress -> running
//   - blocked     -> blocked
//   - done        -> done
func MapStatusColdwineToIntermute(status storage.TaskStatus) intermute.TaskStatus {
	switch status {
	case storage.TaskStatusTodo:
		return intermute.TaskStatusPending
	case storage.TaskStatusInProgress:
		return intermute.TaskStatusRunning
	case storage.TaskStatusBlocked:
		return intermute.TaskStatusBlocked
	case storage.TaskStatusDone:
		return intermute.TaskStatusDone
	default:
		return intermute.TaskStatusPending
	}
}

// MapStatusIntermuteToColdwine converts Intermute task status to Coldwine task status.
// Mapping:
//   - pending -> todo
//   - running -> in_progress
//   - blocked -> blocked
//   - done    -> done
func MapStatusIntermuteToColdwine(status intermute.TaskStatus) storage.TaskStatus {
	switch status {
	case intermute.TaskStatusPending:
		return storage.TaskStatusTodo
	case intermute.TaskStatusRunning:
		return storage.TaskStatusInProgress
	case intermute.TaskStatusBlocked:
		return storage.TaskStatusBlocked
	case intermute.TaskStatusDone:
		return storage.TaskStatusDone
	default:
		return storage.TaskStatusTodo
	}
}

// MapColdwineTaskToIntermute converts a Coldwine WorkTask to an Intermute Task.
// The returned task has an empty ID since Intermute assigns IDs on creation.
// Use MapColdwineTaskToIntermuteWithID when updating an existing Intermute task.
func MapColdwineTaskToIntermute(task storage.WorkTask, project string) intermute.Task {
	return intermute.Task{
		// ID is empty - Intermute will assign one on creation
		Project:   project,
		StoryID:   task.StoryID,
		Title:     task.Title,
		Agent:     task.Assignee,
		SessionID: task.SessionRef,
		Status:    MapStatusColdwineToIntermute(task.Status),
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
	}
}

// MapColdwineTaskToIntermuteWithID converts a Coldwine WorkTask to an Intermute Task
// with a specific Intermute ID. Use this when updating an existing Intermute task.
func MapColdwineTaskToIntermuteWithID(task storage.WorkTask, project, intermuteID string) intermute.Task {
	t := MapColdwineTaskToIntermute(task, project)
	t.ID = intermuteID
	return t
}

// MapIntermuteTaskToColdwine converts an Intermute Task to a Coldwine WorkTask.
// Note: Some Coldwine-specific fields (Description, Priority, WorktreeRef) are not
// available in Intermute and will be zero-valued.
func MapIntermuteTaskToColdwine(task intermute.Task) storage.WorkTask {
	return storage.WorkTask{
		ID:         task.ID,
		StoryID:    task.StoryID,
		Title:      task.Title,
		Assignee:   task.Agent,
		SessionRef: task.SessionID,
		Status:     MapStatusIntermuteToColdwine(task.Status),
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
		// Fields not available from Intermute:
		// Description, Priority, WorktreeRef
	}
}
