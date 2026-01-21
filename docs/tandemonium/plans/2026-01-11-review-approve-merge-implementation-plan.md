# Review Approve + Merge Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Implement the approve/merge path: show review items, merge the task branch, update status, and remove from review queue.

**Architecture:** Add a git merge helper, a storage update helper, and a minimal CLI command that simulates approve for now (TUI wiring later). Keep side effects explicit and minimal.

**Tech Stack:** Go 1.24+, git CLI, SQLite.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Git merge helper

**Files:**
- Create: `internal/git/merge.go`
- Create: `internal/git/merge_test.go`

**Step 1: Write failing test**

```go
package git

import "testing"

type fakeRunner struct{ args [][]string }

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
    f.args = append(f.args, append([]string{name}, args...))
    return "", nil
}

func TestMergeBranch(t *testing.T) {
    r := &fakeRunner{}
    _ = MergeBranch(r, "feature/TAND-001")
    if len(r.args) == 0 || r.args[0][1] != "merge" {
        t.Fatal("expected git merge")
    }
}
```

**Step 2: Run test**

Run: `go test ./internal/git -v`
Expected: FAIL with "undefined: MergeBranch"

**Step 3: Implement helper**

```go
func MergeBranch(r Runner, branch string) error {
    _, err := r.Run("git", "merge", branch)
    return err
}
```

**Step 4: Run test**

Run: `go test ./internal/git -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/merge.go internal/git/merge_test.go
git commit -m "feat: add git merge helper"
```

---

### Task 2: Storage helper to mark done + dequeue

**Files:**
- Modify: `internal/storage/task.go`
- Modify: `internal/storage/review.go`
- Create: `internal/storage/approve_test.go`

**Step 1: Write failing test**

```go
package storage

import "testing"

func TestApproveTask(t *testing.T) {
    db, _ := OpenTemp()
    defer db.Close()
    _ = Migrate(db)
    _ = InsertTask(db, Task{ID: "TAND-001", Title: "Test", Status: "review"})
    _ = AddToReviewQueue(db, "TAND-001")
    _ = ApproveTask(db, "TAND-001")
    tsk, _ := GetTask(db, "TAND-001")
    if tsk.Status != "done" { t.Fatal("expected done") }
    ids, _ := ListReviewQueue(db)
    if len(ids) != 0 { t.Fatal("expected queue empty") }
}
```

**Step 2: Run test**

Run: `go test ./internal/storage -v`
Expected: FAIL with "undefined: ApproveTask"

**Step 3: Implement helper**

```go
func ApproveTask(db *sql.DB, id string) error {
    if err := UpdateTaskStatus(db, id, "done"); err != nil { return err }
    return RemoveFromReviewQueue(db, id)
}
```

**Step 4: Run test**

Run: `go test ./internal/storage -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/storage/task.go internal/storage/review.go internal/storage/approve_test.go
git commit -m "feat: add approve task helper"
```

---

### Task 3: CLI approve command (placeholder for TUI)

**Files:**
- Create: `internal/cli/commands/approve.go`
- Modify: `internal/cli/root.go`
- Create: `internal/cli/commands/approve_test.go`

**Step 1: Write failing test**

```go
package commands

import "testing"

func TestApproveCmd(t *testing.T) {
    if ApproveCmd().Use != "approve" {
        t.Fatal("expected approve")
    }
}
```

**Step 2: Run test**

Run: `go test ./internal/cli/commands -v`
Expected: FAIL

**Step 3: Implement command**

Command signature: `tandemonium approve <task-id> <branch>`

- Run `git merge <branch>`
- Update task status to done + remove from review queue
- Print summary

**Step 4: Run test**

Run: `go test ./internal/cli/commands -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cli/commands/approve.go internal/cli/commands/approve_test.go internal/cli/root.go
git commit -m "feat: add approve command"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
