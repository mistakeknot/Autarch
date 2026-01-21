---
status: pending
priority: p3
issue_id: "018"
tags: [performance, database, code-review]
dependencies: []
---

# Enable SQLite WAL Mode

## Problem Statement

SQLite is opened without explicit WAL configuration, limiting concurrent read performance.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/storage/db.go` lines 11-13

```go
func Open(path string) (*sql.DB, error) {
    return sql.Open("sqlite", path)
}
```

No PRAGMA configuration for:
- WAL mode (better concurrent reads)
- Busy timeout (prevents lock errors)

## Proposed Solutions

### Option 1: Enable WAL Mode (Recommended)
- **Pros:** Better concurrent read performance
- **Cons:** Creates .wal and .shm files
- **Effort:** Trivial
- **Risk:** Low

```go
func Open(path string) (*sql.DB, error) {
    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, err
    }
    _, _ = db.Exec("PRAGMA journal_mode=WAL")
    _, _ = db.Exec("PRAGMA busy_timeout=5000")
    return db, nil
}
```

## Recommended Action

Enable WAL mode in Open function.

## Technical Details

- **Affected files:** `internal/storage/db.go`
- **Components:** Storage
- **Database changes:** WAL journal mode

## Acceptance Criteria

- [ ] WAL mode enabled
- [ ] Busy timeout set
- [ ] Tests pass
- [ ] Concurrent reads tested

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during performance review | WAL default is better for most apps |

## Resources

- SQLite WAL: https://www.sqlite.org/wal.html
