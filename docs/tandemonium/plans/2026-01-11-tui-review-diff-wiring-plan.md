# TUI Review Diff Wiring Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Wire the review queue to real git diff file lists for selected tasks.

**Architecture:** Add a lightweight service that, given a task ID and branch, runs `git diff --name-only` and feeds the TUI model. Keep selection logic minimal (first item only).

**Tech Stack:** Go 1.24+, git CLI.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Diff service helper

**Files:**
- Create: `internal/tui/diff_loader.go`
- Create: `internal/tui/diff_loader_test.go`

**Step 1: Write failing test**

```go
package tui

import (
    "testing"

    "github.com/gensysven/tandemonium/internal/git"
)

type fakeRunner struct{ out string }

func (f *fakeRunner) Run(name string, args ...string) (string, error) { return f.out, nil }

func TestLoadDiffFiles(t *testing.T) {
    r := &fakeRunner{out: "a.txt\n"}
    files, err := LoadDiffFiles(r, "HEAD")
    if err != nil || len(files) != 1 {
        t.Fatal("expected one diff file")
    }
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL

**Step 3: Implement loader**

```go
func LoadDiffFiles(r git.Runner, rev string) ([]string, error) {
    return git.DiffNameOnly(r, rev)
}
```

**Step 4: Run test**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/diff_loader.go internal/tui/diff_loader_test.go
git commit -m "feat: add diff loader for TUI"
```

---

### Task 2: Populate DiffFiles in model (first review item)

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/model_diff_test.go`

**Step 1: Write failing test**

```go
package tui

import "testing"

func TestModelLoadsDiffFiles(t *testing.T) {
    m := NewModel()
    _ = m
    // placeholder: no crash on init
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: PASS (placeholder)

**Step 3: Implement minimal hook**

Add method `LoadDiffs(r git.Runner, rev string)` that sets `m.DiffFiles`.

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_diff_test.go
git commit -m "feat: add model diff loader"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
