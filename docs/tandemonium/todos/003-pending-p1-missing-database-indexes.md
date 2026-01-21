---
status: pending
priority: p1
issue_id: "003"
tags: [performance, database, code-review]
dependencies: []
---

# Missing Database Indexes on Queried Columns

## Problem Statement

The SQLite schema creates no indexes beyond primary keys, causing O(n) table scans for common queries. With 1000+ sessions or tasks, performance will degrade significantly.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/storage/db.go` lines 24-39

```go
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL  // No index on status
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,  // No index on task_id
  state TEXT NOT NULL,
  offset INTEGER NOT NULL DEFAULT 0
);
```

**Affected Queries:**
- `FindSessionByTask` queries `WHERE task_id = ?` - full table scan
- `CountTasksByStatus` queries `GROUP BY status` - scans all rows
- Any future status filtering will be slow

**Projected Impact at Scale:**
- 1,000 sessions: O(1000) scan per lookup
- 500 tasks with status filtering: scans all rows

## Proposed Solutions

### Option 1: Add Indexes in Migration (Recommended)
- **Pros:** Immediate 10-100x improvement, trivial to implement
- **Cons:** None
- **Effort:** Small (5 minutes)
- **Risk:** Low

```go
func Migrate(db *sql.DB) error {
    _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS tasks (...);
CREATE TABLE IF NOT EXISTS sessions (...);
CREATE TABLE IF NOT EXISTS review_queue (...);
CREATE INDEX IF NOT EXISTS idx_sessions_task_id ON sessions(task_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
`)
    return err
}
```

## Recommended Action

Implement Option 1 immediately. This is a quick win with no downside.

## Technical Details

- **Affected files:** `internal/storage/db.go`
- **Components:** Storage layer
- **Database changes:** Two new indexes

## Acceptance Criteria

- [ ] Index on `sessions.task_id` created
- [ ] Index on `tasks.status` created
- [ ] Benchmark shows improvement for `FindSessionByTask`
- [ ] Existing tests still pass

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during performance review | Quick win, high impact |

## Resources

- SQLite Index Documentation: https://www.sqlite.org/lang_createindex.html
