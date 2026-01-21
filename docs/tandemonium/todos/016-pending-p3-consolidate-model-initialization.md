---
status: pending
priority: p3
issue_id: "016"
tags: [architecture, simplification, code-review]
dependencies: []
---

# Consolidate Lazy Model Initialization

## Problem Statement

The `handleReviewSubmit` method has repeated patterns of lazy initialization for callbacks, creating scattered nil-check logic.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/tui/model.go` lines 320-392

The pattern repeats 4 times:
```go
writer := m.ReviewActionWriter
if writer == nil {
    writer = func(id, text string) error {
        root, err := project.FindRoot(".")
        // ... 6 more lines ...
    }
    m.ReviewActionWriter = writer
}
```

Similar patterns for:
- `ReviewRejecter`
- `ReviewStoryUpdater`
- `BranchLookup`

**Impact:** ~50 lines of scattered initialization code.

## Proposed Solutions

### Option 1: Initialize in Factory Function (Recommended)
- **Pros:** Single initialization point, cleaner Update()
- **Cons:** More upfront setup
- **Effort:** Small
- **Risk:** Low

```go
func NewModelWithDefaults() Model {
    m := NewModel()
    m.ReviewLoader = LoadReviewQueueFromProject
    m.ReviewDetailLoader = LoadReviewDetail
    m.ReviewDiffLoader = LoadReviewDiff
    m.ReviewActionWriter = defaultReviewWriter
    m.ReviewStoryUpdater = defaultStoryUpdater
    m.ReviewRejecter = defaultRejecter
    m.BranchLookup = func(taskID string) (string, error) {
        return git.BranchForTask(&git.ExecRunner{}, taskID)
    }
    return m
}
```

## Recommended Action

Create `NewModelWithDefaults()` and remove nil checks from business logic.

## Technical Details

- **Affected files:** `internal/tui/model.go`
- **Components:** TUI
- **Database changes:** None

## Acceptance Criteria

- [ ] All default callbacks initialized in factory
- [ ] Nil checks removed from handleReviewSubmit
- [ ] Tests can inject custom callbacks easily
- [ ] ~50 LOC reduction

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during simplicity review | Lazy init obscures dependencies |

## Resources

- N/A
