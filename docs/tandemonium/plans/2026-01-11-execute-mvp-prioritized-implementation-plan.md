# Execute-Only MVP Prioritized Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Complete the execute-only MVP by wiring tmux session orchestration, log streaming, task lifecycle, review workflow, and drift checks.

**Architecture:** Build a thin execution layer that persists tasks in SQLite, creates worktrees, launches tmux sessions, streams output to logs, and detects completion/blockers. TUI renders sessions + review queue. Spec YAMLs provide file expectations and metadata for drift checks.

**Tech Stack:** Go 1.24+, Bubble Tea, SQLite (modernc.org/sqlite), tmux, git.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

## Priority 1: Task lifecycle + tmux orchestration

### Task 1: Task model + CRUD in SQLite

**Files:**
- Modify: `internal/storage/task.go`
- Create: `internal/storage/task_test.go`

**Step 1: Write the failing test**

```go
package storage

import "testing"

func TestUpdateTaskStatus(t *testing.T) {
    db, _ := OpenTemp()
    defer db.Close()
    _ = Migrate(db)

    _ = InsertTask(db, Task{ID: "TAND-001", Title: "Test", Status: "todo"})
    if err := UpdateTaskStatus(db, "TAND-001", "in_progress"); err != nil {
        t.Fatal(err)
    }
    got, _ := GetTask(db, "TAND-001")
    if got.Status != "in_progress" {
        t.Fatalf("expected in_progress, got %s", got.Status)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -v`
Expected: FAIL with "undefined: UpdateTaskStatus"

**Step 3: Implement minimal CRUD**

```go
func UpdateTaskStatus(db *sql.DB, id, status string) error {
    _, err := db.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, status, id)
    return err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/storage/task.go internal/storage/task_test.go
git commit -m "feat: add task status update"
```

---

### Task 2: Worktree + tmux launch service

**Files:**
- Create: `internal/agent/launcher.go`
- Create: `internal/agent/launcher_test.go`

**Step 1: Write the failing test**

```go
package agent

import "testing"

func TestSessionIDFormat(t *testing.T) {
    id := SessionID("TAND-001")
    if id != "tand-TAND-001" {
        t.Fatalf("unexpected id: %s", id)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent -v`
Expected: FAIL with "undefined: SessionID"

**Step 3: Implement launcher skeleton**

```go
func SessionID(taskID string) string { return "tand-" + taskID }
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/launcher.go internal/agent/launcher_test.go
git commit -m "feat: add session id helper"
```

---

### Task 3: Pipe-pane log writer + tail reader integration

**Files:**
- Modify: `internal/tmux/session.go`
- Create: `internal/tmux/session_integration_test.go`

**Step 1: Write the failing test**

```go
package tmux

import "testing"

func TestPipePaneCommand(t *testing.T) {
    r := &fakeRunner{}
    s := Session{ID: "tand-TAND-001", Workdir: "/tmp/x", LogPath: "/tmp/log"}
    _ = StartSession(r, s)
    found := false
    for _, cmd := range r.cmds {
        if len(cmd) >= 3 && cmd[1] == "pipe-pane" {
            found = true
        }
    }
    if !found { t.Fatal("expected pipe-pane") }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tmux -v`
Expected: FAIL (update test if needed)

**Step 3: Adjust StartSession if needed**

Ensure `pipe-pane` command includes `-o` and appends to log file.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tmux -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tmux/session.go internal/tmux/session_integration_test.go
git commit -m "feat: ensure pipe-pane logging"
```

---

## Priority 2: Output detection + review queue wiring

### Task 4: Detection pipeline integration

**Files:**
- Modify: `internal/agent/detect.go`
- Create: `internal/agent/detect_test.go`

**Step 1: Write failing test**

```go
func TestDetectBlocked(t *testing.T) {
    state := DetectState("Blocked: waiting on user")
    if state != "blocked" { t.Fatal("expected blocked") }
}
```

**Step 2: Run test**

Run: `go test ./internal/agent -v`
Expected: FAIL if missing

**Step 3: Update detection**

Add additional keywords or regexes as needed.

**Step 4: Run test**

Run: `go test ./internal/agent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/detect.go internal/agent/detect_test.go
git commit -m "feat: expand detection keywords"
```

---

### Task 5: Review queue persistence in SQLite

**Files:**
- Modify: `internal/storage/db.go`
- Create: `internal/storage/review.go`
- Create: `internal/storage/review_test.go`

**Step 1: Write failing test**

```go
func TestReviewQueueAdd(t *testing.T) {
    db, _ := OpenTemp()
    defer db.Close()
    _ = Migrate(db)
    _ = AddToReviewQueue(db, "TAND-001")
    ids, _ := ListReviewQueue(db)
    if len(ids) != 1 { t.Fatal("expected 1") }
}
```

**Step 2: Run test**

Run: `go test ./internal/storage -v`
Expected: FAIL

**Step 3: Implement queue table + helpers**

Add `review_queue` table and CRUD helpers.

**Step 4: Run test**

Run: `go test ./internal/storage -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/storage/db.go internal/storage/review.go internal/storage/review_test.go
git commit -m "feat: add review queue storage"
```

---

## Priority 3: TUI integration (minimal)

### Task 6: Fleet view uses task/session data

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/model_test.go`

**Step 1: Write failing test**

```go
func TestModelHasSessions(t *testing.T) {
    m := NewModel()
    if m.Sessions == nil { t.Fatal("expected sessions slice") }
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL

**Step 3: Implement minimal struct fields**

Add `Sessions []string` to model and wire a stub load.

**Step 4: Run test**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: add sessions to TUI model"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
