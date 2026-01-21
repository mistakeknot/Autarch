# TUI MVP Boundary Actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Add MVP scope warning actions in Review View: [A]ccept to update scope, [r]evert file (commit), and e[x]plain.

**Architecture:** Extend review rendering to show an MVP warning block when `Alignment == "out"`. Add action handlers that (1) update MVP scope or task override, (2) revert a selected file on the task branch and commit, (3) persist explanation text to spec. Use existing git base/branch resolution from review diff and update review detail/queue after actions.

**Tech Stack:** Go 1.24+, Bubble Tea, YAML, git CLI.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: MVP override persistence in spec

**Files:**
- Modify: `internal/specs/review.go`
- Modify: `internal/specs/review_test.go`

**Step 1: Write failing test**

```go
func TestAppendMVPExplanation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	if err := os.WriteFile(path, []byte("id: T1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendMVPExplanation(path, "Approved for launch" ); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "mvp_explanation") {
		t.Fatal("expected mvp_explanation in yaml")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/specs -v`
Expected: FAIL with "undefined: AppendMVPExplanation"

**Step 3: Implement helper**

Add `AppendMVPExplanation(path, text string)` to append a string to `mvp_explanation` list (or create list if missing).

**Step 4: Run tests**

Run: `go test ./internal/specs -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/review.go internal/specs/review_test.go
git commit -m "feat: store MVP explanations"
```

---

### Task 2: Render MVP warning + wire [A]ccept/[x]plain input

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/mvp_actions_test.go`

**Step 1: Write failing tests**

```go
func TestReviewViewShowsMVPWarning(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewDetail = ReviewDetail{Alignment: "out"}
	out := m.View()
	if !strings.Contains(out, "MVP SCOPE WARNING") {
		t.Fatalf("expected mvp warning")
	}
}
```

```go
func TestMVPExplainClearsInput(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewInputMode = ReviewInputFeedback
	m.ReviewInput = "Reason"
	m.ReviewDetail = ReviewDetail{TaskID: "T1", Alignment: "out"}
	m.MVPExplainWriter = func(taskID, text string) error { return nil }
	m.handleMVPExplainSubmit()
	if m.ReviewInputMode != ReviewInputNone {
		t.Fatalf("expected input cleared")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui -v`
Expected: FAIL with "expected mvp warning"

**Step 3: Implement warning + handlers**

- Render warning block when `Alignment == "out"` with `[A]ccept [r]evert file [x]plain`.
- Add `MVPExplainWriter func(taskID, text string) error` (default: specs.AppendMVPExplanation).
- Add `handleMVPExplainSubmit()` to write explanation, clear input, refresh detail, set status.
- Map `x` key to `ReviewInputMode` (reuse feedback input), and on submit call `handleMVPExplainSubmit`.

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/mvp_actions_test.go
git commit -m "feat: add MVP warning and explain"
```

---

### Task 3: [A]ccept updates MVP scope

**Files:**
- Modify: `internal/specs/plan.go` (or create helper)
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/mvp_actions_test.go`

**Step 1: Write failing test**

```go
func TestMVPAcceptUpdatesScope(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewDetail = ReviewDetail{TaskID: "T1", Alignment: "out"}
	m.MVPAcceptor = func(taskID string) error { return nil }
	m.handleMVPAccept()
	if m.Status == "" {
		t.Fatalf("expected status")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui -v`
Expected: FAIL with "handleMVPAccept undefined"

**Step 3: Implement accept**

- Add `MVPAcceptor func(taskID string) error` default implementation:
  - Find project root
  - Update plan/MVP scope if a plan file exists (append feature ID to included list), else write a task-level override in spec (`mvp_override: acknowledged`)
- On success, refresh review detail and set status.

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/mvp_actions_test.go internal/specs/plan.go
git commit -m "feat: accept MVP scope"
```

---

### Task 4: Revert file + commit

**Files:**
- Create: `internal/git/revert_file.go`
- Create: `internal/git/revert_file_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/mvp_actions_test.go`

**Step 1: Write failing test**

```go
func TestRevertFileCallsRunner(t *testing.T) {
	r := &fakeRunner{output: ""}
	if err := RevertFile(r, "main", "file.txt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/git -v`
Expected: FAIL with "undefined: RevertFile"

**Step 3: Implement revert + commit**

- `RevertFile(r Runner, base, path string)` runs `git checkout base -- path`.
- In TUI, add `MVPReverter func(taskID, path string) error`:
  - Resolve base/branch (same as review diff)
  - Run revert on task branch
  - Commit with message `"chore: revert <path> for MVP scope"`
- Add file selection for revert using existing diff file list (first file by default, j/k to choose).

**Step 4: Run tests**

Run: `go test ./internal/git -v`
Expected: PASS

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/revert_file.go internal/git/revert_file_test.go internal/tui/model.go internal/tui/mvp_actions_test.go
git commit -m "feat: revert file for MVP scope"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
