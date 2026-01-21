# TUI Approve Action Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Add a minimal approve action in the TUI that triggers the approve flow and refreshes the view.

**Architecture:** Add a method on the TUI model that calls the existing approve command logic (merge + storage update) via small helpers. Keep it synchronous and text-only for MVP.

**Tech Stack:** Go 1.24+, git CLI, SQLite, Bubble Tea.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Approve helper in TUI

**Files:**
- Create: `internal/tui/approve.go`
- Create: `internal/tui/approve_test.go`

**Step 1: Write failing test**

```go
package tui

import "testing"

type fakeApprover struct{ called bool }

func (f *fakeApprover) Approve(taskID, branch string) error { f.called = true; return nil }

func TestModelApprove(t *testing.T) {
    m := NewModel()
    a := &fakeApprover{}
    _ = m.ApproveTask(a, "TAND-001", "feature/TAND-001")
    if !a.called { t.Fatal("expected approve call") }
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL

**Step 3: Implement helper**

```go
type Approver interface { Approve(taskID, branch string) error }

func (m *Model) ApproveTask(a Approver, taskID, branch string) error {
    return a.Approve(taskID, branch)
}
```

**Step 4: Run test**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/approve.go internal/tui/approve_test.go
git commit -m "feat: add TUI approve helper"
```

---

### Task 2: Wire approve helper to CLI logic (adapter)

**Files:**
- Create: `internal/tui/approve_adapter.go`
- Create: `internal/tui/approve_adapter_test.go`

**Step 1: Write failing test**

```go
package tui

import "testing"

func TestApproveAdapterImplements(t *testing.T) {
    var _ Approver = (*ApproveAdapter)(nil)
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL

**Step 3: Implement adapter**

```go
package tui

import (
    "github.com/gensysven/tandemonium/internal/git"
    "github.com/gensysven/tandemonium/internal/project"
    "github.com/gensysven/tandemonium/internal/storage"
)

type ApproveAdapter struct{}

func (a *ApproveAdapter) Approve(taskID, branch string) error {
    if err := git.MergeBranch(&git.ExecRunner{}, branch); err != nil {
        return err
    }
    root, err := project.FindRoot(".")
    if err != nil { return err }
    db, err := storage.Open(project.StateDBPath(root))
    if err != nil { return err }
    defer db.Close()
    return storage.ApproveTask(db, taskID)
}
```

**Step 4: Run test**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/approve_adapter.go internal/tui/approve_adapter_test.go
git commit -m "feat: add TUI approve adapter"
```

---

### Task 3: Minimal key hook in TUI (placeholder)

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/approve_key_test.go`

**Step 1: Write failing test**

```go
package tui

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea"
)

type fakeApprover struct {
    called   bool
    taskID   string
    branch   string
}

func (f *fakeApprover) Approve(taskID, branch string) error {
    f.called = true
    f.taskID = taskID
    f.branch = branch
    return nil
}

func TestApproveKeyCallsApprover(t *testing.T) {
    m := NewModel()
    m.ReviewQueue = []string{"TAND-001"}
    fake := &fakeApprover{}
    m.Approver = fake

    _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

    if !fake.called {
        t.Fatal("expected approve call")
    }
    if fake.taskID != "TAND-001" {
        t.Fatalf("expected task ID, got %q", fake.taskID)
    }
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL

**Step 3: Implement minimal key handling**

Add a case in Update to handle key "a" and call ApproveTask with stub adapter only if ReviewQueue not empty.

**Step 4: Run test**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/approve_key_test.go
git commit -m "feat: add approve key stub"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
