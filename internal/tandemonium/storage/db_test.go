package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestCreateAndReadTask(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	task := Task{ID: "TAND-001", Title: "Test", Status: "todo"}
	if err := InsertTask(db, task); err != nil {
		t.Fatal(err)
	}

	got, err := GetTask(db, "TAND-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Test" {
		t.Fatalf("expected title Test, got %s", got.Title)
	}
}

func TestMigrateCreatesIndexes(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	if !hasIndex(t, db, "sessions", "idx_sessions_task_id") {
		t.Fatalf("expected sessions task_id index")
	}
	if !hasIndex(t, db, "tasks", "idx_tasks_status") {
		t.Fatalf("expected tasks status index")
	}
}

func hasIndex(t *testing.T, db *sql.DB, table, name string) bool {
	rows, err := db.Query("PRAGMA index_list('" + table + "')")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var idxName string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &idxName, &unique, &origin, &partial); err != nil {
			t.Fatal(err)
		}
		if idxName == name {
			return true
		}
	}
	return false
}

func TestOpenSharedReturnsSameInstance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	db1, err := OpenShared(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	db2, err := OpenShared(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db1 != db2 {
		t.Fatalf("expected shared db instance")
	}
}
