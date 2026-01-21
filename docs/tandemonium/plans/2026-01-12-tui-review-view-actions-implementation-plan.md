# TUI Review View Actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Implement the full Review View with action keys: diff, approve, feedback, reject, and edit story.

**Architecture:** Add a review-focused view mode with simple text rendering. Implement feedback and story edits by writing to spec YAML. Reject updates task status and removes from the review queue. Approve uses the existing approve path.

**Tech Stack:** Go 1.24+, Bubble Tea, YAML (gopkg.in/yaml.v3), SQLite.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Review view mode + action line

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/view_test.go`
- Create: `internal/tui/review_view_test.go`

**Step 1: Write failing tests**

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEnterReviewView(t *testing.T) {
	m := NewModel()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	updated := next.(Model)
	if updated.ViewMode != ViewReview {
		t.Fatalf("expected review view")
	}
}
```

```go
func TestReviewViewIncludesActions(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	out := m.View()
	if !strings.Contains(out, "[d]iff") || !strings.Contains(out, "[a]pprove") {
		t.Fatalf("expected review actions, got %q", out)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui -v`
Expected: FAIL with "ViewMode undefined"

**Step 3: Implement review view mode**

```go
type ViewMode string

const (
	ViewFleet  ViewMode = "fleet"
	ViewReview ViewMode = "review"
)

type Model struct {
	// ...
	ViewMode        ViewMode
	ReviewShowDiffs bool
}

func NewModel() Model {
	return Model{
		// ...
		ViewMode: ViewFleet,
	}
}

// Update: handle "R" to enter review view and "b" to go back.
```

In `View()`, if `ViewMode == ViewReview`, render a simple review layout:

```go
out := "REVIEW\n"
out += "\n[d]iff  [a]pprove  [f]eedback  [r]eject  [e]dit story  [b]ack\n"
```

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/view_test.go internal/tui/review_view_test.go
git commit -m "feat: add review view mode"
```

---

### Task 2: Spec YAML helpers for feedback + story

**Files:**
- Create: `internal/specs/review.go`
- Create: `internal/specs/review_test.go`

**Step 1: Write failing tests**

```go
package specs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateUserStory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	if err := os.WriteFile(path, []byte("id: T1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateUserStory(path, "New story"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "user_story") {
		t.Fatal("expected user_story in yaml")
	}
}

func TestAppendReviewFeedback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	if err := os.WriteFile(path, []byte("id: T1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendReviewFeedback(path, "Needs tests"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "review_feedback") {
		t.Fatal("expected review_feedback in yaml")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/specs -v`
Expected: FAIL with "undefined: UpdateUserStory"

**Step 3: Implement helpers**

```go
func UpdateUserStory(path, text string) error { /* load yaml into map, set user_story.text */ }
func AppendReviewFeedback(path, text string) error { /* append to review_feedback list */ }
```

Use `yaml.v3` to unmarshal into `map[string]interface{}` and marshal back.

**Step 4: Run tests**

Run: `go test ./internal/specs -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/review.go internal/specs/review_test.go
git commit -m "feat: add review yaml helpers"
```

---

### Task 3: Wire review actions (feedback/reject/edit)

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/approve_key_test.go`
- Create: `internal/tui/review_action_test.go`
- Modify: `internal/storage/review.go`
- Create: `internal/storage/reject_test.go`

**Step 1: Write failing tests**

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestReviewRejectRequiresFeedback(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewQueue = []string{"T1"}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	updated := next.(Model)
	if updated.ReviewInputMode != ReviewInputFeedback {
		t.Fatalf("expected feedback mode")
	}
}
```

```go
package storage

func TestRejectTask(t *testing.T) {
	db, _ := OpenTemp()
	_ = Migrate(db)
	_ = InsertTask(db, Task{ID: "T1", Title: "Test", Status: "review"})
	_ = AddToReviewQueue(db, "T1")
	if err := RejectTask(db, "T1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ids, _ := ListReviewQueue(db)
	if len(ids) != 0 {
		t.Fatal("expected review queue cleared")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui -v`
Expected: FAIL with "ReviewInputMode undefined"

Run: `go test ./internal/storage -v`
Expected: FAIL with "undefined: RejectTask"

**Step 3: Implement actions**

In `model.go`, add:

```go
type ReviewInputMode string
const (
	ReviewInputNone     ReviewInputMode = "none"
	ReviewInputFeedback ReviewInputMode = "feedback"
	ReviewInputStory    ReviewInputMode = "story"
)

type Model struct {
	// ...
	ReviewInputMode ReviewInputMode
	ReviewInput     string
}
```

Handle `f`, `r`, `e` in review view:
- `f`: set input mode feedback.
- `r`: set input mode feedback + mark pending reject.
- `e`: set input mode story.

Capture runes for input when `ReviewInputMode != none` and apply on `enter`:
- feedback: call `specs.AppendReviewFeedback`.
- reject: call `specs.AppendReviewFeedback` + `storage.RejectTask`.
- edit story: call `specs.UpdateUserStory`.

Use helper to resolve spec path: `filepath.Join(project.SpecsDir(root), taskID+".yaml")`.

Add `RejectTask` in `internal/storage/review.go`:

```go
func RejectTask(db *sql.DB, id string) error {
	if err := UpdateTaskStatus(db, id, "rejected"); err != nil { return err }
	return RemoveFromReviewQueue(db, id)
}
```

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

Run: `go test ./internal/storage -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/review_action_test.go internal/tui/approve_key_test.go internal/storage/review.go internal/storage/reject_test.go
git commit -m "feat: wire review actions"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
