package storage

import "testing"

func TestUpdateTaskStatus(t *testing.T) {
    db, err := OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    if err := Migrate(db); err != nil {
        t.Fatal(err)
    }

    if err := InsertTask(db, Task{ID: "TAND-001", Title: "Test", Status: "todo"}); err != nil {
        t.Fatal(err)
    }
    if err := UpdateTaskStatus(db, "TAND-001", "in_progress"); err != nil {
        t.Fatal(err)
    }
    got, err := GetTask(db, "TAND-001")
    if err != nil {
        t.Fatal(err)
    }
    if got.Status != "in_progress" {
        t.Fatalf("expected in_progress, got %s", got.Status)
    }
}
