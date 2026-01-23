# Tandemonium Init Strict Validation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `Vauxpraudemonium-3hn (Task reference)`

**Goal:** Enforce strict validation of agent-generated epics/stories, produce a clear error report + raw output files, and improve prompt guidance so invalid outputs do not write specs.

**Architecture:** Add a validation layer in `internal/tandemonium/epics` that checks schema/IDs/enums and can emit a report. Update `init`’s agent output parsing to validate, write `init-epics-output.yaml` + `init-epics-errors.txt` on failure, and return a clear error. Harden the agent prompt with explicit enums and `estimates` guidance.

**Tech Stack:** Go, YAML (`gopkg.in/yaml.v3`).

**Worktree:** User requested no worktree; execute on current branch.

---

### Task 1: Epic/story validation rules

**Files:**
- Create: `internal/tandemonium/epics/validate.go`
- Test: `internal/tandemonium/epics/validate_test.go`

**Step 1: Write the failing test (invalid IDs/status/priority)**

```go
func TestValidateEpicsReportsErrors(t *testing.T) {
	epics := []Epic{
		{
			ID:       "EPIC-1",
			Title:    "Auth",
			Status:   Status("bogus"),
			Priority: Priority("p9"),
			Stories: []Story{
				{ID: "EPIC-002-S01", Title: "Bad story", Status: StatusTodo, Priority: PriorityP1},
			},
		},
	}

	errList := Validate(epics)
	if len(errList) == 0 {
		t.Fatalf("expected validation errors")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/epics -run TestValidateEpicsReportsErrors`

Expected: FAIL (Validate undefined).

**Step 3: Write minimal implementation**

```go
// validate.go
package epics

import "regexp"

type ValidationError struct {
	Path    string
	Message string
}

var epicIDPattern = regexp.MustCompile(`^EPIC-\d{3}$`)
var storyIDPattern = regexp.MustCompile(`^EPIC-\d{3}-S\d{2}$`)

func Validate(list []Epic) []ValidationError {
	var errs []ValidationError
	seenEpics := map[string]bool{}
	seenStories := map[string]bool{}
	for i, epic := range list {
		path := func(field string) string { return "epics[" + itoa(i) + "]." + field }
		if epic.ID == "" || !epicIDPattern.MatchString(epic.ID) {
			errs = append(errs, ValidationError{Path: path("id"), Message: "invalid epic id"})
		} else if seenEpics[epic.ID] {
			errs = append(errs, ValidationError{Path: path("id"), Message: "duplicate epic id"})
		} else {
			seenEpics[epic.ID] = true
		}
		if epic.Title == "" {
			errs = append(errs, ValidationError{Path: path("title"), Message: "title required"})
		}
		if !validStatus(epic.Status) {
			errs = append(errs, ValidationError{Path: path("status"), Message: "invalid status"})
		}
		if !validPriority(epic.Priority) {
			errs = append(errs, ValidationError{Path: path("priority"), Message: "invalid priority"})
		}
		for j, story := range epic.Stories {
			sp := func(field string) string {
				return "epics[" + itoa(i) + "].stories[" + itoa(j) + "]." + field
			}
			if story.ID == "" || !storyIDPattern.MatchString(story.ID) {
				errs = append(errs, ValidationError{Path: sp("id"), Message: "invalid story id"})
			} else if epic.ID != "" && !strings.HasPrefix(story.ID, epic.ID+"-") {
				errs = append(errs, ValidationError{Path: sp("id"), Message: "story id must match epic"})
			} else if seenStories[story.ID] {
				errs = append(errs, ValidationError{Path: sp("id"), Message: "duplicate story id"})
			} else {
				seenStories[story.ID] = true
			}
			if story.Title == "" {
				errs = append(errs, ValidationError{Path: sp("title"), Message: "title required"})
			}
			if !validStatus(story.Status) {
				errs = append(errs, ValidationError{Path: sp("status"), Message: "invalid status"})
			}
			if !validPriority(story.Priority) {
				errs = append(errs, ValidationError{Path: sp("priority"), Message: "invalid priority"})
			}
		}
	}
	return errs
}

func validStatus(s Status) bool {
	switch s {
	case StatusTodo, StatusInProgress, StatusReview, StatusBlocked, StatusDone:
		return true
	default:
		return false
	}
}

func validPriority(p Priority) bool {
	switch p {
	case PriorityP0, PriorityP1, PriorityP2, PriorityP3:
		return true
	default:
		return false
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tandemonium/epics -run TestValidateEpicsReportsErrors`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tandemonium/epics/validate.go internal/tandemonium/epics/validate_test.go
git commit -m "feat(tandemonium): add epic validation rules"
```

---

### Task 2: Validation report writer

**Files:**
- Modify: `internal/tandemonium/epics/validate.go`
- Test: `internal/tandemonium/epics/validate_test.go`

**Step 1: Write failing test (writes report files)**

```go
func TestWriteValidationReport(t *testing.T) {
	dir := t.TempDir()
	errList := []ValidationError{{Path: "epics[0].id", Message: "invalid epic id"}}
	outPath, errPath, err := WriteValidationReport(dir, []byte("raw"), errList)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output file: %v", err)
	}
	if _, err := os.Stat(errPath); err != nil {
		t.Fatalf("expected error file: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/epics -run TestWriteValidationReport`

Expected: FAIL (WriteValidationReport undefined).

**Step 3: Implement report writer + formatter**

```go
func WriteValidationReport(dir string, raw []byte, errs []ValidationError) (string, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	outPath := filepath.Join(dir, "init-epics-output.yaml")
	errPath := filepath.Join(dir, "init-epics-errors.txt")
	if err := os.WriteFile(outPath, raw, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(errPath, []byte(FormatValidationErrors(errs)), 0o644); err != nil {
		return "", "", err
	}
	return outPath, errPath, nil
}

func FormatValidationErrors(errs []ValidationError) string {
	var b strings.Builder
	for _, err := range errs {
		fmt.Fprintf(&b, "%s: %s\n", err.Path, err.Message)
	}
	return b.String()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tandemonium/epics -run TestWriteValidationReport`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tandemonium/epics/validate.go internal/tandemonium/epics/validate_test.go
git commit -m "feat(tandemonium): add validation report writer"
```

---

### Task 3: Strict validation in init flow + richer prompt

**Files:**
- Modify: `internal/tandemonium/cli/init_flow.go`
- Test: `internal/tandemonium/cli/init_flow_test.go`

**Step 1: Write failing test (invalid output writes report and returns error)**

```go
func TestParseAndValidateEpicsWritesReportOnError(t *testing.T) {
	planDir := t.TempDir()
	raw := []byte("epics:\n- id: EPIC-001\n  title: X\n  status: bogus\n  priority: p1\n")
	_, err := parseAndValidateEpics(raw, planDir)
	if err == nil {
		t.Fatalf("expected error")
	}
	if _, err := os.Stat(filepath.Join(planDir, "init-epics-output.yaml")); err != nil {
		t.Fatalf("expected output file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(planDir, "init-epics-errors.txt")); err != nil {
		t.Fatalf("expected error report: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/cli -run TestParseAndValidateEpicsWritesReportOnError`

Expected: FAIL (parseAndValidateEpics undefined).

**Step 3: Implement parse+validate helper and error guidance**

```go
func parseAndValidateEpics(raw []byte, planDir string) ([]epics.Epic, error) {
	list, err := parseAgentEpics(raw)
	if err != nil {
		return nil, err
	}
	errList := epics.Validate(list)
	if len(errList) > 0 {
		outPath, errPath, writeErr := epics.WriteValidationReport(planDir, raw, errList)
		if writeErr != nil {
			return nil, writeErr
		}
		return nil, fmt.Errorf("agent output invalid; wrote %s and %s", outPath, errPath)
	}
	return list, nil
}
```

Update agent generator to call `parseAndValidateEpics` with `planDir` so errors are always persisted.

**Step 4: Update the agent prompt**

Add explicit enums + “YAML only” to `buildAgentPrompt`:

```go
"Allowed status: todo|in_progress|review|blocked|done\n" +
"Allowed priority: p0|p1|p2|p3\n" +
"Use estimates (plural)\n" +
"Output YAML only (no prose)\n"
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/tandemonium/cli -run TestParseAndValidateEpicsWritesReportOnError`

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/tandemonium/cli/init_flow.go internal/tandemonium/cli/init_flow_test.go
git commit -m "feat(tandemonium): enforce strict init validation"
```

---

### Task 4: Full verification

**Files:**
- None

**Step 1: Run full test suite**

Run: `go test ./...`

Expected: PASS.

**Step 2: Commit (if any doc notes added later)**

```bash
git status -sb
```

---

Plan complete and saved to `docs/plans/2026-01-23-tandemonium-init-epics-validation-plan.md`.

Two execution options:

1. Subagent-Driven (this session) — I dispatch a fresh subagent per task, review between tasks
2. Parallel Session (separate) — Open a new session with executing-plans and batch execution

Which approach?
