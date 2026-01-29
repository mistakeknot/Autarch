# Scan Validation Wiring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** [none] (Task reference)

**Goal:** Wire scan artifact validation into the codebase scan pipeline and surface validation errors in the UI.

**Architecture:** Extend scan results to carry structured validation errors. Validate scan artifacts immediately after parsing, block invalid results, and display errors in the Kickoff chat panel with a rescan prompt.

**Tech Stack:** Go 1.24, Bubble Tea

---

### Task 1: Add validation error types to scan results

**Files:**
- Modify: `internal/autarch/agent/scan.go`
- Modify: `internal/tui/messages.go`
- Test: `internal/autarch/agent/scan_validate_test.go`

**Step 1: Write failing test**
Add a test ensuring invalid scan output returns validation errors.

```go
func TestScanCodebase_ReportsValidationErrors(t *testing.T) {
	// stub Agent returning JSON missing evidence
	// expect ScanResult.ValidationErrors non-empty
}
```

**Step 2: Run test to confirm failure**
```bash
GOCACHE=/tmp/gocache go test ./internal/autarch/agent -run TestScanCodebase_ReportsValidationErrors -v
```
Expected: FAIL

**Step 3: Implement minimal changes**
- Add `ValidationErrors []ValidationError` to `ScanResult`.
- In `ScanCodebaseWithProgress`, after `parseScanResponse`, validate against phase schemas (vision/problem/users extracted into per‑phase artifacts) and populate `ValidationErrors` if any. For now, validate only the legacy scan output by mapping into a `VisionArtifact`/`ProblemArtifact`/`UsersArtifact` with evidence set from file sources (or mark as missing evidence so validation fails). This ensures guardrails even before the richer scan output is implemented.

**Step 4: Run test to confirm pass**
```bash
GOCACHE=/tmp/gocache go test ./internal/autarch/agent -run TestScanCodebase_ReportsValidationErrors -v
```
Expected: PASS

**Step 5: Commit**
```bash
git add internal/autarch/agent/scan.go internal/autarch/agent/scan_validate_test.go internal/tui/messages.go

git commit -m "feat(agent): surface scan validation errors"
```

---

### Task 2: Surface validation errors in scan UI

**Files:**
- Modify: `internal/tui/messages.go`
- Modify: `internal/tui/views/kickoff.go`
- Test: `internal/tui/views/kickoff_chat_test.go`

**Step 1: Write failing test**
```go
func TestKickoffShowsScanValidationErrors(t *testing.T) {
	v := NewKickoffView()
	_, _ = v.Update(tui.CodebaseScanResultMsg{ValidationErrors: []tui.ValidationError{{Code:"missing_evidence", Message:"At least 2 evidence items required"}}})
	msgs := v.ChatMessagesForTest()
	// expect error message to appear in chat
}
```

**Step 2: Run test to confirm failure**
```bash
GOCACHE=/tmp/gocache go test ./internal/tui/views -run TestKickoffShowsScanValidationErrors -v
```
Expected: FAIL

**Step 3: Implement minimal UI handling**
- Add `ValidationErrors []ValidationError` to `CodebaseScanResultMsg`.
- In `KickoffView.Update` for `CodebaseScanResultMsg`, if validation errors present:
  - Do **not** enter scan review mode.
  - Append a system message list with errors + "Press ctrl+s to rescan.".
  - Keep `scanReview=false`.

**Step 4: Run test to confirm pass**
```bash
GOCACHE=/tmp/gocache go test ./internal/tui/views -run TestKickoffShowsScanValidationErrors -v
```
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/messages.go internal/tui/views/kickoff.go internal/tui/views/kickoff_chat_test.go

git commit -m "feat(tui): show scan validation errors"
```

---

### Task 3: Full test pass

**Files:**
- Test: `internal/autarch/agent`, `internal/tui/views`, `internal/tui`

**Step 1: Run tests**
```bash
GOCACHE=/tmp/gocache go test ./internal/autarch/agent -v
GOCACHE=/tmp/gocache go test ./internal/tui/views -v
GOCACHE=/tmp/gocache go test ./internal/tui -v
```
Expected: PASS

---

### Task 4: Commit and push

**Step 1: Final commit (if needed)**
```bash
git status --short
```
If any remaining changes:
```bash
git add internal/autarch/agent/scan.go internal/tui/messages.go internal/tui/views/kickoff.go internal/tui/views/kickoff_chat_test.go

git commit -m "feat(tui): wire scan validation"
```

**Step 2: Push**
```bash
git push
```
