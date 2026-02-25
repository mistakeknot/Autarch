package views

import (
	"testing"

	"github.com/mistakeknot/autarch/pkg/autarch"
	"github.com/mistakeknot/autarch/pkg/intercore"
)

func TestTaskMatchesDispatch_ByDispatchID(t *testing.T) {
	task := autarch.Task{ID: "task-1", Title: "Fix the bug", Status: autarch.TaskStatusRunning}
	d := intercore.Dispatch{ID: "d-abc", Status: "completed"}

	dispatches := map[string]string{"task-1": "d-abc"}

	if !taskMatchesDispatch(task, d, dispatches) {
		t.Error("should match via dispatch ID mapping")
	}
}

func TestTaskMatchesDispatch_ByName(t *testing.T) {
	task := autarch.Task{ID: "task-2", Title: "Fix the bug", Status: autarch.TaskStatusRunning}
	name := "Fix the bug"
	d := intercore.Dispatch{ID: "d-xyz", Name: &name, Status: "completed"}

	// No dispatch mapping — should fall back to name match.
	if !taskMatchesDispatch(task, d, nil) {
		t.Error("should match via dispatch Name field")
	}
}

func TestTaskMatchesDispatch_ByAgent(t *testing.T) {
	task := autarch.Task{ID: "task-3", Title: "Write tests", Status: autarch.TaskStatusRunning}
	d := intercore.Dispatch{ID: "d-999", Agent: "Write tests", Status: "completed"}

	// Legacy fallback via Agent field.
	if !taskMatchesDispatch(task, d, nil) {
		t.Error("should match via legacy Agent field")
	}
}

func TestTaskMatchesDispatch_NoMatch(t *testing.T) {
	task := autarch.Task{ID: "task-4", Title: "My task", Status: autarch.TaskStatusRunning}
	d := intercore.Dispatch{ID: "d-000", Agent: "other task", Status: "completed"}

	dispatches := map[string]string{"task-4": "d-different"}

	if taskMatchesDispatch(task, d, dispatches) {
		t.Error("should NOT match — dispatch ID and name differ")
	}
}

func TestTaskMatchesDispatch_DispatchIDTakesPrecedence(t *testing.T) {
	// Dispatch name matches task title, but dispatch ID mapping says different dispatch.
	// The ID mapping is authoritative — name matching must NOT be used as fallback.
	task := autarch.Task{ID: "task-5", Title: "Same title", Status: autarch.TaskStatusRunning}
	name := "Same title"
	d := intercore.Dispatch{ID: "d-wrong", Name: &name, Status: "completed"}

	// The dispatch mapping says task-5 belongs to d-correct, not d-wrong.
	dispatches := map[string]string{"task-5": "d-correct"}

	if taskMatchesDispatch(task, d, dispatches) {
		t.Error("should NOT match — dispatch ID mapping is authoritative and says d-correct, not d-wrong")
	}
}

func TestTaskMatchesDispatch_NilDispatches(t *testing.T) {
	task := autarch.Task{ID: "task-6", Title: "Title", Status: autarch.TaskStatusRunning}
	name := "Title"
	d := intercore.Dispatch{ID: "d-1", Name: &name, Status: "completed"}

	// Nil map should not panic.
	if !taskMatchesDispatch(task, d, nil) {
		t.Error("should match via name when dispatches map is nil")
	}
}
