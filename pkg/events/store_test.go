package events

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/pkg/contract"
)

func TestStoreOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	if store.Path() != path {
		t.Errorf("expected path %s, got %s", path, store.Path())
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestStoreAppendAndQuery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Append an event
	event := &Event{
		EventType:   EventTaskCreated,
		EntityType:  EntityTask,
		EntityID:    "TASK-001",
		SourceTool:  SourceColdwine,
		Payload:     []byte(`{"title":"Test task"}`),
		ProjectPath: "/test/project",
		CreatedAt:   time.Now(),
	}

	if err := store.Append(event); err != nil {
		t.Fatalf("failed to append event: %v", err)
	}

	if event.ID == 0 {
		t.Error("expected event ID to be set after append")
	}

	// Query all events
	events, err := store.Query(nil)
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}

	if events[0].EntityID != "TASK-001" {
		t.Errorf("expected entity ID TASK-001, got %s", events[0].EntityID)
	}
}

func TestStoreQueryWithFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Add multiple events
	events := []*Event{
		{EventType: EventTaskCreated, EntityType: EntityTask, EntityID: "TASK-001", SourceTool: SourceColdwine, Payload: []byte(`{}`), CreatedAt: time.Now()},
		{EventType: EventTaskStarted, EntityType: EntityTask, EntityID: "TASK-001", SourceTool: SourceColdwine, Payload: []byte(`{}`), CreatedAt: time.Now()},
		{EventType: EventRunStarted, EntityType: EntityRun, EntityID: "RUN-001", SourceTool: SourceColdwine, Payload: []byte(`{}`), CreatedAt: time.Now()},
		{EventType: EventInitiativeCreated, EntityType: EntityInitiative, EntityID: "INIT-001", SourceTool: SourceGurgeh, Payload: []byte(`{}`), CreatedAt: time.Now()},
	}

	for _, e := range events {
		if err := store.Append(e); err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	// Filter by event type
	result, err := store.Query(NewEventFilter().WithEventTypes(EventTaskCreated))
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 TaskCreated event, got %d", len(result))
	}

	// Filter by entity type
	result, err = store.Query(NewEventFilter().WithEntityTypes(EntityTask))
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 Task events, got %d", len(result))
	}

	// Filter by source tool
	result, err = store.Query(NewEventFilter().WithSourceTools(SourceGurgeh))
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 Gurgeh event, got %d", len(result))
	}
}

func TestWriterEmitEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	writer := NewWriter(store, SourceColdwine)
	writer.SetProjectPath("/test/project")

	// Emit a task created event
	task := &contract.Task{
		ID:         "TASK-001",
		StoryID:    "STORY-001",
		Title:      "Test task",
		Status:     contract.TaskStatusTodo,
		SourceTool: contract.SourceColdwine,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := writer.EmitTaskCreated(task); err != nil {
		t.Fatalf("failed to emit task created: %v", err)
	}

	// Emit task started
	if err := writer.EmitTaskStarted("TASK-001"); err != nil {
		t.Fatalf("failed to emit task started: %v", err)
	}

	// Query and verify
	events, err := store.Query(nil)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	// Verify project path was set
	for _, e := range events {
		if e.ProjectPath != "/test/project" {
			t.Errorf("expected project path /test/project, got %s", e.ProjectPath)
		}
	}
}

func TestReaderBuildState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	writer := NewWriter(store, SourceColdwine)

	// Emit some events
	task := &contract.Task{
		ID:         "TASK-001",
		StoryID:    "STORY-001",
		Title:      "Build feature",
		Status:     contract.TaskStatusTodo,
		SourceTool: contract.SourceColdwine,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	writer.EmitTaskCreated(task)
	writer.EmitTaskAssigned("TASK-001", "claude")
	writer.EmitTaskStarted("TASK-001")

	// Build state
	reader := NewReader(store)
	state, err := reader.BuildState(nil)
	if err != nil {
		t.Fatalf("failed to build state: %v", err)
	}

	// Verify task state
	taskState, ok := state.Tasks["TASK-001"]
	if !ok {
		t.Fatal("expected task TASK-001 in state")
	}

	if taskState.Status != "in_progress" {
		t.Errorf("expected status in_progress, got %s", taskState.Status)
	}

	if taskState.Assignee != "claude" {
		t.Errorf("expected assignee claude, got %s", taskState.Assignee)
	}
}

func TestStoreReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Add events
	for i := 1; i <= 5; i++ {
		e := &Event{
			EventType:  EventTaskCreated,
			EntityType: EntityTask,
			EntityID:   "TASK-001",
			SourceTool: SourceColdwine,
			Payload:    []byte(`{}`),
			CreatedAt:  time.Now(),
		}
		store.Append(e)
	}

	// Replay from ID 2
	count := 0
	err = store.Replay(2, nil, func(e *Event) error {
		count++
		if e.ID <= 2 {
			t.Errorf("expected ID > 2, got %d", e.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 events replayed, got %d", count)
	}
}
