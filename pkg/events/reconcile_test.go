package events

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReconcileSpecsEmitsAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, ".gurgeh", "specs")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	specPath := filepath.Join(specDir, "PRD-001.yaml")
	specV1 := "id: \"PRD-001\"\n" +
		"title: \"Test Spec\"\n" +
		"status: \"draft\"\n" +
		"version: 1\n"
	if err := os.WriteFile(specPath, []byte(specV1), 0644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	store, err := OpenStore(filepath.Join(root, "events.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if _, err := ReconcileProject(root, store); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	events, err := store.Query((&EventFilter{}).WithEventTypes(EventSpecRevised))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 spec event, got %d", len(events))
	}

	if _, err := ReconcileProject(root, store); err != nil {
		t.Fatalf("reconcile second: %v", err)
	}
	events, err = store.Query((&EventFilter{}).WithEventTypes(EventSpecRevised))
	if err != nil {
		t.Fatalf("query second: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected idempotent spec events, got %d", len(events))
	}

	specV2 := "id: \"PRD-001\"\n" +
		"title: \"Test Spec\"\n" +
		"status: \"draft\"\n" +
		"version: 2\n"
	if err := os.WriteFile(specPath, []byte(specV2), 0644); err != nil {
		t.Fatalf("write spec v2: %v", err)
	}

	if _, err := ReconcileProject(root, store); err != nil {
		t.Fatalf("reconcile third: %v", err)
	}
	events, err = store.Query((&EventFilter{}).WithEventTypes(EventSpecRevised))
	if err != nil {
		t.Fatalf("query third: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 spec events after update, got %d", len(events))
	}
}

func TestReconcileTasksEmitsStatusTransitions(t *testing.T) {
	root := t.TempDir()
	tasksDir := filepath.Join(root, ".coldwine", "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	taskPath := filepath.Join(tasksDir, "TASK-001.yaml")
	taskPending := "id: \"TASK-001\"\n" +
		"title: \"Task One\"\n" +
		"status: \"pending\"\n"
	if err := os.WriteFile(taskPath, []byte(taskPending), 0644); err != nil {
		t.Fatalf("write task: %v", err)
	}

	store, err := OpenStore(filepath.Join(root, "events.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if _, err := ReconcileProject(root, store); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	created, err := store.Query((&EventFilter{}).WithEventTypes(EventTaskCreated))
	if err != nil {
		t.Fatalf("query created: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 task_created event, got %d", len(created))
	}

	inProgress := "id: \"TASK-001\"\n" +
		"title: \"Task One\"\n" +
		"status: \"in_progress\"\n"
	if err := os.WriteFile(taskPath, []byte(inProgress), 0644); err != nil {
		t.Fatalf("write task in_progress: %v", err)
	}

	if _, err := ReconcileProject(root, store); err != nil {
		t.Fatalf("reconcile second: %v", err)
	}

	started, err := store.Query((&EventFilter{}).WithEventTypes(EventTaskStarted))
	if err != nil {
		t.Fatalf("query started: %v", err)
	}
	if len(started) != 1 {
		t.Fatalf("expected 1 task_started event, got %d", len(started))
	}

	if _, err := ReconcileProject(root, store); err != nil {
		t.Fatalf("reconcile idempotent: %v", err)
	}
	started, err = store.Query((&EventFilter{}).WithEventTypes(EventTaskStarted))
	if err != nil {
		t.Fatalf("query started again: %v", err)
	}
	if len(started) != 1 {
		t.Fatalf("expected idempotent task_started events, got %d", len(started))
	}
}
