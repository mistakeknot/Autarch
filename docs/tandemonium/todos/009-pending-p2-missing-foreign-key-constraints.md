---
status: pending
priority: p2
issue_id: "009"
tags: [data-integrity, database, code-review]
dependencies: []
---

# Missing Foreign Key Constraints in SQLite Schema

## Problem Statement

The schema defines no foreign key constraints, allowing orphaned records in `review_queue` and `sessions` tables that reference non-existent tasks.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/storage/db.go` lines 24-39

```go
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS review_queue (
  task_id TEXT PRIMARY KEY  // No FK to tasks!
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,    // No FK to tasks!
  state TEXT NOT NULL,
  offset INTEGER NOT NULL DEFAULT 0
);
```

**Risk Scenarios:**
- `review_queue` can contain task IDs that don't exist
- `sessions` can reference non-existent tasks
- Deleting a task leaves orphaned records

## Proposed Solutions

### Option 1: Add FK Constraints with CASCADE (Recommended)
- **Pros:** Automatic cleanup, referential integrity
- **Cons:** Need to enable PRAGMA foreign_keys
- **Effort:** Small
- **Risk:** Low

```go
func Migrate(db *sql.DB) error {
    _, _ = db.Exec("PRAGMA foreign_keys = ON")
    _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS review_queue (
  task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  state TEXT NOT NULL,
  offset INTEGER NOT NULL DEFAULT 0
);
`)
    return err
}
```

## Recommended Action

Implement Option 1. Enable foreign_keys PRAGMA and add constraints.

## Technical Details

- **Affected files:** `internal/storage/db.go`
- **Components:** Storage
- **Database changes:** Foreign key constraints

## Acceptance Criteria

- [ ] PRAGMA foreign_keys = ON set on connection
- [ ] Foreign keys added to review_queue and sessions
- [ ] ON DELETE CASCADE behavior verified
- [ ] Tests verify orphan prevention

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during data integrity review | SQLite FKs off by default |

## Resources

- SQLite Foreign Keys: https://www.sqlite.org/foreignkeys.html
