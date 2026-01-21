# TUI Review Diff Navigation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Add full-screen file-by-file diff navigation in Review View, using target-branch diff with fallback to current branch.

**Architecture:** Introduce git helpers for unified diff output and name-only diff between base and task branch. Add diff state to TUI model (files, selected index, scroll offsets, per-file cache). Toggle Review diff view with `d`, render unified diff for selected file, and provide navigation keys.

**Tech Stack:** Go 1.24+, Bubble Tea, git CLI.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Add git unified diff helper (base..branch, per file)

**Files:**
- Create: `internal/git/diff_unified.go`
- Create: `internal/git/diff_unified_test.go`

**Step 1: Write failing test**

```go
package git

import "testing"

type fakeRunnerUnified struct{ out string }

func (f *fakeRunnerUnified) Run(name string, args ...string) (string, error) { return f.out, nil }

func TestDiffUnified(t *testing.T) {
	r := &fakeRunnerUnified{out: "@@ -1 +1 @@\n-old\n+new\n"}
	lines, err := DiffUnified(r, "main", "feature", "file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) == 0 || lines[0] != "@@ -1 +1 @@" {
		t.Fatalf("unexpected diff lines: %v", lines)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/git -v`
Expected: FAIL with "undefined: DiffUnified"

**Step 3: Implement helper**

```go
func DiffUnified(r Runner, base, branch, path string) ([]string, error) {
	out, err := r.Run("git", "diff", "--unified=3", base+".."+branch, "--", path)
	if err != nil {
		return nil, err
	}
	return ParseLines(out), nil
}
```

Add a small `ParseLines` helper in the same file (split on `\n`, trim trailing empty line). Keep it private.

**Step 4: Run tests**

Run: `go test ./internal/git -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/diff_unified.go internal/git/diff_unified_test.go
git commit -m "feat: add unified diff helper"
```

---

### Task 2: Add review diff state + loader in TUI

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/review_diff.go`
- Create: `internal/tui/review_diff_test.go`

**Step 1: Write failing tests**

```go
func TestReviewDiffLoadsFiles(t *testing.T) {
	m := NewModel()
	m.ReviewDiffLoader = func(taskID string) (ReviewDiffState, error) {
		return ReviewDiffState{Files: []string{"a.txt"}}, nil
	}
	m.ViewMode = ViewReview
	m.handleReviewDiff()
	if len(m.ReviewDiff.Files) != 1 {
		t.Fatalf("expected diff files")
	}
}
```

```go
func TestReviewDiffViewRendersHeader(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewShowDiffs = true
	m.ReviewDiff = ReviewDiffState{Files: []string{"a.txt"}, Current: 0, Lines: []string{"@@ -1 +1 @@"}}
	out := m.View()
	if !strings.Contains(out, "REVIEW DIFF") {
		t.Fatalf("expected diff header")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui -v`
Expected: FAIL with "ReviewDiff undefined"

**Step 3: Implement model + loader**

Add to `Model`:
- `ReviewShowDiffs bool`
- `ReviewDiff ReviewDiffState`
- `ReviewDiffLoader func(taskID string) (ReviewDiffState, error)`

Create `ReviewDiffState` (files, current index, lines, offsets per file, cached diffs). Implement `handleReviewDiff()`:
- Resolve selected task ID from review queue
- Call loader (default to `LoadReviewDiff`)
- Set `ReviewShowDiffs = true`
- Initialize `ReviewDiff.Current = 0` and `ReviewDiff.Lines` for first file

In `View()`, when `ReviewShowDiffs` render diff header/body/footer instead of review summary.

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/review_diff.go internal/tui/review_diff_test.go
git commit -m "feat: add review diff state"
```

---

### Task 3: Implement diff navigation + git-backed loader

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/review_diff.go`
- Modify: `internal/tui/review_diff_test.go`
- Modify: `internal/tui/review_detail.go`
- Modify: `internal/git/diff_runner.go`

**Step 1: Write failing tests**

```go
func TestReviewDiffNextPrev(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewShowDiffs = true
	m.ReviewDiff = ReviewDiffState{Files: []string{"a.txt", "b.txt"}, Current: 0}
	m.handleReviewDiffKey("j")
	if m.ReviewDiff.Current != 1 {
		t.Fatalf("expected next file")
	}
	m.handleReviewDiffKey("k")
	if m.ReviewDiff.Current != 0 {
		t.Fatalf("expected prev file")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui -v`
Expected: FAIL with "handleReviewDiffKey undefined"

**Step 3: Implement loader + navigation**

- Extend `DiffNameOnly` to accept base+branch (or add new helper `DiffNameOnlyRange`) to fetch `git diff --name-only base..branch`.
- `LoadReviewDiff(taskID)`:
  - Resolve project root
  - Resolve target base branch: use config `review.target_branch` if present, else current branch
  - Resolve task branch via `git.BranchForTask`
  - Load file list via name-only diff range
  - Load first file’s unified diff
- Add `handleReviewDiffKey(key string)` to move file selection, update cached lines, and clamp scroll offsets.
- Add diff scrolling with `u/d` for page up/down and `g/G` for top/bottom.

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/review_diff.go internal/tui/review_diff_test.go internal/tui/review_detail.go internal/git/diff_runner.go
git commit -m "feat: add review diff navigation"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
