package storage

import "testing"

func TestHasTasksTable(t *testing.T) {
    db, err := OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    if err := Migrate(db); err != nil {
        t.Fatal(err)
    }
    ok, err := HasTasksTable(db)
    if err != nil {
        t.Fatal(err)
    }
    if !ok {
        t.Fatal("expected tasks table")
    }
}
