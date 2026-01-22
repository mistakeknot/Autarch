package storage

import "testing"

func TestApplyDetectionAtomicUpdatesSessionAndTask(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := InsertTask(db, Task{ID: "T1", Title: "t", Status: "in_progress"}); err != nil {
		t.Fatal(err)
	}
	if err := InsertSession(db, Session{ID: "S1", TaskID: "T1", State: "working", Offset: 0}); err != nil {
		t.Fatal(err)
	}

	if err := ApplyDetectionAtomic(db, "T1", "S1", "done"); err != nil {
		t.Fatal(err)
	}

	task, err := GetTask(db, "T1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "done" {
		t.Fatalf("expected task done, got %s", task.Status)
	}
	session, err := GetSession(db, "S1")
	if err != nil {
		t.Fatal(err)
	}
	if session.State != "done" {
		t.Fatalf("expected session done, got %s", session.State)
	}
}

func TestApplyDetectionAtomicRollsBackOnFailure(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := InsertTask(db, Task{ID: "T2", Title: "t", Status: "in_progress"}); err != nil {
		t.Fatal(err)
	}
	if err := InsertSession(db, Session{ID: "S2", TaskID: "T2", State: "working", Offset: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_task_update BEFORE UPDATE ON tasks BEGIN SELECT RAISE(ABORT, 'boom'); END;`); err != nil {
		t.Fatal(err)
	}

	if err := ApplyDetectionAtomic(db, "T2", "S2", "done"); err == nil {
		t.Fatal("expected error")
	}

	session, err := GetSession(db, "S2")
	if err != nil {
		t.Fatal(err)
	}
	if session.State != "working" {
		t.Fatalf("expected session rollback to working, got %s", session.State)
	}
}
