# Branch Lookup for TUI Approve Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Resolve the correct git branch for a review task and pass it into the approve flow.

**Architecture:** Add a git helper to list local branches and find the best match for a task ID. The TUI model will use a branch-lookup function (injectable for tests) to determine the branch before calling the approver.

**Tech Stack:** Go 1.24+, git CLI, Bubble Tea.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Add git branch lookup helper

**Files:**
- Create: `internal/git/branch.go`
- Create: `internal/git/branch_test.go`

**Step 1: Write failing tests**

```go
package git

import (
	"errors"
	"testing"
)

type fakeRunner struct{ out string }

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	return f.out, nil
}

func TestBranchForTaskPrefersExactMatch(t *testing.T) {
	r := &fakeRunner{out: "feature/TAND-001\nTAND-001\n"}
	branch, err := BranchForTask(r, "TAND-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "TAND-001" {
		t.Fatalf("expected exact match, got %q", branch)
	}
}

func TestBranchForTaskFallsBackToContains(t *testing.T) {
	r := &fakeRunner{out: "feature/TAND-001\nbugfix/other\n"}
	branch, err := BranchForTask(r, "TAND-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "feature/TAND-001" {
		t.Fatalf("expected contains match, got %q", branch)
	}
}

func TestBranchForTaskNotFound(t *testing.T) {
	r := &fakeRunner{out: "feature/OTHER\n"}
	_, err := BranchForTask(r, "TAND-001")
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("expected ErrBranchNotFound, got %v", err)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/git -v`
Expected: FAIL with "undefined: BranchForTask"

**Step 3: Implement helper**

```go
package git

import (
	"errors"
	"strings"
)

var ErrBranchNotFound = errors.New("branch not found for task")

func ListBranches(r Runner) ([]string, error) {
	out, err := r.Run("git", "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	return ParseNameOnly(out), nil
}

func BranchForTask(r Runner, taskID string) (string, error) {
	branches, err := ListBranches(r)
	if err != nil {
		return "", err
	}
	lowerID := strings.ToLower(taskID)
	for _, b := range branches {
		if strings.EqualFold(b, taskID) {
			return b, nil
		}
	}
	for _, b := range branches {
		if strings.Contains(strings.ToLower(b), lowerID) {
			return b, nil
		}
	}
	return "", ErrBranchNotFound
}
```

**Step 4: Run tests**

Run: `go test ./internal/git -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/branch.go internal/git/branch_test.go
git commit -m "feat: add git branch lookup helper"
```

---

### Task 2: Use branch lookup in the TUI approve key path

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/approve_key_test.go`

**Step 1: Update failing test**

```go
func TestApproveKeyCallsApprover(t *testing.T) {
	m := NewModel()
	m.ReviewQueue = []string{"TAND-001"}
	m.BranchLookup = func(taskID string) (string, error) {
		if taskID != "TAND-001" {
			t.Fatalf("unexpected task ID: %s", taskID)
		}
		return "feature/TAND-001", nil
	}
	fake := &fakeKeyApprover{}
	m.Approver = fake

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	if !fake.called {
		t.Fatal("expected approve call")
	}
	if fake.taskID != "TAND-001" {
		t.Fatalf("expected task ID, got %q", fake.taskID)
	}
	if fake.branch != "feature/TAND-001" {
		t.Fatalf("expected branch, got %q", fake.branch)
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL with "m.BranchLookup undefined"

**Step 3: Implement branch lookup in model**

```go
type BranchLookup func(taskID string) (string, error)

type Model struct {
	Title        string
	Sessions     []string
	ReviewQueue  []string
	DiffFiles    []string
	Approver     Approver
	BranchLookup BranchLookup
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'a' {
			if len(m.ReviewQueue) > 0 {
				approver := m.Approver
				if approver == nil {
					approver = &ApproveAdapter{}
					m.Approver = approver
				}
				lookup := m.BranchLookup
				if lookup == nil {
					lookup = func(taskID string) (string, error) {
						return git.BranchForTask(&git.ExecRunner{}, taskID)
					}
				}
				branch, err := lookup(m.ReviewQueue[0])
				if err != nil {
					return m, nil
				}
				_ = m.ApproveTask(approver, m.ReviewQueue[0], branch)
			}
		}
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
git commit -m "feat: lookup branch for approve key"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
