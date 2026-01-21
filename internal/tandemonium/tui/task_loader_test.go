package tui

import (
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestLoadTasksReturnsRows(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := storage.InsertTask(db, storage.Task{ID: "T1", Title: "One", Status: "todo"}); err != nil {
		t.Fatal(err)
	}
	list, err := LoadTasks(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "T1" {
		t.Fatalf("expected task list with T1")
	}
	if list[0].SessionState != "" {
		t.Fatalf("expected empty session state")
	}
}

func TestLoadTasksIncludesSessionState(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := storage.InsertTask(db, storage.Task{ID: "T1", Title: "One", Status: "todo"}); err != nil {
		t.Fatal(err)
	}
	if err := storage.InsertSession(db, storage.Session{ID: "s1", TaskID: "T1", State: "working", Offset: 0}); err != nil {
		t.Fatal(err)
	}
	list, err := LoadTasks(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].SessionState != "working" {
		t.Fatalf("expected session state working")
	}
}
