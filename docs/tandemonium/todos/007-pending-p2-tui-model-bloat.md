---
status: pending
priority: p2
issue_id: "007"
tags: [architecture, maintainability, code-review]
dependencies: []
---

# TUI Model Struct Growing Large (Potential God Object)

## Problem Statement

The `Model` struct has grown to 25+ fields managing multiple concerns, making it difficult to maintain and test. The file has been modified in 23 of the last 100 commits.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/tui/model.go` lines 15-41

```go
type Model struct {
    Title       string
    Sessions    []string
    ReviewQueue []string
    DiffFiles   []string
    Approver    Approver
    BranchLookup BranchLookup
    Status      string
    StatusLevel StatusLevel
    ReviewLoader ReviewLoader
    ReviewBranches map[string]string
    SelectedReview int
    ConfirmApprove    bool
    PendingApproveTask string
    ViewMode        ViewMode
    ReviewShowDiffs bool
    ReviewInputMode   ReviewInputMode
    ReviewInput       string
    ReviewPendingReject bool
    ReviewDetail       ReviewDetail
    ReviewDetailLoader func(taskID string) (ReviewDetail, error)
    ReviewDiff         ReviewDiffState
    ReviewDiffLoader   func(taskID string) (ReviewDiffState, error)
    ReviewActionWriter func(taskID, text string) error
    ReviewStoryUpdater func(taskID, text string) error
    ReviewRejecter      func(taskID string) error
}
```

**Impact:**
- 509 lines in a single file
- Mixes fleet view, review view, status, and callback state
- Difficult to test individual components
- Update() method has deeply nested switch statements

## Proposed Solutions

### Option 1: Extract Sub-Models (Recommended)
- **Pros:** Clear separation, easier testing
- **Cons:** More files, need to pass parent reference
- **Effort:** Medium
- **Risk:** Low

```go
type Model struct {
    Fleet    FleetState
    Review   ReviewState
    Status   StatusState
    Services Services  // Injected dependencies
}

type ReviewState struct {
    Queue          []string
    Selected       int
    Branches       map[string]string
    Detail         ReviewDetail
    Diff           ReviewDiffState
    InputMode      ReviewInputMode
    Input          string
    PendingReject  bool
    PendingApprove string
}
```

### Option 2: Delegate to View-Specific Handlers
- **Pros:** Keep single Model, reduce Update complexity
- **Cons:** Still one large struct
- **Effort:** Small
- **Risk:** Low

## Recommended Action

Implement Option 1 when adding next major feature. Extract ReviewState first as it has the most fields.

## Technical Details

- **Affected files:** `internal/tui/model.go`
- **Components:** TUI
- **Database changes:** None

## Acceptance Criteria

- [ ] ReviewState extracted to separate struct
- [ ] FleetState extracted (when fleet view is built)
- [ ] Services struct for injected dependencies
- [ ] Each sub-model has focused tests
- [ ] Update() method delegates to sub-handlers

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during architecture review | 23 modifications in 100 commits indicates hotspot |

## Resources

- Clean Architecture: https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html
