package storage

import "testing"

func TestSessionCRUD(t *testing.T) {
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
    s := Session{ID: "tand-TAND-001", TaskID: "TAND-001", State: "working", Offset: 10}
    if err := InsertSession(db, s); err != nil {
        t.Fatal(err)
    }
    if err := UpdateSessionOffset(db, s.ID, 42); err != nil {
        t.Fatal(err)
    }
    got, err := GetSession(db, s.ID)
    if err != nil {
        t.Fatal(err)
    }
    if got.Offset != 42 {
        t.Fatalf("expected offset 42, got %d", got.Offset)
    }
}
