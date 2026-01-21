package storage

import "testing"

func TestApproveTask(t *testing.T) {
	db, _ := OpenTemp()
	defer db.Close()
	_ = Migrate(db)
	_ = InsertTask(db, Task{ID: "TAND-001", Title: "Test", Status: "review"})
	_ = AddToReviewQueue(db, "TAND-001")
	_ = ApproveTask(db, "TAND-001")
	tsk, _ := GetTask(db, "TAND-001")
	if tsk.Status != "done" {
		t.Fatal("expected done")
	}
	ids, _ := ListReviewQueue(db)
	if len(ids) != 0 {
		t.Fatal("expected queue empty")
	}
}
