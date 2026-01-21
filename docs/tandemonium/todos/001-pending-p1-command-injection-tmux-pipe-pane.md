---
status: pending
priority: p1
issue_id: "001"
tags: [security, code-review]
dependencies: []
---

# Command Injection via tmux pipe-pane

## Problem Statement

The `StartSession` function in `internal/tmux/session.go` concatenates the `LogPath` directly into a shell command string passed to `pipe-pane -o`. If an attacker can control the `LogPath` value, they can inject arbitrary shell commands.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/tmux/session.go` lines 14-18

```go
func StartSession(r Runner, s Session) error {
    if err := r.Run("tmux", "new-session", "-d", "-s", s.ID, "-c", s.Workdir); err != nil {
        return err
    }
    return r.Run("tmux", "pipe-pane", "-t", s.ID, "-o", "cat >> "+s.LogPath)
}
```

**Exploitation Example:**
```
LogPath = "/tmp/log; rm -rf / #"
```
Results in: `cat >> /tmp/log; rm -rf / #`

**Impact:** Remote code execution with the privileges of the user running Tandemonium.

## Proposed Solutions

### Option 1: Shell-escape the LogPath (Recommended)
- **Pros:** Simple fix, preserves existing architecture
- **Cons:** Must ensure escaping is bulletproof
- **Effort:** Small
- **Risk:** Low if using battle-tested library

```go
import "github.com/alessio/shellescape"

func StartSession(r Runner, s Session) error {
    // ...
    return r.Run("tmux", "pipe-pane", "-t", s.ID, "-o", "cat >> "+shellescape.Quote(s.LogPath))
}
```

### Option 2: Validate LogPath characters
- **Pros:** Defense in depth
- **Cons:** May be too restrictive
- **Effort:** Small
- **Risk:** Low

```go
func validateLogPath(path string) error {
    if !regexp.MustCompile(`^[a-zA-Z0-9/_.-]+$`).MatchString(path) {
        return fmt.Errorf("invalid log path: %s", path)
    }
    return nil
}
```

### Option 3: Use alternative logging mechanism
- **Pros:** Eliminates shell entirely
- **Cons:** Architecture change
- **Effort:** Medium
- **Risk:** Medium

## Recommended Action

Implement Option 1 + Option 2 (defense in depth): shell-escape AND validate allowed characters.

## Technical Details

- **Affected files:** `internal/tmux/session.go`
- **Components:** Tmux session management
- **Database changes:** None

## Acceptance Criteria

- [ ] LogPath is shell-escaped before use in pipe-pane command
- [ ] LogPath is validated against allowlist pattern
- [ ] Test verifies shell metacharacters are rejected
- [ ] Test verifies valid paths still work

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during security review | Critical: shell injection in tmux commands |

## Resources

- PR: N/A (full codebase review)
- OWASP Command Injection: https://owasp.org/www-community/attacks/Command_Injection
