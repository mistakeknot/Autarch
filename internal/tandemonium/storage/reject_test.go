package storage

import "testing"

func TestRejectTask(t *testing.T) {
	db, _ := OpenTemp()
	_ = Migrate(db)
	_ = InsertTask(db, Task{ID: "T1", Title: "Test", Status: "review"})
	_ = AddToReviewQueue(db, "T1")
	if err := RejectTask(db, "T1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ids, _ := ListReviewQueue(db)
	if len(ids) != 0 {
		t.Fatal("expected review queue cleared")
	}
}
