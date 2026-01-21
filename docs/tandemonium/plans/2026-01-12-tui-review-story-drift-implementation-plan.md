# TUI Review Story Drift + Alignment Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Detect story drift from spec YAML hash and render story/alignment warnings in the Review View.

**Architecture:** Extend spec detail parsing to read `user_story.hash` and `strategic_context.mvp_included`. Add a hash helper to compute the current story hash and compare against stored hash to derive drift status. Render a warning block in Review View when drift is detected or unknown, plus a simple alignment line for MVP scope.

**Tech Stack:** Go 1.24+, Bubble Tea, YAML, SHA256.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Parse story hash + MVP included from spec YAML

**Files:**
- Modify: `internal/specs/detail.go`
- Modify: `internal/specs/detail_test.go`

**Step 1: Write failing test**

```go
func TestLoadDetailParsesStoryHashAndMVP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	data := []byte(`id: T1
title: Example
user_story:
  text: As a user, I want X.
  hash: abcdef12
strategic_context:
  mvp_included: true
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	detail, err := LoadDetail(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.UserStoryHash != "abcdef12" {
		t.Fatalf("expected hash, got %q", detail.UserStoryHash)
	}
	if detail.MVPIncluded == nil || *detail.MVPIncluded != true {
		t.Fatalf("expected MVP included true")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/specs -v`
Expected: FAIL with "UserStoryHash undefined"

**Step 3: Implement parsing**

```go
type SpecDetail struct {
	// ...
	UserStoryHash string
	MVPIncluded   *bool
}
```

Parse:
- `user_story.hash` into `UserStoryHash`
- `strategic_context.mvp_included` into `MVPIncluded` (bool pointer, nil if missing)

**Step 4: Run tests**

Run: `go test ./internal/specs -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/detail.go internal/specs/detail_test.go
git commit -m "feat: parse story hash and MVP flag"
```

---

### Task 2: Story hash helper + drift status in review detail

**Files:**
- Create: `internal/specs/story_hash.go`
- Create: `internal/specs/story_hash_test.go`
- Modify: `internal/tui/review_detail.go`
- Modify: `internal/tui/review_detail_test.go`

**Step 1: Write failing tests**

```go
package specs

import "testing"

func TestStoryHash(t *testing.T) {
	h := StoryHash("As a user, I want X.")
	if len(h) != 8 {
		t.Fatalf("expected 8-char hash, got %q", h)
	}
}
```

```go
func TestReviewDetailIncludesStoryDrift(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewDetail = ReviewDetail{StoryDrift: "changed"}
	out := m.View()
	if !strings.Contains(out, "STORY DRIFT") {
		t.Fatalf("expected drift warning")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/specs -v`
Expected: FAIL with "undefined: StoryHash"

Run: `go test ./internal/tui -v`
Expected: FAIL with "ReviewDetail.StoryDrift undefined"

**Step 3: Implement helper + drift status**

```go
func StoryHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:8]
}
```

In `ReviewDetail`, add:

```go
StoryDrift string // "ok" | "changed" | "unknown"
Alignment  string // "mvp" | "out" | "unknown"
```

In `LoadReviewDetail`, compute:
- If `UserStoryHash` present and `UserStory` present: compare with `StoryHash(UserStory)`.
- If mismatch: `StoryDrift = "changed"`.
- If match: `StoryDrift = "ok"`.
- If missing hash or story: `StoryDrift = "unknown"`.

Alignment:
- If `MVPIncluded == nil` → "unknown"
- If true → "mvp"
- If false → "out"

**Step 4: Run tests**

Run: `go test ./internal/specs -v`
Expected: PASS

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/story_hash.go internal/specs/story_hash_test.go internal/tui/review_detail.go internal/tui/review_detail_test.go
git commit -m "feat: add story drift status"
```

---

### Task 3: Render drift + alignment warnings

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/review_detail_test.go`

**Step 1: Write failing test**

```go
func TestReviewViewShowsAlignment(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewDetail = ReviewDetail{Alignment: "mvp"}
	out := m.View()
	if !strings.Contains(out, "ALIGNMENT") {
		t.Fatalf("expected alignment section")
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL with "expected alignment section"

**Step 3: Implement rendering**

In review view:
- If `StoryDrift == "changed"`: render a `STORY DRIFT DETECTED` block.
- If `StoryDrift == "unknown"`: render `Story drift: unknown`.
- Add `ALIGNMENT` section with MVP status.

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/review_detail_test.go
git commit -m "feat: render story drift and alignment"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
