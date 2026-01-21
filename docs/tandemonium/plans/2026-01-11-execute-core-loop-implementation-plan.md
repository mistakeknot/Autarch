# Execute Core Loop Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Implement the core execute loop: session state persistence, tmux lifecycle controls, log offset reading, and a minimal task start workflow that ties storage + tmux together.

**Architecture:** Add a `sessions` table in SQLite to track tmux sessions and offsets, extend tmux helpers with stop + log offset reading, and add an `agent` workflow service with injected interfaces for worktree creation and tmux session start. Keep everything testable with fakes; no destructive actions in tests.

**Tech Stack:** Go 1.24+, SQLite (modernc.org/sqlite), tmux, git.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Session state persistence in SQLite

**Files:**
- Modify: `internal/storage/db.go`
- Create: `internal/storage/session.go`
- Create: `internal/storage/session_test.go`

**Step 1: Write the failing test**

```go
package storage

import "testing"

func TestSessionCRUD(t *testing.T) {
    db, _ := OpenTemp()
    defer db.Close()
    _ = Migrate(db)

    s := Session{ID: "tand-TAND-001", TaskID: "TAND-001", State: "working", Offset: 10}
    if err := InsertSession(db, s); err != nil {
        t.Fatal(err)
    }
    if err := UpdateSessionOffset(db, s.ID, 42); err != nil {
        t.Fatal(err)
    }
    got, err := GetSession(db, s.ID)
    if err != nil {
        t.Fatal(err)
    }
    if got.Offset != 42 {
        t.Fatalf("expected offset 42, got %d", got.Offset)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -v`
Expected: FAIL with "undefined: InsertSession"

**Step 3: Implement minimal session storage**

Add `sessions` table in `Migrate`:

```go
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  state TEXT NOT NULL,
  offset INTEGER NOT NULL DEFAULT 0
);
```

Create `internal/storage/session.go`:

```go
type Session struct {
    ID     string
    TaskID string
    State  string
    Offset int64
}

func InsertSession(db *sql.DB, s Session) error {
    _, err := db.Exec(`INSERT INTO sessions (id, task_id, state, offset) VALUES (?, ?, ?, ?)`, s.ID, s.TaskID, s.State, s.Offset)
    return err
}

func UpdateSessionOffset(db *sql.DB, id string, offset int64) error {
    _, err := db.Exec(`UPDATE sessions SET offset = ? WHERE id = ?`, offset, id)
    return err
}

func UpdateSessionState(db *sql.DB, id, state string) error {
    _, err := db.Exec(`UPDATE sessions SET state = ? WHERE id = ?`, state, id)
    return err
}

func GetSession(db *sql.DB, id string) (Session, error) {
    row := db.QueryRow(`SELECT id, task_id, state, offset FROM sessions WHERE id = ?`, id)
    var s Session
    if err := row.Scan(&s.ID, &s.TaskID, &s.State, &s.Offset); err != nil {
        return Session{}, err
    }
    return s, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/storage/db.go internal/storage/session.go internal/storage/session_test.go
git commit -m "feat: add session storage"
```

---

### Task 2: tmux stop + log offset reader

**Files:**
- Modify: `internal/tmux/session.go`
- Modify: `internal/tmux/stream.go`
- Create: `internal/tmux/stream_offset_test.go`
- Create: `internal/tmux/session_stop_test.go`

**Step 1: Write the failing tests**

```go
package tmux

import "testing"

func TestStopSessionBuildsCommand(t *testing.T) {
    r := &fakeRunner{}
    if err := StopSession(r, "tand-TAND-001"); err != nil {
        t.Fatal(err)
    }
    if len(r.cmds) == 0 || r.cmds[0][1] != "kill-session" {
        t.Fatal("expected kill-session")
    }
}
```

```go
package tmux

import (
    "os"
    "testing"
)

func TestReadFromOffset(t *testing.T) {
    f, _ := os.CreateTemp("", "tand-log-*")
    defer os.Remove(f.Name())
    _, _ = f.WriteString("one\n")
    _ = f.Sync()

    lines, next, err := ReadFromOffset(f.Name(), 0)
    if err != nil || len(lines) != 1 || next == 0 {
        t.Fatal("expected one line and advanced offset")
    }
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tmux -v`
Expected: FAIL with "undefined: StopSession" / "ReadFromOffset"

**Step 3: Implement minimal helpers**

```go
func StopSession(r Runner, id string) error {
    return r.Run("tmux", "kill-session", "-t", id)
}
```

```go
func ReadFromOffset(path string, offset int64) ([]string, int64, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, offset, err
    }
    defer f.Close()
    if _, err := f.Seek(offset, 0); err != nil {
        return nil, offset, err
    }
    data, err := io.ReadAll(f)
    if err != nil {
        return nil, offset, err
    }
    newOffset := offset + int64(len(data))
    lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
    if len(lines) == 1 && lines[0] == "" {
        return []string{}, newOffset, nil
    }
    return lines, newOffset, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tmux -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tmux/session.go internal/tmux/stream.go internal/tmux/stream_offset_test.go internal/tmux/session_stop_test.go
git commit -m "feat: add tmux stop and log offset reader"
```

---

### Task 3: Agent start workflow (no real git/tmux in tests)

**Files:**
- Create: `internal/agent/workflow.go`
- Create: `internal/agent/workflow_test.go`

**Step 1: Write the failing test**

```go
package agent

import "testing"

type fakeWorktree struct{ called bool }
func (f *fakeWorktree) Create(repo, path, branch string) error { f.called = true; return nil }

type fakeSession struct{ called bool }
func (f *fakeSession) Start(id, workdir, logPath string) error { f.called = true; return nil }

func TestStartTaskWorkflow(t *testing.T) {
    w := &fakeWorktree{}
    s := &fakeSession{}
    if err := StartTask(w, s, "TAND-001", "/repo", "/wt", "/log"); err != nil {
        t.Fatal(err)
    }
    if !w.called || !s.called {
        t.Fatal("expected worktree + session start")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent -v`
Expected: FAIL with "undefined: StartTask"

**Step 3: Implement minimal workflow**

```go
type WorktreeCreator interface { Create(repo, path, branch string) error }
type SessionStarter interface { Start(id, workdir, logPath string) error }

func StartTask(w WorktreeCreator, s SessionStarter, taskID, repo, worktree, logPath string) error {
    if err := w.Create(repo, worktree, "feature/"+taskID); err != nil {
        return err
    }
    return s.Start(SessionID(taskID), worktree, logPath)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/workflow.go internal/agent/workflow_test.go
git commit -m "feat: add start task workflow"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
