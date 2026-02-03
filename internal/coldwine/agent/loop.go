package agent

import (
	"context"
	"log"

	"github.com/mistakeknot/autarch/internal/coldwine/intermute"
	"github.com/mistakeknot/autarch/internal/coldwine/storage"
)

type StatusStore interface {
	UpdateSessionState(id, state string) error
	UpdateTaskStatus(id, status string) error
	EnqueueReview(id string) error
	ApplyDetectionAtomic(taskID, sessionID, state string) error
}

func ApplyDetection(store StatusStore, taskID, sessionID, state string) error {
	if err := store.ApplyDetectionAtomic(taskID, sessionID, state); err != nil {
		return err
	}
	if state == "done" || state == "blocked" {
		if state == "done" {
			return store.EnqueueReview(taskID)
		}
		return nil
	}
	return nil
}

// NotifyBlocked broadcasts a task.blocked event via Intermute.
// It is best-effort: errors are logged but not returned.
func NotifyBlocked(broadcaster *intermute.TaskBroadcaster, taskID, reason, specID string) {
	if broadcaster == nil {
		return
	}
	task := storage.WorkTask{ID: taskID}
	if err := broadcaster.BroadcastBlocked(context.Background(), task, reason, specID); err != nil {
		log.Printf("warning: failed to broadcast blocked event for %s: %v", taskID, err)
	}
}
