# Review Workflow Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Add the review workflow: queue completed tasks, show basic diff info, and provide approve/merge plumbing.

**Architecture:** Extend the detection apply logic to enqueue completed tasks in SQLite. Add a small git diff helper that returns changed files for a worktree branch. Keep TUI changes minimal (placeholder rendering of review items).

**Tech Stack:** Go 1.24+, git CLI, SQLite.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Enqueue review on completion

**Files:**
- Modify: `internal/agent/loop.go`
- Create: `internal/agent/loop_review_test.go`

**Step 1: Write the failing test**

```go
package agent

import "testing"

type fakeReviewStore struct{ enqueued bool }

func (f *fakeReviewStore) UpdateSessionState(id, state string) error { return nil }
func (f *fakeReviewStore) UpdateTaskStatus(id, status string) error { return nil }
func (f *fakeReviewStore) EnqueueReview(id string) error { f.enqueued = true; return nil }

func TestApplyDetectionEnqueuesOnDone(t *testing.T) {
    fs := &fakeReviewStore{}
    _ = ApplyDetection(fs, "TAND-001", "tand-TAND-001", "done")
    if !fs.enqueued { t.Fatal("expected enqueue") }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent -v`
Expected: FAIL with "missing EnqueueReview" in interface

**Step 3: Update interface + implementation**

Extend `StatusStore` to include `EnqueueReview(id string) error` and call it when state == "done".

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/loop.go internal/agent/loop_review_test.go
git commit -m "feat: enqueue review on completion"
```

---

### Task 2: Review queue helpers in storage

**Files:**
- Modify: `internal/storage/review.go`
- Create: `internal/storage/review_queue_test.go`

**Step 1: Write the failing test**

```go
package storage

import "testing"

func TestRemoveFromReviewQueue(t *testing.T) {
    db, _ := OpenTemp()
    defer db.Close()
    _ = Migrate(db)
    _ = AddToReviewQueue(db, "TAND-001")
    _ = RemoveFromReviewQueue(db, "TAND-001")
    ids, _ := ListReviewQueue(db)
    if len(ids) != 0 { t.Fatal("expected empty") }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -v`
Expected: FAIL with "undefined: RemoveFromReviewQueue"

**Step 3: Implement remove helper**

```go
func RemoveFromReviewQueue(db *sql.DB, taskID string) error {
    _, err := db.Exec(`DELETE FROM review_queue WHERE task_id = ?`, taskID)
    return err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/storage/review.go internal/storage/review_queue_test.go
git commit -m "feat: add review queue remove"
```

---

### Task 3: Git diff helper (list changed files)

**Files:**
- Create: `internal/git/diff.go`
- Create: `internal/git/diff_test.go`

**Step 1: Write the failing test**

```go
package git

import "testing"

func TestParseNameOnly(t *testing.T) {
    out := "a.txt\nb.txt\n"
    files := ParseNameOnly(out)
    if len(files) != 2 { t.Fatal("expected 2") }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git -v`
Expected: FAIL with "undefined: ParseNameOnly"

**Step 3: Implement helper**

```go
func ParseNameOnly(output string) []string {
    // split lines, trim blanks
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/git -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/diff.go internal/git/diff_test.go
git commit -m "feat: add git diff name-only helper"
```

---

### Task 4: TUI review list stub

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/review_test.go`

**Step 1: Write the failing test**

```go
package tui

import "testing"

func TestModelHasReviewQueue(t *testing.T) {
    m := NewModel()
    if m.ReviewQueue == nil { t.Fatal("expected review queue") }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -v`
Expected: FAIL

**Step 3: Implement stub field**

Add `ReviewQueue []string` to model.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/review_test.go
git commit -m "feat: add review queue to TUI model"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
