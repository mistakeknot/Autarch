# Tandemonium ApplyDetection Atomic Update Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `none (no bead in use)`

**Goal:** Ensure session + task status updates in ApplyDetection are atomic by using a storage-layer transaction helper.

**Architecture:** Add a storage helper that updates session state and (optionally) task status within a single transaction. Update the agent loop to call this helper instead of two separate operations. Keep the StatusStore interface unchanged for other uses.

**Tech Stack:** Go, SQLite (`database/sql`), existing storage layer.

---

### Task 1: Add failing tests for atomic detection updates

**Files:**
- Modify: `internal/tandemonium/storage/review_test.go` (or new test file)
- Modify: `internal/tandemonium/agent/loop_test.go`

**Step 1: Write failing test (storage helper updates both records)**

```go
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

    task, _ := GetTask(db, "T1")
    if task.Status != "done" {
        t.Fatalf("expected task done, got %s", task.Status)
    }
    session, _ := FindSessionByTask(db, "T1")
    if session.State != "done" {
        t.Fatalf("expected session done, got %s", session.State)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/storage -run TestApplyDetectionAtomicUpdatesSessionAndTask`

Expected: FAIL with `ApplyDetectionAtomic` undefined.

**Step 3: Write failing test (agent uses helper)**

```go
func TestApplyDetectionUsesAtomicHelper(t *testing.T) {
    called := false
    store := &fakeStatusStore{
        ApplyAtomic: func(taskID, sessionID, state string) error {
            called = true
            return nil
        },
    }
    if err := ApplyDetection(store, "T1", "S1", "done"); err != nil {
        t.Fatal(err)
    }
    if !called {
        t.Fatal("expected ApplyDetectionAtomic called")
    }
}
```

**Step 4: Run test to verify it fails**

Run: `go test ./internal/tandemonium/agent -run TestApplyDetectionUsesAtomicHelper`

Expected: FAIL due to missing interface method or missing helper.

---

### Task 2: Implement storage helper

**Files:**
- Modify: `internal/tandemonium/storage/review.go`
- Modify: `internal/tandemonium/storage/review_test.go` (or new test file)

**Step 1: Write minimal implementation**

```go
func ApplyDetectionAtomic(db *sql.DB, taskID, sessionID, state string) error {
    tx, err := db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    if _, err := tx.Exec(`UPDATE sessions SET state = ? WHERE id = ?`, state, sessionID); err != nil {
        return err
    }
    if state == "done" || state == "blocked" {
        if _, err := tx.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, state, taskID); err != nil {
            return err
        }
    }
    return tx.Commit()
}
```

**Step 2: Run test to verify it passes**

Run: `go test ./internal/tandemonium/storage -run TestApplyDetectionAtomicUpdatesSessionAndTask`

Expected: PASS.

---

### Task 3: Update agent loop to use helper

**Files:**
- Modify: `internal/tandemonium/agent/loop.go`
- Modify: `internal/tandemonium/agent/loop_test.go`

**Step 1: Update StatusStore**

Add method to interface:

```go
type StatusStore interface {
    UpdateSessionState(sessionID, state string) error
    UpdateTaskStatus(taskID, status string) error
    EnqueueReview(taskID string) error
    ApplyDetectionAtomic(taskID, sessionID, state string) error
}
```

**Step 2: Update ApplyDetection**

```go
func ApplyDetection(store StatusStore, taskID, sessionID, state string) error {
    if err := store.ApplyDetectionAtomic(taskID, sessionID, state); err != nil {
        return err
    }
    if state == "done" || state == "blocked" {
        return store.EnqueueReview(taskID)
    }
    return nil
}
```

**Step 3: Run test to verify it passes**

Run: `go test ./internal/tandemonium/agent -run TestApplyDetectionUsesAtomicHelper`

Expected: PASS.

---

### Task 4: Implement StatusStore for storage-backed usage

**Files:**
- Modify: `internal/tandemonium/agent/loop.go` (if a concrete store exists)
- Modify: `internal/tandemonium/storage` (new adapter if needed)

**Step 1: Provide storage-backed implementation**

If there is a storage adapter, add:

```go
func (s *StorageStatusStore) ApplyDetectionAtomic(taskID, sessionID, state string) error {
    return storage.ApplyDetectionAtomic(s.db, taskID, sessionID, state)
}
```

**Step 2: Run broader tests**

Run: `go test ./internal/tandemonium/agent ./internal/tandemonium/storage`

Expected: PASS.

---

### Task 5: Update todo and commit

**Files:**
- Modify: `docs/tandemonium/todos/006-pending-p1-missing-transaction-boundaries.md`

**Step 1: Update todo**

Mark todo as done and note ApplyDetection now uses storage transaction helper.

**Step 2: Commit**

```bash
git add internal/tandemonium/storage/review.go internal/tandemonium/storage/review_test.go internal/tandemonium/agent/loop.go internal/tandemonium/agent/loop_test.go docs/tandemonium/todos/006-pending-p1-missing-transaction-boundaries.md docs/plans/2026-01-22-tandemonium-apply-detection-atomic-plan.md
git commit -m "fix(tandemonium): make detection updates atomic"
```

---

Plan complete and saved to `docs/plans/2026-01-22-tandemonium-apply-detection-atomic-plan.md`.

Two execution options:

1. Subagent-Driven (this session) — I dispatch a fresh subagent per task, review between tasks
2. Parallel Session (separate) — Open a new session with executing-plans and batch execution

Which approach?
