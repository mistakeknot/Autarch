---
status: pending
priority: p2
issue_id: "008"
tags: [bug, data-integrity, code-review]
dependencies: []
---

# RejectTask Has Redundant/Buggy Status Update

## Problem Statement

The `RejectTask` function sets status to "rejected", then immediately overwrites to "ready". The first status update is pointless or indicates a logic error.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/storage/review.go` lines 39-47

```go
func RejectTask(db *sql.DB, id string) error {
    if err := UpdateTaskStatus(db, id, "rejected"); err != nil {
        return err
    }
    if err := UpdateTaskStatus(db, id, "ready"); err != nil {  // Overwrites previous!
        return err
    }
    return RemoveFromReviewQueue(db, id)
}
```

**Impact:**
- "rejected" status is never visible to any code
- Extra database write for no purpose
- May indicate missing business logic (e.g., should log rejection before returning to ready)

## Proposed Solutions

### Option 1: Remove Redundant Status (If Ready is Correct)
- **Pros:** Simple fix
- **Cons:** May lose intended behavior
- **Effort:** Trivial
- **Risk:** Low

```go
func RejectTask(db *sql.DB, id string) error {
    if err := UpdateTaskStatus(db, id, "ready"); err != nil {
        return err
    }
    return RemoveFromReviewQueue(db, id)
}
```

### Option 2: Keep Rejected Status (If That Was Intended)
- **Pros:** May be the correct business logic
- **Cons:** Need to understand requirements
- **Effort:** Trivial
- **Risk:** Low

```go
func RejectTask(db *sql.DB, id string) error {
    if err := UpdateTaskStatus(db, id, "rejected"); err != nil {
        return err
    }
    return RemoveFromReviewQueue(db, id)
}
```

## Recommended Action

Clarify intended behavior, then fix accordingly. Most likely Option 1 is correct (rejected tasks go back to "ready" for rework).

## Technical Details

- **Affected files:** `internal/storage/review.go`
- **Components:** Storage
- **Database changes:** None

## Acceptance Criteria

- [ ] Determine intended rejection flow
- [ ] Remove redundant status update
- [ ] Add test for rejection behavior
- [ ] Document expected state transitions

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during pattern/data-integrity review | Likely copy-paste error |

## Resources

- N/A
