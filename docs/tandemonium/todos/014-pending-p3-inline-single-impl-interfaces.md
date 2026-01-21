---
status: pending
priority: p3
issue_id: "014"
tags: [simplification, architecture, code-review]
dependencies: []
---

# Inline Single-Implementation Interfaces

## Problem Statement

Several interfaces have exactly one implementation, adding complexity without value. The adapters are thin wrappers that could be replaced with function types.

## Findings

**Approver Interface:**
- `internal/tui/approve.go` - Interface definition (10 lines)
- `internal/tui/approve_adapter.go` - Only implementation (25 lines)

**Agent Interfaces:**
- `internal/agent/workflow.go` - WorktreeCreator, SessionStarter interfaces
- `internal/agent/adapters.go` - Thin wrapper implementations (19 lines)

**Diff Loader:**
- `internal/tui/diff_loader.go` - 1-line pass-through to git.DiffNameOnly (7 lines)

**Total:** ~61 lines of indirection

## Proposed Solutions

### Option 1: Replace with Function Types (Recommended)
- **Pros:** Less indirection, equally testable
- **Cons:** Pattern change
- **Effort:** Small
- **Risk:** Low

```go
// Instead of Approver interface:
type Model struct {
    Approve func(taskID, branch string) error
}

// In NewModel or initialization:
m.Approve = func(taskID, branch string) error {
    // inline implementation
}
```

## Recommended Action

Replace interfaces with function types when refactoring.

## Technical Details

- **Affected files:**
  - Delete: `internal/tui/approve.go`
  - Delete: `internal/tui/approve_adapter.go`
  - Delete: `internal/agent/adapters.go`
  - Delete: `internal/tui/diff_loader.go`
  - Update: `internal/tui/model.go`
  - Update: `internal/agent/workflow.go`
- **Components:** TUI, Agent
- **Database changes:** None

## Acceptance Criteria

- [ ] Approver interface replaced with function
- [ ] Agent adapters inlined
- [ ] diff_loader.go deleted
- [ ] Tests still pass
- [ ] ~61 LOC removed

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during simplicity review | Single-impl interfaces are premature abstraction |

## Resources

- N/A
