---
status: pending
priority: p2
issue_id: "012"
tags: [performance, code-review]
dependencies: []
---

# N+1 Git Operations in Branch Lookup

## Problem Statement

`RefreshReviewBranches` calls git for each task in the review queue, causing 10 separate git commands for 10 tasks.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/tui/model.go` lines 249-261

```go
func (m *Model) RefreshReviewBranches() {
    branches := map[string]string{}
    for _, id := range m.ReviewQueue {
        if branch, err := m.BranchLookup(id); err == nil {
            branches[id] = branch  // One git call per task
        }
    }
}
```

Where `BranchForTask` calls `git branch` and iterates:

```go
// git/branch.go:18-35
func BranchForTask(r Runner, taskID string) (string, error) {
    branches, err := ListBranches(r)  // Calls git each time!
    for _, b := range branches { ... }
}
```

**Projected Impact:**
- Git command overhead: ~50-100ms per call
- 10 reviews = 500-1000ms total latency

## Proposed Solutions

### Option 1: Batch Branch Lookup (Recommended)
- **Pros:** Single git call for all tasks
- **Cons:** Minor API change
- **Effort:** Small
- **Risk:** Low

```go
func BranchesForTasks(r Runner, taskIDs []string) (map[string]string, error) {
    branches, err := ListBranches(r)  // Single git call
    if err != nil {
        return nil, err
    }
    result := make(map[string]string)
    for _, id := range taskIDs {
        for _, b := range branches {
            if strings.Contains(strings.ToLower(b), strings.ToLower(id)) {
                result[id] = b
                break
            }
        }
    }
    return result, nil
}
```

## Recommended Action

Implement Option 1. Batch the git branch lookup.

## Technical Details

- **Affected files:**
  - `internal/git/branch.go`
  - `internal/tui/model.go`
- **Components:** Git, TUI
- **Database changes:** None

## Acceptance Criteria

- [ ] `BranchesForTasks` batch function created
- [ ] `RefreshReviewBranches` uses batch function
- [ ] Single git call for any number of tasks
- [ ] Performance test shows improvement

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during performance review | N+1 pattern in git ops |

## Resources

- N/A
