# Tandemonium Foreign Keys Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `none (no bead in use)`

**Goal:** Enforce foreign key integrity for new Tandemonium databases by adding FK constraints and enabling `PRAGMA foreign_keys = ON` on connection.

**Architecture:** Update the migration schema for `review_queue` and `sessions` to reference `tasks(id)` with `ON DELETE CASCADE`. Ensure every connection enables `PRAGMA foreign_keys = ON`. No migration of existing DBs.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Go testing.

---

### Task 1: Add failing FK enforcement test

**Files:**
- Modify: `internal/tandemonium/storage/db_test.go`

**Step 1: Write failing test (orphan insert blocked)**

```go
func TestForeignKeysPreventOrphans(t *testing.T) {
    db, err := OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    if err := Migrate(db); err != nil {
        t.Fatal(err)
    }
    if _, err := db.Exec("INSERT INTO sessions (id, task_id, state, offset) VALUES ('S1', 'MISSING', 'working', 0)"); err == nil {
        t.Fatalf("expected FK violation")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/storage -run TestForeignKeysPreventOrphans`

Expected: FAIL (no FK enforcement yet).

---

### Task 2: Add failing cascade delete test

**Files:**
- Modify: `internal/tandemonium/storage/db_test.go`

**Step 1: Write failing test (cascade delete)**

```go
func TestForeignKeysCascadeDelete(t *testing.T) {
    db, err := OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    if err := Migrate(db); err != nil {
        t.Fatal(err)
    }
    if err := InsertTask(db, Task{ID: "T1", Title: "Test", Status: "todo"}); err != nil {
        t.Fatal(err)
    }
    if _, err := db.Exec("INSERT INTO review_queue (task_id) VALUES ('T1')"); err != nil {
        t.Fatal(err)
    }
    if _, err := db.Exec("INSERT INTO sessions (id, task_id, state, offset) VALUES ('S1', 'T1', 'working', 0)"); err != nil {
        t.Fatal(err)
    }
    if _, err := db.Exec("DELETE FROM tasks WHERE id = 'T1'"); err != nil {
        t.Fatal(err)
    }
    var count int
    _ = db.QueryRow("SELECT COUNT(*) FROM review_queue WHERE task_id = 'T1'").Scan(&count)
    if count != 0 {
        t.Fatalf("expected review_queue cleared")
    }
    _ = db.QueryRow("SELECT COUNT(*) FROM sessions WHERE task_id = 'T1'").Scan(&count)
    if count != 0 {
        t.Fatalf("expected sessions cleared")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/storage -run TestForeignKeysCascadeDelete`

Expected: FAIL (no FK constraints yet).

---

### Task 3: Enable foreign keys and add constraints

**Files:**
- Modify: `internal/tandemonium/storage/db.go`

**Step 1: Enable foreign_keys on connection**

Add a small helper and call it from `Open`:

```go
func Open(path string) (*sql.DB, error) {
    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, err
    }
    if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
        _ = db.Close()
        return nil, err
    }
    return db, nil
}
```

**Step 2: Add FK constraints in Migrate**

```sql
CREATE TABLE IF NOT EXISTS review_queue (
  task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  state TEXT NOT NULL,
  offset INTEGER NOT NULL DEFAULT 0
);
```

**Step 3: Run tests to verify they pass**

Run:
- `go test ./internal/tandemonium/storage -run TestForeignKeysPreventOrphans`
- `go test ./internal/tandemonium/storage -run TestForeignKeysCascadeDelete`

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/tandemonium/storage/db.go internal/tandemonium/storage/db_test.go
git commit -m "feat(tandemonium): add sqlite foreign keys"
```

---

### Task 4: Verify and update todo

**Files:**
- Modify: `docs/tandemonium/todos/009-pending-p2-missing-foreign-key-constraints.md`

**Step 1: Run full storage tests**

Run: `go test ./internal/tandemonium/storage`

Expected: PASS.

**Step 2: Update todo**

- Set `status: done`
- Check off acceptance criteria
- Add work log entry dated 2026-01-22 noting FK constraints + PRAGMA enablement

**Step 3: Commit**

```bash
git add docs/tandemonium/todos/009-pending-p2-missing-foreign-key-constraints.md
git commit -m "docs(tandemonium): close foreign key todo"
```

---

Plan complete and saved to `docs/plans/2026-01-22-tandemonium-foreign-keys-implementation-plan.md`.

Two execution options:

1. Subagent-Driven (this session) — I dispatch a fresh subagent per task, review between tasks
2. Parallel Session (separate) — Open a new session with executing-plans and batch execution

Which approach?
