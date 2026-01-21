package agent

type StatusStore interface {
	UpdateSessionState(id, state string) error
	UpdateTaskStatus(id, status string) error
	EnqueueReview(id string) error
}

func ApplyDetection(store StatusStore, taskID, sessionID, state string) error {
	if err := store.UpdateSessionState(sessionID, state); err != nil {
		return err
	}
	if state == "done" || state == "blocked" {
		if err := store.UpdateTaskStatus(taskID, state); err != nil {
			return err
		}
		if state == "done" {
			return store.EnqueueReview(taskID)
		}
		return nil
	}
	return nil
}
