package tui

import (
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestNewModelWithDBUsesDBLoaders(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := storage.InsertTask(db, storage.Task{ID: "TAND-DB-1", Title: "From DB", Status: "todo"}); err != nil {
		t.Fatal(err)
	}
	if err := storage.AddToReviewQueue(db, "TAND-DB-1"); err != nil {
		t.Fatal(err)
	}

	m := NewModelWithDB(db)
	m.RefreshTasks()
	if len(m.TaskList) == 0 || m.TaskList[0].ID != "TAND-DB-1" {
		t.Fatalf("expected TaskList from DB")
	}

	m.RefreshReviewQueue()
	if len(m.ReviewQueue) == 0 || m.ReviewQueue[0] != "TAND-DB-1" {
		t.Fatalf("expected ReviewQueue from DB")
	}
}
