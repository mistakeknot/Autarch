package storage

import "testing"

func TestReviewQueueAdd(t *testing.T) {
    db, err := OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := InsertTask(db, Task{ID: "TAND-001", Title: "Test", Status: "review"}); err != nil {
		t.Fatal(err)
	}
	if err := AddToReviewQueue(db, "TAND-001"); err != nil {
		t.Fatal(err)
	}
    ids, err := ListReviewQueue(db)
    if err != nil {
        t.Fatal(err)
    }
    if len(ids) != 1 {
        t.Fatal("expected 1")
    }
}

func TestRejectTaskRequeues(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := InsertTask(db, Task{ID: "TAND-002", Title: "Test", Status: "review"}); err != nil {
		t.Fatal(err)
	}
	if err := AddToReviewQueue(db, "TAND-002"); err != nil {
		t.Fatal(err)
	}
	if err := RejectTask(db, "TAND-002"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	task, err := GetTask(db, "TAND-002")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "ready" {
		t.Fatalf("expected ready, got %q", task.Status)
	}
	queue, err := ListReviewQueue(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 0 {
		t.Fatalf("expected empty review queue")
	}
}

func TestRejectTaskTransactionRollsBack(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := InsertTask(db, Task{ID: "TAND-003", Title: "Test", Status: "review"}); err != nil {
		t.Fatal(err)
	}
	if err := AddToReviewQueue(db, "TAND-003"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE review_queue"); err != nil {
		t.Fatal(err)
	}
	if err := RejectTask(db, "TAND-003"); err == nil {
		t.Fatalf("expected error")
	}
	task, err := GetTask(db, "TAND-003")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "review" {
		t.Fatalf("expected review, got %q", task.Status)
	}
}

func TestRejectTaskDoesNotSetRejected(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := InsertTask(db, Task{ID: "TAND-004", Title: "Test", Status: "review"}); err != nil {
		t.Fatal(err)
	}
	if err := AddToReviewQueue(db, "TAND-004"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TRIGGER reject_status_block
BEFORE UPDATE ON tasks
WHEN NEW.status = 'rejected'
BEGIN
	SELECT RAISE(FAIL, 'rejected status not allowed');
END;
`); err != nil {
		t.Fatal(err)
	}
	if err := RejectTask(db, "TAND-004"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
