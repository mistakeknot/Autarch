---
status: pending
priority: p3
issue_id: "013"
tags: [simplification, yagni, code-review]
dependencies: []
---

# Delete Unused In-Memory Review Queue

## Problem Statement

The `internal/review/queue.go` package implements an in-memory queue that is never used in production. The actual review queue lives in SQLite.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/review/queue.go` (lines 1-12)

```go
type Queue struct {
    ids []string
}
func NewQueue() *Queue { return &Queue{ids: []string{}} }
func (q *Queue) Add(id string) { q.ids = append(q.ids, id) }
func (q *Queue) Len() int { return len(q.ids) }
```

**Evidence:** The only usage is in its own test file. The actual review queue uses `storage.ListReviewQueue(db)`.

**Impact:** 24 lines of dead code.

## Proposed Solutions

### Option 1: Delete Entire Package (Recommended)
- **Pros:** Eliminates dead code
- **Cons:** None
- **Effort:** Trivial
- **Risk:** None

```bash
rm -rf internal/review/
```

## Recommended Action

Delete the package.

## Technical Details

- **Affected files:**
  - Delete: `internal/review/queue.go`
  - Delete: `internal/review/queue_test.go`
- **Components:** Review (unused)
- **Database changes:** None

## Acceptance Criteria

- [ ] Package deleted
- [ ] Build still passes
- [ ] No import errors

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during simplicity review | YAGNI violation |

## Resources

- N/A
