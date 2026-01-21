# TUI Status Line and Review Refresh Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Add a TUI status line that surfaces approve errors and refresh the review queue after successful approval.

**Architecture:** Extend the TUI model with status fields and helper setters, render a status line in the view, and update the approve key path to report errors and refresh the review queue using a loader injectable for tests.

**Tech Stack:** Go 1.24+, Bubble Tea, SQLite.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Add status line fields + view rendering

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/status_test.go`

**Step 1: Write failing tests**

```go
package tui

import "testing"

func TestViewIncludesStatusLine(t *testing.T) {
	m := NewModel()
	m.Status = "ready"
	m.StatusLevel = StatusInfo
	view := m.View()
	if !strings.Contains(view, "STATUS: ready") {
		t.Fatalf("expected status line, got %q", view)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui -v`
Expected: FAIL with "undefined: StatusInfo"

**Step 3: Implement status fields + render**

```go
type StatusLevel string

const (
	StatusInfo  StatusLevel = "info"
	StatusError StatusLevel = "error"
)

type Model struct {
	// ...
	Status      string
	StatusLevel StatusLevel
}

func NewModel() Model {
	return Model{
		Title: "Tandemonium",
		// ...
		Status:      "ready",
		StatusLevel: StatusInfo,
	}
}

func (m *Model) SetStatus(level StatusLevel, message string) {
	m.StatusLevel = level
	m.Status = message
}

func (m *Model) SetStatusError(message string) {
	m.SetStatus(StatusError, message)
}

func (m *Model) SetStatusInfo(message string) {
	m.SetStatus(StatusInfo, message)
}

func (m Model) View() string {
	// ...
	if m.StatusLevel == StatusError {
		out += "\nERROR: " + m.Status + "\n"
	} else {
		out += "\nSTATUS: " + m.Status + "\n"
	}
	return out
}
```

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/status_test.go
git commit -m "feat: add TUI status line"
```

---

### Task 2: Report approve errors + refresh review queue

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/approve_key_test.go`
- Create: `internal/tui/review_refresh.go`

**Step 1: Write failing tests**

```go
func TestApproveKeyRefreshesReviewQueue(t *testing.T) {
	m := NewModel()
	m.ReviewQueue = []string{"TAND-001"}
	m.BranchLookup = func(taskID string) (string, error) {
		return "feature/TAND-001", nil
	}
	m.ReviewLoader = func() ([]string, error) {
		return []string{}, nil
	}
	fake := &fakeKeyApprover{}
	m.Approver = fake

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	updated := next.(Model)

	if len(updated.ReviewQueue) != 0 {
		t.Fatalf("expected review queue to refresh, got %v", updated.ReviewQueue)
	}
	if updated.StatusLevel != StatusInfo {
		t.Fatalf("expected status info, got %v", updated.StatusLevel)
	}
}

func TestApproveKeySetsErrorStatus(t *testing.T) {
	m := NewModel()
	m.ReviewQueue = []string{"TAND-001"}
	m.BranchLookup = func(taskID string) (string, error) {
		return "", errors.New("boom")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	updated := next.(Model)

	if updated.StatusLevel != StatusError {
		t.Fatalf("expected error status, got %v", updated.StatusLevel)
	}
	if !strings.Contains(updated.Status, "branch lookup failed") {
		t.Fatalf("expected branch lookup error, got %q", updated.Status)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui -v`
Expected: FAIL with "m.ReviewLoader undefined"

**Step 3: Implement review refresh + error reporting**

```go
type ReviewLoader func() ([]string, error)

type Model struct {
	// ...
	ReviewLoader ReviewLoader
}

func LoadReviewQueueFromProject() ([]string, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return nil, err
	}
	db, err := storage.Open(project.StateDBPath(root))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return storage.ListReviewQueue(db)
}

// in Update after successful approve:
loader := m.ReviewLoader
if loader == nil {
	loader = LoadReviewQueueFromProject
	m.ReviewLoader = loader
}
queue, err := loader()
if err != nil {
	m.SetStatusError(fmt.Sprintf("review refresh failed: %v", err))
	return m, nil
}
m.ReviewQueue = queue
m.SetStatusInfo(fmt.Sprintf("approved %s (merged %s)", id, branch))

// on branch lookup error:
m.SetStatusError(fmt.Sprintf("branch lookup failed: %v", err))

// on approve error:
m.SetStatusError(fmt.Sprintf("approve failed: %v", err))
```

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/approve_key_test.go internal/tui/review_refresh.go
git commit -m "feat: report approve errors and refresh review queue"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
