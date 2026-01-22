---
status: done
priority: p1
issue_id: "006"
tags: [data-integrity, database, code-review]
dependencies: []
---

# Missing Transaction Boundaries in Multi-Step Operations

## Problem Statement

Multiple database operations that should be atomic are executed as separate statements. A crash between operations leaves the database in an inconsistent state.

## Findings

**Location 1:** `/Users/sma/Tandemonium/internal/storage/review.go` lines 32-47

```go
func ApproveTask(db *sql.DB, id string) error {
    if err := UpdateTaskStatus(db, id, "done"); err != nil {
        return err
    }
    return RemoveFromReviewQueue(db, id)  // No transaction boundary!
}

func RejectTask(db *sql.DB, id string) error {
    if err := UpdateTaskStatus(db, id, "rejected"); err != nil {
        return err
    }
    if err := UpdateTaskStatus(db, id, "ready"); err != nil {  // Bug: overwrites
        return err
    }
    return RemoveFromReviewQueue(db, id)  // Three operations with no atomicity
}
```

**Location 2:** `/Users/sma/Tandemonium/internal/agent/loop.go` lines 9-22

```go
func ApplyDetection(store StatusStore, taskID, sessionID, state string) error {
    if err := store.UpdateSessionState(sessionID, state); err != nil {
        return err
    }
    if state == "done" || state == "blocked" {
        if err := store.UpdateTaskStatus(taskID, state); err != nil {
            return err  // Session updated but task not - inconsistent!
        }
    }
}
```

**Risk Scenarios:**
- Crash between `UpdateTaskStatus` and `RemoveFromReviewQueue` leaves task marked done but still in queue
- `RejectTask` has three separate updates - any failure leaves partially applied state

## Proposed Solutions

### Option 1: Explicit Transactions (Recommended)
- **Pros:** Proper ACID guarantees
- **Cons:** Slightly more complex API
- **Effort:** Medium
- **Risk:** Low

```go
func ApproveTask(db *sql.DB, id string) error {
    tx, err := db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    if _, err := tx.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, "done", id); err != nil {
        return err
    }
    if _, err := tx.Exec(`DELETE FROM review_queue WHERE task_id = ?`, id); err != nil {
        return err
    }
    return tx.Commit()
}
```

## Recommended Action

Implement Option 1 for all multi-step database operations.

## Technical Details

- **Affected files:**
  - `internal/storage/review.go`
  - `internal/agent/loop.go`
- **Components:** Storage, Agent
- **Database changes:** None (just wrapping in transactions)

## Resolution

Added a storage-layer transaction helper for ApplyDetection and updated the agent loop to call it.

## Acceptance Criteria

- [x] `ApproveTask` wrapped in transaction
- [x] `RejectTask` wrapped in transaction
- [x] `ApplyDetection` wrapped in transaction (may need API change)
- [x] Tests verify rollback on partial failure
- [x] Tests verify atomicity

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during data integrity review | Multi-step DB ops need transactions |
| 2026-01-22 | Added ApplyDetectionAtomic + tests | Prevents partial updates on crash |

## Resources

- Go database/sql transactions: https://pkg.go.dev/database/sql#DB.Begin
