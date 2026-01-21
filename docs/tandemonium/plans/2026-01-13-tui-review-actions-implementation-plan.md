# TUI Review Actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Wire Review View actions (feedback, reject, edit story) to persist to YAML/storage and refresh the UI.

**Architecture:** Reuse `specs.AppendReviewFeedback` and update `specs.UpdateUserStory` to also write `user_story.hash`. Add review action handlers in TUI to submit feedback/reject/edit, call storage to requeue on reject, and refresh `ReviewDetail`/queue. Keep immediate-write behavior on submit.

**Tech Stack:** Go 1.24+, Bubble Tea, YAML, SQLite.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Update user story write to include hash

**Files:**
- Modify: `internal/specs/review.go`
- Modify: `internal/specs/review_test.go`

**Step 1: Write failing test**

```go
func TestUpdateUserStoryWritesHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	data := []byte(`id: T1
user_story:
  text: As a user, I want X.
  hash: oldhash
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateUserStory(path, "As a user, I want Y."); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, err := LoadDetail(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.UserStoryHash == "oldhash" || updated.UserStoryHash == "" {
		t.Fatalf("expected updated hash")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/specs -v`
Expected: FAIL with "expected updated hash"

**Step 3: Implement hash write**

In `UpdateUserStory`, set `user_story.hash` to `StoryHash(text)`.

**Step 4: Run tests**

Run: `go test ./internal/specs -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/review.go internal/specs/review_test.go
git commit -m "feat: update story hash on edit"
```

---

### Task 2: Add review action submit handlers in TUI

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/review_actions_test.go` (create if missing)
- Modify: `internal/tui/review_detail.go`

**Step 1: Write failing tests**

```go
func TestSubmitFeedbackClearsInput(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewInputMode = ReviewInputFeedback
	m.ReviewInput = "Looks good"
	m.ReviewDetail = ReviewDetail{TaskID: "T1"}
	m.ReviewActionWriter = func(taskID, text string) error { return nil }
	m.handleReviewSubmit()
	if m.ReviewInputMode != ReviewInputNone || m.ReviewInput != "" {
		t.Fatalf("expected input cleared")
	}
}
```

```go
func TestSubmitRejectRequeues(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewInputMode = ReviewInputFeedback
	m.ReviewPendingReject = true
	m.ReviewInput = "Needs work"
	m.ReviewDetail = ReviewDetail{TaskID: "T1"}
	m.ReviewActionWriter = func(taskID, text string) error { return nil }
	m.ReviewRejecter = func(taskID string) error { return nil }
	m.handleReviewSubmit()
	if m.ReviewPendingReject {
		t.Fatalf("expected reject cleared")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui -v`
Expected: FAIL with "handleReviewSubmit undefined"

**Step 3: Implement handlers**

Add in `Model`:
- `ReviewActionWriter func(taskID, text string) error` (defaults to specs.AppendReviewFeedback)
- `ReviewStoryUpdater func(taskID, text string) error` (defaults to specs.UpdateUserStory)
- `ReviewRejecter func(taskID string) error` (defaults to storage.RejectTask)

Implement `handleReviewSubmit()`:
- On feedback: call writer, clear input mode, set status, refresh queue/detail
- On reject: call writer then rejecter, clear input mode + reject flag, refresh queue/detail
- On edit story: call story updater, clear input mode, refresh detail (to update drift/hash)

Hook submit on Enter when `ReviewInputMode != none`.

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/review_actions_test.go internal/tui/review_detail.go
git commit -m "feat: wire review submit actions"
```

---

### Task 3: Reject auto-requeue + queue refresh

**Files:**
- Modify: `internal/storage/review.go`
- Modify: `internal/storage/review_test.go`

**Step 1: Write failing test**

```go
func TestRejectTaskRequeues(t *testing.T) {
	db := setupTestDB(t)
	seedTask(t, db, "T1", "review")
	if err := RejectTask(db, "T1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	status := loadTaskStatus(t, db, "T1")
	if status != "ready" {
		t.Fatalf("expected ready, got %q", status)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/storage -v`
Expected: FAIL with "expected ready"

**Step 3: Implement requeue**

Modify `RejectTask` to set status to `rejected` then immediately to `ready` (or update directly to ready while recording rejection if needed).

**Step 4: Run tests**

Run: `go test ./internal/storage -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/storage/review.go internal/storage/review_test.go
git commit -m "feat: requeue tasks on reject"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
