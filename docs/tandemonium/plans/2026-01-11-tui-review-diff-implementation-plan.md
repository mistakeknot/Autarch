# TUI Review Diff Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Render a minimal review queue and per-task diff list in the TUI.

**Architecture:** Extend the TUI model to hold review items and diff file lists, add helper to load review queue from SQLite, and render a basic list view. Keep UI text-only (no color/formatting requirements).

**Tech Stack:** Go 1.24+, Bubble Tea, SQLite, git CLI.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Load review queue from SQLite

**Files:**
- Create: `internal/tui/review_loader.go`
- Create: `internal/tui/review_loader_test.go`

**Step 1: Write failing test**

```go
package tui

import (
    "testing"

    "github.com/gensysven/tandemonium/internal/storage"
)

func TestLoadReviewQueue(t *testing.T) {
    db, _ := storage.OpenTemp()
    defer db.Close()
    _ = storage.Migrate(db)
    _ = storage.AddToReviewQueue(db, "TAND-001")

    ids, err := LoadReviewQueue(db)
    if err != nil || len(ids) != 1 {
        t.Fatal("expected one review item")
    }
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL

**Step 3: Implement loader**

```go
func LoadReviewQueue(db *sql.DB) ([]string, error) {
    return storage.ListReviewQueue(db)
}
```

**Step 4: Run test**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/review_loader.go internal/tui/review_loader_test.go
git commit -m "feat: load review queue for TUI"
```

---

### Task 2: Add diff file list to TUI model

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/diff_test.go`

**Step 1: Write failing test**

```go
package tui

import "testing"

func TestModelHasDiffFiles(t *testing.T) {
    m := NewModel()
    if m.DiffFiles == nil {
        t.Fatal("expected diff files")
    }
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL

**Step 3: Add field + init**

Add `DiffFiles []string` to model and initialize to empty.

**Step 4: Run test**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/diff_test.go
git commit -m "feat: add diff files to TUI model"
```

---

### Task 3: Render review queue + diffs in View()

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/view_test.go`

**Step 1: Write failing test**

```go
package tui

import "strings"
import "testing"

func TestViewIncludesReviewHeader(t *testing.T) {
    m := NewModel()
    out := m.View()
    if !strings.Contains(out, "REVIEW QUEUE") {
        t.Fatal("expected review header")
    }
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL

**Step 3: Implement minimal rendering**

Append to View():

```
REVIEW QUEUE
- <id>

DIFF FILES
- <file>
```

**Step 4: Run test**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/view_test.go
git commit -m "feat: render review queue in TUI"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
