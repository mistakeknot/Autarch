# Tandemonium TUI DB Reuse Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `none (no bead in use)`

**Goal:** Open the TUI SQLite database once at startup and reuse that connection across TUI operations to eliminate repeated opens.

**Architecture:** The `execute` command will open and migrate the state DB once and pass it into a new `tui.NewModelWithDB` constructor. The model will store the shared `*sql.DB` and wire default loaders/adapters to use it, falling back to the existing project-based loaders when no DB is provided.

**Tech Stack:** Go, `database/sql`, SQLite (`modernc.org/sqlite`), Bubble Tea.

---

### Task 1: Add failing tests for DB reuse

**Files:**
- Create: `internal/tandemonium/tui/model_db_test.go`
- Create: `internal/tandemonium/tui/approve_adapter_test.go`

**Step 1: Write the failing test (model uses shared DB)**

```go
package tui

import (
    "testing"

    "github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestNewModelWithDBUsesDBLoaders(t *testing.T) {
    db, err := storage.OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    if err := storage.Migrate(db); err != nil {
        t.Fatal(err)
    }
    if err := storage.InsertTask(db, storage.Task{ID: "TAND-DB-1", Title: "From DB", Status: "todo"}); err != nil {
        t.Fatal(err)
    }
    if err := storage.AddToReviewQueue(db, "TAND-DB-1"); err != nil {
        t.Fatal(err)
    }

    m := NewModelWithDB(db)
    m.RefreshTasks()
    if len(m.TaskList) == 0 || m.TaskList[0].ID != "TAND-DB-1" {
        t.Fatalf("expected TaskList from DB")
    }

    m.RefreshReviewQueue()
    if len(m.ReviewQueue) == 0 || m.ReviewQueue[0] != "TAND-DB-1" {
        t.Fatalf("expected ReviewQueue from DB")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/tui -run TestNewModelWithDBUsesDBLoaders`

Expected: FAIL with compile error `NewModelWithDB` undefined.

**Step 3: Write the failing test (ApproveAdapter uses provided DB)**

```go
package tui

import (
    "testing"

    "github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/git"
    "github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

type fakeGitRunner struct{ calls [][]string }

func (f *fakeGitRunner) Run(name string, args ...string) (string, error) {
    f.calls = append(f.calls, append([]string{name}, args...))
    return "", nil
}

func TestApproveAdapterUsesProvidedDB(t *testing.T) {
    db, err := storage.OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    if err := storage.Migrate(db); err != nil {
        t.Fatal(err)
    }
    if err := storage.InsertTask(db, storage.Task{ID: "TAND-DB-2", Title: "Approve", Status: "review"}); err != nil {
        t.Fatal(err)
    }
    if err := storage.AddToReviewQueue(db, "TAND-DB-2"); err != nil {
        t.Fatal(err)
    }

    runner := &fakeGitRunner{}
    adapter := &ApproveAdapter{DB: db, Runner: runner}
    if err := adapter.Approve("TAND-DB-2", "feature/TAND-DB-2"); err != nil {
        t.Fatal(err)
    }

    if len(runner.calls) == 0 {
        t.Fatalf("expected git merge call")
    }

    task, err := storage.GetTask(db, "TAND-DB-2")
    if err != nil {
        t.Fatal(err)
    }
    if task.Status != "done" {
        t.Fatalf("expected status done, got %s", task.Status)
    }

    queue, err := storage.ListReviewQueue(db)
    if err != nil {
        t.Fatal(err)
    }
    if len(queue) != 0 {
        t.Fatalf("expected review queue cleared")
    }
}
```

**Step 4: Run test to verify it fails**

Run: `go test ./internal/tandemonium/tui -run TestApproveAdapterUsesProvidedDB`

Expected: FAIL with compile error (`ApproveAdapter` missing DB/Runner fields).

---

### Task 2: Implement shared DB in the TUI model and execute command

**Files:**
- Modify: `internal/tandemonium/tui/model.go`
- Modify: `internal/tandemonium/cli/commands/execute.go`

**Step 1: Write minimal implementation**

`internal/tandemonium/tui/model.go` (add DB field and constructor):

```go
import "database/sql"

// in Model struct
DB *sql.DB

func NewModelWithDB(db *sql.DB) Model {
    m := NewModel()
    m.DB = db
    if db != nil {
        m.ReviewLoader = func() ([]string, error) { return LoadReviewQueue(db) }
        m.TaskLoader = func() ([]TaskItem, error) { return LoadTasks(db) }
        m.CoordInboxLoader = func(recipient string, limit int, urgent bool) ([]storage.MessageDelivery, error) {
            return LoadCoordInbox(db, recipient, limit, urgent)
        }
        m.CoordLocksLoader = func(limit int) ([]storage.Reservation, error) {
            return LoadCoordLocks(db, limit)
        }
        m.Approver = &ApproveAdapter{DB: db, Runner: &git.ExecRunner{}}
    }
    return m
}
```

`internal/tandemonium/cli/commands/execute.go` (open DB once, migrate, pass into model):

```go
root, err := project.FindRoot(".")
if err != nil {
    return err
}

db, err := storage.OpenShared(project.StateDBPath(root))
if err != nil {
    return err
}
defer db.Close()

if err := storage.Migrate(db); err != nil {
    return err
}

m := tui.NewModelWithDB(db)
```

**Step 2: Run tests to verify they pass**

Run: `go test ./internal/tandemonium/tui -run TestNewModelWithDBUsesDBLoaders`

Expected: PASS.

---

### Task 3: Update ApproveAdapter to use provided DB + runner

**Files:**
- Modify: `internal/tandemonium/tui/approve_adapter.go`

**Step 1: Write minimal implementation**

```go
package tui

import (
    "database/sql"

    "github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/git"
    "github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
    "github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

type ApproveAdapter struct {
    DB     *sql.DB
    Runner git.Runner
}

func (a *ApproveAdapter) Approve(taskID, branch string) error {
    runner := a.Runner
    if runner == nil {
        runner = &git.ExecRunner{}
    }
    if err := git.MergeBranch(runner, branch); err != nil {
        return err
    }
    db := a.DB
    if db == nil {
        root, err := project.FindRoot(".")
        if err != nil {
            return err
        }
        db, err = storage.OpenShared(project.StateDBPath(root))
        if err != nil {
            return err
        }
    }
    return storage.ApproveTask(db, taskID)
}
```

**Step 2: Run tests to verify they pass**

Run: `go test ./internal/tandemonium/tui -run TestApproveAdapterUsesProvidedDB`

Expected: PASS.

---

### Task 4: Run targeted suite and update todo

**Files:**
- Modify: `docs/tandemonium/todos/004-pending-p1-repeated-database-connections.md`

**Step 1: Run relevant tests**

Run: `go test ./internal/tandemonium/tui`

Expected: PASS.

**Step 2: Update todo**

Mark todo as done and note that execute opens + migrates DB once and model uses shared DB loaders.

**Step 3: Commit**

```bash
git add internal/tandemonium/tui/model.go internal/tandemonium/cli/commands/execute.go internal/tandemonium/tui/approve_adapter.go internal/tandemonium/tui/model_db_test.go internal/tandemonium/tui/approve_adapter_test.go docs/tandemonium/todos/004-pending-p1-repeated-database-connections.md
git commit -m "refactor(tandemonium): reuse shared DB in TUI"
```

---

Plan complete and saved to `docs/plans/2026-01-22-tandemonium-tui-db-reuse-plan.md`.

Two execution options:

1. Subagent-Driven (this session) — I dispatch a fresh subagent per task, review between tasks
2. Parallel Session (separate) — Open a new session with executing-plans and batch execution

Which approach?
