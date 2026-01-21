# TUI Review Branches + Navigation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Precompute branch names when loading the review queue, show them in the TUI list, and add keyboard navigation through review items.

**Architecture:** Extend the TUI model with a review-branch cache and selection index. When the review queue is loaded/refreshed, compute branch names using the existing branch lookup helper and store them in the model. Render review items with branch labels and a selection marker, and handle `j/k` plus arrow keys to move selection.

**Tech Stack:** Go 1.24+, Bubble Tea, git CLI.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Precompute review branches + display in view

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/approve_key_test.go`
- Create: `internal/tui/review_branch_test.go`

**Step 1: Write failing test**

```go
package tui

import "testing"

func TestReviewBranchesShownInView(t *testing.T) {
	m := NewModel()
	m.ReviewQueue = []string{"TAND-001"}
	m.ReviewBranches = map[string]string{"TAND-001": "feature/TAND-001"}
	view := m.View()
	if !strings.Contains(view, "TAND-001 (feature/TAND-001)") {
		t.Fatalf("expected branch label in view, got %q", view)
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL with "m.ReviewBranches undefined"

**Step 3: Implement review branch cache + view**

```go
type Model struct {
	// ...
	ReviewBranches map[string]string
}

func (m *Model) RefreshReviewBranches() {
	if m.BranchLookup == nil || len(m.ReviewQueue) == 0 {
		m.ReviewBranches = map[string]string{}
		return
	}
	branches := map[string]string{}
	for _, id := range m.ReviewQueue {
		if branch, err := m.BranchLookup(id); err == nil {
			branches[id] = branch
		}
	}
	m.ReviewBranches = branches
}

func (m Model) View() string {
	// ...
	for _, id := range m.ReviewQueue {
		label := id
		if branch, ok := m.ReviewBranches[id]; ok {
			label = id + " (" + branch + ")"
		}
		out += "- " + label + "\n"
	}
	// ...
}
```

Update approve refresh path in `Update` to call `m.RefreshReviewBranches()` after `m.ReviewQueue = queue`.

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/review_branch_test.go internal/tui/approve_key_test.go
git commit -m "feat: show review branches in TUI list"
```

---

### Task 2: Add review list navigation keys

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/review_nav_test.go`

**Step 1: Write failing test**

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestReviewSelectionMovesDown(t *testing.T) {
	m := NewModel()
	m.ReviewQueue = []string{"T1", "T2"}
	m.SelectedReview = 0

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	updated := next.(Model)
	if updated.SelectedReview != 1 {
		t.Fatalf("expected selection 1, got %d", updated.SelectedReview)
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL with "m.SelectedReview undefined"

**Step 3: Implement selection + key handling**

```go
type Model struct {
	// ...
	SelectedReview int
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.ReviewQueue) > 0 && m.SelectedReview < len(m.ReviewQueue)-1 {
				m.SelectedReview++
			}
		case "k", "up":
			if m.SelectedReview > 0 {
				m.SelectedReview--
			}
		}
		// existing approve handling...
	}
	return m, nil
}

func (m Model) View() string {
	for i, id := range m.ReviewQueue {
		prefix := "- "
		if i == m.SelectedReview {
			prefix = "> "
		}
		// ...
		out += prefix + label + "\n"
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/review_nav_test.go
git commit -m "feat: add review navigation keys"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
