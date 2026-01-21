---
status: pending
priority: p1
issue_id: "002"
tags: [security, code-review]
dependencies: []
---

# Missing Task ID Input Validation

## Problem Statement

Task IDs from command-line arguments and user input are used directly without validation throughout the codebase. This affects session ID construction, branch name generation, file path construction, and can lead to path traversal and command injection vulnerabilities.

## Findings

**Locations:**
- `/Users/sma/Tandemonium/internal/cli/commands/approve.go` lines 17-19
- `/Users/sma/Tandemonium/internal/agent/launcher.go` lines 3-5
- `/Users/sma/Tandemonium/internal/agent/workflow.go` (branch name generation)
- `/Users/sma/Tandemonium/internal/tui/review_detail.go` lines 33-40 (path traversal)
- `/Users/sma/Tandemonium/internal/tui/model.go` lines 327-328 (path traversal)

**Example - Branch Name Injection:**
```go
func StartTask(w WorktreeCreator, s SessionStarter, taskID, repo, worktree, logPath string) error {
    if err := w.Create(repo, worktree, "feature/"+taskID); err != nil {
```
A malicious taskID could contain characters that cause issues in git or filesystem operations.

**Example - Path Traversal:**
```go
specPath := filepath.Join(project.SpecsDir(root), taskID+".yaml")
```
With `taskID = "../../../etc/passwd"`, this reads/writes outside the specs directory.

## Proposed Solutions

### Option 1: Centralized Validation Function (Recommended)
- **Pros:** Single source of truth, easy to maintain
- **Cons:** Must be applied at all entry points
- **Effort:** Small
- **Risk:** Low

```go
// internal/validation/task_id.go
var taskIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func ValidateTaskID(id string) error {
    if !taskIDPattern.MatchString(id) {
        return fmt.Errorf("invalid task ID: %q", id)
    }
    return nil
}
```

### Option 2: Path Sanitization
- **Pros:** Catches path traversal specifically
- **Cons:** Doesn't prevent other issues
- **Effort:** Small
- **Risk:** Low

```go
func safePath(base, untrusted string) (string, error) {
    full := filepath.Join(base, filepath.Clean(untrusted))
    if !strings.HasPrefix(full, base) {
        return "", fmt.Errorf("path traversal attempt: %s", untrusted)
    }
    return full, nil
}
```

## Recommended Action

Implement Option 1 (validation) at all entry points AND Option 2 (path sanitization) for file operations. Defense in depth.

## Technical Details

- **Affected files:**
  - `internal/cli/commands/approve.go`
  - `internal/agent/launcher.go`
  - `internal/agent/workflow.go`
  - `internal/tui/review_detail.go`
  - `internal/tui/model.go`
  - New: `internal/validation/task_id.go`
- **Components:** CLI, Agent, TUI
- **Database changes:** None

## Acceptance Criteria

- [ ] Task ID validation function created
- [ ] Validation applied at CLI entry points
- [ ] Validation applied in TUI before file operations
- [ ] Path sanitization added for all file path construction
- [ ] Tests verify malicious IDs are rejected
- [ ] Tests verify path traversal is blocked

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during security review | Multiple attack vectors via task ID |

## Resources

- OWASP Path Traversal: https://owasp.org/www-community/attacks/Path_Traversal
