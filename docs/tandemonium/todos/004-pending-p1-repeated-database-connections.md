---
status: done
priority: p1
issue_id: "004"
tags: [performance, architecture, code-review]
dependencies: []
---

# Repeated Database Connection Opens

## Problem Statement

Every user action (approve, reject, load review, load detail) opens a new SQLite database connection. This creates 1-5ms overhead per operation and defeats connection pooling.

## Findings

**Evidence:** 10+ instances of `storage.Open` in production code, each creating a new connection.

**Locations:**
- `/Users/sma/Tandemonium/internal/tui/review_loader.go` lines 14-25
- `/Users/sma/Tandemonium/internal/tui/model.go` lines 344-349
- `/Users/sma/Tandemonium/internal/tui/review_detail.go`
- `/Users/sma/Tandemonium/internal/tui/approve_adapter.go`

```go
// Repeated pattern:
func LoadReviewQueueFromProject() ([]string, error) {
    root, err := project.FindRoot(".")
    db, err := storage.Open(project.StateDBPath(root))
    defer db.Close()
    return storage.ListReviewQueue(db)
}
```

**Projected Impact:**
- SQLite connection overhead: ~1-5ms per open
- With 50 reviews and multiple operations: 250-1000ms cumulative latency
- File descriptor churn under concurrent usage

## Proposed Solutions

### Option 1: Pass DB Through Model (Recommended)
- **Pros:** Clean dependency injection, testable
- **Cons:** Requires refactoring Model constructor
- **Effort:** Medium
- **Risk:** Low

```go
type Model struct {
    db *sql.DB  // Opened once at startup
}

func NewModelWithDB(db *sql.DB) Model {
    return Model{db: db}
}
```

### Option 2: Singleton Connection
- **Pros:** Minimal code changes
- **Cons:** Global state, harder to test
- **Effort:** Small
- **Risk:** Medium

## Recommended Action

Implement Option 1. Refactor Model to hold a persistent database connection opened at startup.

## Technical Details

- **Affected files:**
  - `internal/tui/model.go`
  - `internal/tui/review_loader.go`
  - `internal/tui/review_detail.go`
  - `internal/tui/approve_adapter.go`
- **Components:** TUI, Storage
- **Database changes:** None

## Resolution

The TUI now opens a shared DB once in `execute` and passes it into `NewModelWithDB`, which wires loaders/adapters to use the shared connection. Fallback paths still open when no DB is provided.

## Acceptance Criteria

- [x] Model struct holds persistent `*sql.DB`
- [x] Database opened once in `main()` or `NewModel()`
- [x] All TUI operations use the shared connection
- [x] Connection closed on application exit
- [x] Tests verify single connection pattern

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during performance review | Connection reuse critical for TUI responsiveness |
| 2026-01-22 | Reused shared DB in TUI + added tests | Avoided repeated connection churn |

## Resources

- Go database/sql package: https://pkg.go.dev/database/sql
