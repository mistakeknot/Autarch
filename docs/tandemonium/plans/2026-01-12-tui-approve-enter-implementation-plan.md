# TUI Approve on Enter Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Approve the currently selected review item when pressing Enter, while keeping `a` as a shortcut.

**Architecture:** Use the existing selection index to determine the active review item, route both Enter and `a` to the same approve path, and clamp selection when the queue changes.

**Tech Stack:** Go 1.24+, Bubble Tea.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Approve selected item on Enter

**Files:**
- Modify: `internal/tui/approve_key_test.go`
- Modify: `internal/tui/model.go`

**Step 1: Write failing test**

```go
func TestApproveKeyUsesSelectedItem(t *testing.T) {
	m := NewModel()
	m.ReviewQueue = []string{"T1", "T2"}
	m.SelectedReview = 1
	m.BranchLookup = func(taskID string) (string, error) {
		return "feature/" + taskID, nil
	}
	m.ReviewLoader = func() ([]string, error) {
		return []string{}, nil
	}
	fake := &fakeKeyApprover{}
	m.Approver = fake

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !fake.called {
		t.Fatal("expected approve call")
	}
	if fake.taskID != "T2" {
		t.Fatalf("expected selected task, got %q", fake.taskID)
	}
	if fake.branch != "feature/T2" {
		t.Fatalf("expected selected branch, got %q", fake.branch)
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL with "expected selected task"

**Step 3: Implement selected approve + Enter**

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			// existing selection logic
		case "k", "up":
			// existing selection logic
		case "enter", "a":
			if len(m.ReviewQueue) > 0 {
				idx := m.SelectedReview
				if idx < 0 || idx >= len(m.ReviewQueue) {
					idx = 0
				}
				taskID := m.ReviewQueue[idx]
				// existing approve path using taskID
			}
		}
		// remove the old `a`-only branch below
	}
	return m, nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/approve_key_test.go
git commit -m "feat: approve selected review on enter"
```

---

### Task 2: Clamp selection after review refresh

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/review_select_test.go`

**Step 1: Write failing test**

```go
package tui

import (
	"testing"
)

func TestClampSelectionAfterRefresh(t *testing.T) {
	m := NewModel()
	m.ReviewQueue = []string{"T1", "T2"}
	m.SelectedReview = 1
	m.ReviewQueue = []string{"T1"}
	m.ClampReviewSelection()
	if m.SelectedReview != 0 {
		t.Fatalf("expected selection 0, got %d", m.SelectedReview)
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL with "undefined: ClampReviewSelection"

**Step 3: Implement clamp helper**

```go
func (m *Model) ClampReviewSelection() {
	if len(m.ReviewQueue) == 0 {
		m.SelectedReview = 0
		return
	}
	if m.SelectedReview < 0 {
		m.SelectedReview = 0
		return
	}
	if m.SelectedReview >= len(m.ReviewQueue) {
		m.SelectedReview = len(m.ReviewQueue) - 1
	}
}
```

Call it after review refresh:

```go
m.ReviewQueue = queue
m.ClampReviewSelection()
m.RefreshReviewBranches()
```

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/review_select_test.go
git commit -m "feat: clamp review selection after refresh"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
