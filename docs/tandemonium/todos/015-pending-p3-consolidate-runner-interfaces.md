---
status: pending
priority: p3
issue_id: "015"
tags: [architecture, duplication, code-review]
dependencies: []
---

# Consolidate Duplicate Runner Interfaces

## Problem Statement

Two different `Runner` interfaces exist with different signatures, causing confusion and duplicate `ExecRunner` implementations.

## Findings

**Location 1:** `/Users/sma/Tandemonium/internal/git/diff_runner.go` line 5
```go
type Runner interface { Run(name string, args ...string) (string, error) }
```

**Location 2:** `/Users/sma/Tandemonium/internal/tmux/session.go` line 3
```go
type Runner interface { Run(name string, args ...string) error }
```

**Impact:**
- Duplicate ExecRunner implementations
- Different return types prevent unification
- Confusing for new contributors

## Proposed Solutions

### Option 1: Unified Runner with Output (Recommended)
- **Pros:** Single interface, less duplication
- **Cons:** tmux ignores output
- **Effort:** Small
- **Risk:** Low

```go
// internal/exec/exec.go
type Runner interface {
    Run(name string, args ...string) (string, error)
}

type ExecRunner struct{}
func (e *ExecRunner) Run(name string, args ...string) (string, error) {
    out, err := exec.Command(name, args...).CombinedOutput()
    return string(out), err
}
```

## Recommended Action

Create shared `internal/exec` package with unified Runner.

## Technical Details

- **Affected files:**
  - Create: `internal/exec/exec.go`
  - Update: `internal/git/diff_runner.go` (use exec.Runner)
  - Update: `internal/tmux/session.go` (use exec.Runner)
- **Components:** Git, Tmux, new Exec package
- **Database changes:** None

## Acceptance Criteria

- [ ] Unified Runner interface created
- [ ] Single ExecRunner implementation
- [ ] Git and Tmux packages use shared interface
- [ ] Tests pass

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during pattern review | Same-named interfaces in different packages confuse |

## Resources

- N/A
