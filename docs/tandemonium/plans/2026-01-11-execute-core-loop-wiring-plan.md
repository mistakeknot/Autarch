# Execute Core Loop Wiring Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Wire the core execute loop end‑to‑end: start tasks with real worktrees + tmux sessions, stream logs with offsets, and update session/task state based on detection.

**Architecture:** Add concrete adapters for git worktrees and tmux sessions that implement the interfaces used by `agent.StartTask`, plus a lightweight runner that tails log files and updates SQLite session offsets + task statuses when detection signals completion or blocking.

**Tech Stack:** Go 1.24+, SQLite (modernc.org/sqlite), tmux, git.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Concrete adapters for worktree + tmux

**Files:**
- Create: `internal/agent/adapters.go`
- Create: `internal/agent/adapters_test.go`

**Step 1: Write the failing test**

```go
package agent

import "testing"

type fakeWorktreeCreator struct{ called bool }

type fakeSessionStarter struct{ called bool }

func TestAdaptersImplementInterfaces(t *testing.T) {
    var _ WorktreeCreator = (*GitWorktreeAdapter)(nil)
    var _ SessionStarter = (*TmuxSessionAdapter)(nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent -v`
Expected: FAIL with "undefined: GitWorktreeAdapter"

**Step 3: Implement adapters**

```go
package agent

import "github.com/gensysven/tandemonium/internal/git"
import "github.com/gensysven/tandemonium/internal/tmux"

// GitWorktreeAdapter implements WorktreeCreator using internal/git

type GitWorktreeAdapter struct{}

func (g *GitWorktreeAdapter) Create(repo, path, branch string) error {
    return git.CreateWorktree(repo, path, branch)
}

// TmuxSessionAdapter implements SessionStarter using internal/tmux

type TmuxSessionAdapter struct{ Runner tmux.Runner }

func (t *TmuxSessionAdapter) Start(id, workdir, logPath string) error {
    return tmux.StartSession(t.Runner, tmux.Session{ID: id, Workdir: workdir, LogPath: logPath})
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/adapters.go internal/agent/adapters_test.go
git commit -m "feat: add git/tmux adapters for workflow"
```

---

### Task 2: Session log streaming runner (offset persistence)

**Files:**
- Create: `internal/agent/streamer.go`
- Create: `internal/agent/streamer_test.go`

**Step 1: Write the failing test**

```go
package agent

import "testing"

func TestAdvanceOffset(t *testing.T) {
    next := advanceOffset(10, []string{"a", "b"})
    if next <= 10 {
        t.Fatal("expected offset to advance")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent -v`
Expected: FAIL with "undefined: advanceOffset"

**Step 3: Implement minimal streamer**

```go
func advanceOffset(offset int64, lines []string) int64 {
    var n int64
    for _, l := range lines {
        n += int64(len(l)) + 1 // newline
    }
    return offset + n
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/streamer.go internal/agent/streamer_test.go
git commit -m "feat: add streamer offset helper"
```

---

### Task 3: End-to-end update loop (detect -> update session/task)

**Files:**
- Create: `internal/agent/loop.go`
- Create: `internal/agent/loop_test.go`

**Step 1: Write the failing test**

```go
package agent

import "testing"

type fakeStore struct{ sessionUpdated, taskUpdated bool }

func (f *fakeStore) UpdateSessionState(id, state string) error { f.sessionUpdated = true; return nil }
func (f *fakeStore) UpdateTaskStatus(id, status string) error { f.taskUpdated = true; return nil }

func TestApplyDetection(t *testing.T) {
    fs := &fakeStore{}
    if err := ApplyDetection(fs, "TAND-001", "tand-TAND-001", "done"); err != nil {
        t.Fatal(err)
    }
    if !fs.sessionUpdated || !fs.taskUpdated {
        t.Fatal("expected both updates")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent -v`
Expected: FAIL with "undefined: ApplyDetection"

**Step 3: Implement minimal apply logic**

```go
type StatusStore interface {
    UpdateSessionState(id, state string) error
    UpdateTaskStatus(id, status string) error
}

func ApplyDetection(store StatusStore, taskID, sessionID, state string) error {
    if err := store.UpdateSessionState(sessionID, state); err != nil { return err }
    if state == "done" || state == "blocked" {
        return store.UpdateTaskStatus(taskID, state)
    }
    return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/loop.go internal/agent/loop_test.go
git commit -m "feat: add apply-detection update logic"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
