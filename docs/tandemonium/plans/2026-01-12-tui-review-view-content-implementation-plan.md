# TUI Review View Content Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Render real review content (summary, files changed, tests summary, acceptance criteria) using spec YAML, git diff stats, and session logs.

**Architecture:** Add spec detail loader, git diffstat helper, session lookup + test-summary parser, then wire a review-detail loader into the TUI Review View. Use best‑effort parsing for tests and fallback to “unknown”.

**Tech Stack:** Go 1.24+, Bubble Tea, SQLite, YAML, git CLI.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Spec detail loader (summary + acceptance criteria)

**Files:**
- Create: `internal/specs/detail.go`
- Create: `internal/specs/detail_test.go`

**Step 1: Write failing tests**

```go
package specs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDetail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "T1.yaml")
	data := []byte(`id: T1
title: Example
summary: |
  Did the thing.
acceptance_criteria:
  - id: ac-1
    description: First
  - id: ac-2
    description: Second
user_story:
  text: As a user, I want X.
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	detail, err := LoadDetail(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.ID != "T1" || detail.Title != "Example" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if len(detail.AcceptanceCriteria) != 2 {
		t.Fatalf("expected acceptance criteria")
	}
	if detail.UserStory != "As a user, I want X." {
		t.Fatalf("expected user story")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/specs -v`
Expected: FAIL with "undefined: LoadDetail"

**Step 3: Implement loader**

```go
type SpecDetail struct {
	ID                 string
	Title              string
	Summary            string
	UserStory          string
	AcceptanceCriteria []string
}

func LoadDetail(path string) (SpecDetail, error) {
	// Unmarshal YAML into map and extract fields:
	// - id, title, summary
	// - user_story.text
	// - acceptance_criteria[].description
}
```

**Step 4: Run tests**

Run: `go test ./internal/specs -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/detail.go internal/specs/detail_test.go
git commit -m "feat: add spec detail loader"
```

---

### Task 2: Git diff stats helper

**Files:**
- Create: `internal/git/diffstat.go`
- Create: `internal/git/diffstat_test.go`

**Step 1: Write failing tests**

```go
package git

import "testing"

func TestParseNumstat(t *testing.T) {
	out := "10\t2\tsrc/app.go\n5\t0\tREADME.md\n"
	stats := ParseNumstat(out)
	if len(stats) != 2 {
		t.Fatalf("expected 2 entries")
	}
	if stats[0].Path != "src/app.go" || stats[0].Added != 10 || stats[0].Deleted != 2 {
		t.Fatalf("unexpected stat: %+v", stats[0])
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/git -v`
Expected: FAIL with "undefined: ParseNumstat"

**Step 3: Implement helper**

```go
type DiffStat struct {
	Path    string
	Added   int
	Deleted int
}

func ParseNumstat(output string) []DiffStat { /* parse \t-separated */ }

func DiffNumstat(r Runner, base, branch string) ([]DiffStat, error) {
	out, err := r.Run("git", "diff", "--numstat", base+".."+branch)
	if err != nil { return nil, err }
	return ParseNumstat(out), nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/git -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/diffstat.go internal/git/diffstat_test.go
git commit -m "feat: add git diffstat helper"
```

---

### Task 3: Session lookup + test summary parsing

**Files:**
- Modify: `internal/storage/session.go`
- Create: `internal/storage/session_lookup_test.go`
- Create: `internal/tui/test_summary.go`
- Create: `internal/tui/test_summary_test.go`

**Step 1: Write failing tests**

```go
package storage

import "testing"

func TestFindSessionByTask(t *testing.T) {
	db, _ := OpenTemp()
	_ = Migrate(db)
	_ = InsertSession(db, Session{ID: "s1", TaskID: "T1", State: "working", Offset: 0})
	s, err := FindSessionByTask(db, "T1")
	if err != nil || s.ID != "s1" {
		t.Fatalf("expected session")
	}
}
```

```go
package tui

import "testing"

func TestFindTestSummary(t *testing.T) {
	log := "start\nRunning tests...\nPASS 8/8\nend\n"
	summary := FindTestSummary(log)
	if summary != "PASS 8/8" {
		t.Fatalf("unexpected summary: %q", summary)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/storage -v`
Expected: FAIL with "undefined: FindSessionByTask"

Run: `go test ./internal/tui -v`
Expected: FAIL with "undefined: FindTestSummary"

**Step 3: Implement helpers**

```go
func FindSessionByTask(db *sql.DB, taskID string) (Session, error) {
	row := db.QueryRow(`SELECT id, task_id, state, offset FROM sessions WHERE task_id = ? ORDER BY rowid DESC LIMIT 1`, taskID)
	// scan into Session
}
```

```go
func FindTestSummary(log string) string {
	// return last line containing "test", "PASS", "FAIL", "passed", "failed"
	// fallback "Tests: unknown"
}
```

**Step 4: Run tests**

Run: `go test ./internal/storage -v`
Expected: PASS

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/storage/session.go internal/storage/session_lookup_test.go internal/tui/test_summary.go internal/tui/test_summary_test.go
git commit -m "feat: add session lookup and test summary"
```

---

### Task 4: Review detail loader + render real sections

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/review_detail.go`
- Create: `internal/tui/review_detail_test.go`

**Step 1: Write failing tests**

```go
package tui

import "testing"

func TestReviewDetailRenderIncludesSummary(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewDetail = ReviewDetail{
		TaskID: "T1",
		Title: "Example",
		Summary: "Did the thing.",
		TestsSummary: "PASS 8/8",
		Files: []ReviewFile{{Path: "src/app.go", Added: 10, Deleted: 2}},
		AcceptanceCriteria: []string{"First", "Second"},
	}
	out := m.View()
	if !strings.Contains(out, "SUMMARY") || !strings.Contains(out, "Did the thing.") {
		t.Fatalf("expected summary content, got %q", out)
	}
	if !strings.Contains(out, "FILES CHANGED") || !strings.Contains(out, "src/app.go") {
		t.Fatalf("expected files content")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui -v`
Expected: FAIL with "ReviewDetail undefined"

**Step 3: Implement loader + view**

Create `review_detail.go`:

```go
type ReviewFile struct { Path string; Added int; Deleted int }

type ReviewDetail struct {
	TaskID             string
	Title              string
	Summary            string
	UserStory          string
	AcceptanceCriteria []string
	Files              []ReviewFile
	TestsSummary        string
}

func LoadReviewDetail(taskID string) (ReviewDetail, error) {
	// - root := project.FindRoot(".")
	// - load spec: specs.LoadDetail(specPath)
	// - branch from git.BranchForTask
	// - diff stats from git.DiffNumstat(defaultBranch, branch)
	// - session log from storage.FindSessionByTask + read .tandemonium/sessions/<id>.log
	// - tests summary via FindTestSummary
}
```

In `Model` add:

```go
ReviewDetail       ReviewDetail
ReviewDetailLoader func(taskID string) (ReviewDetail, error)
```

When entering review view or changing selection, load detail:

```go
if m.ViewMode == ViewReview {
	m.ensureReviewDetail()
}
```

Render sections in review view:

```
REVIEW - <id>: <title>
SUMMARY
...
FILES CHANGED
...
TESTS: <summary>
ACCEPTANCE CRITERIA
...
```

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/review_detail.go internal/tui/review_detail_test.go
git commit -m "feat: render review details"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
