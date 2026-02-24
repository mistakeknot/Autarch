# Code Review: Autarch Status Tool

**Date:** 2026-02-20
**Files reviewed:**
- `/root/projects/Interverse/hub/autarch/internal/status/data.go`
- `/root/projects/Interverse/hub/autarch/internal/status/model.go`
- `/root/projects/Interverse/hub/autarch/internal/status/runs.go`
- `/root/projects/Interverse/hub/autarch/internal/status/dispatches.go`
- `/root/projects/Interverse/hub/autarch/internal/status/events.go`
- `/root/projects/Interverse/hub/autarch/cmd/autarch/status.go`
- `/root/projects/Interverse/hub/autarch/internal/status/data_test.go`
- `/root/projects/Interverse/hub/autarch/internal/status/runs_test.go`

**Scope:** Go 1.24, Bubble Tea v1.3.10, lipgloss, Cobra. Read-only TUI dashboard reading Intercore kernel state via `ic` CLI subprocess.

---

## Overall Assessment

The code is well-structured, readable, and consistent with Go conventions. The Bubble Tea model follows the correct immutable-update pattern; data fetching is cleanly separated from rendering; the subprocess interface is safely handled. This is a solid first implementation. The findings below are ordered by impact.

---

## Findings

### 1. Silent error discard in `fetchData` (data.go / model.go) — Correctness Risk

**File:** `/root/projects/Interverse/hub/autarch/internal/status/model.go`, lines 267–269

```go
dispatches, _ = FetchDispatches(ctx, projectDir, true)
events, _ = FetchEvents(ctx, projectDir, runID, eventLimit)
tokens, _ = FetchTokens(ctx, projectDir, runID)
```

These three calls discard errors with `_`. If `ic` fails for dispatches or events — due to a crash, a context deadline, or a schema mismatch — the UI silently shows empty panes with no indication that data retrieval failed. The run pane will look active while dispatches and events are blank, which is misleading.

The design intent is clear: a failure on secondary data should not abort the whole frame. But that does not justify silent discard. The correct pattern here is to carry secondary errors into `dataMsg` and render them in the affected pane headers, or aggregate them into a non-fatal warnings field.

**Suggested fix — add a warnings field to `dataMsg`:**
```go
type dataMsg struct {
    Runs       []Run
    Dispatches []Dispatch
    Events     []Event
    Tokens     TokenSummary
    Err        error
    Warnings   []string // non-fatal errors for secondary fetches
}
```

Then collect the errors and surface them in the affected pane's header line, e.g. `DISPATCHES (fetch error: context deadline exceeded)`. This is a material correctness gap for a monitoring tool.

---

### 2. Naming inconsistency: `DispatchPane` vs `RunsPane` / `EventsPane` (data.go, dispatches.go) — Naming

**File:** `/root/projects/Interverse/hub/autarch/internal/status/dispatches.go`, line 13

The type is named `DispatchPane` (singular) while the other two are `RunsPane` and `EventsPane` (plural). This inconsistency appears in constructor names too: `NewRunsPane`, `NewDispatchPane`, `NewEventsPane`.

The data type is also named `Dispatch` (singular) while `Run` and `Event` are also singular — that part is consistent. The pane names should all follow the same pattern. Either all plural (`RunsPane`, `DispatchesPane`, `EventsPane`) or all singular. Given that the existing model field is named `dispatches` (plural), `DispatchesPane` and `NewDispatchesPane` would be the most consistent choice.

This is a naming defect, not a style preference: it signals structural uncertainty about whether the type holds one or many items, which is confusing when reading `model.go` where all three are used side by side.

---

### 3. `formatNumber` does not handle values >= 1 billion correctly (model.go) — Correctness

**File:** `/root/projects/Interverse/hub/autarch/internal/status/model.go`, lines 321–329

```go
func formatNumber(n int64) string {
    if n < 1000 {
        return fmt.Sprintf("%d", n)
    }
    if n < 1_000_000 {
        return fmt.Sprintf("%d,%03d", n/1000, n%1000)
    }
    return fmt.Sprintf("%d,%03d,%03d", n/1_000_000, (n%1_000_000)/1000, n%1000)
}
```

For values >= 1,000,000,000 (one billion), the last branch produces incorrect output. For example, `1_234_567_890` would format as `1234,567,890` — the leading group has no thousands separator for the billions digit. Token counts at that scale are realistic for long-running multi-agent runs.

This is exactly what the existing table-driven test in `data_test.go` validates, but the test only checks up to 1,234,567. Add a test case for 1,234,567,890 and fix the function to handle four groups (or delegate to `golang.org/x/text/message` / a locale formatter, though that adds a dependency). A simple fix:

```go
func formatNumber(n int64) string {
    if n < 1_000 {
        return fmt.Sprintf("%d", n)
    }
    if n < 1_000_000 {
        return fmt.Sprintf("%d,%03d", n/1_000, n%1_000)
    }
    if n < 1_000_000_000 {
        return fmt.Sprintf("%d,%03d,%03d", n/1_000_000, (n%1_000_000)/1_000, n%1_000)
    }
    return fmt.Sprintf("%d,%03d,%03d,%03d",
        n/1_000_000_000,
        (n%1_000_000_000)/1_000_000,
        (n%1_000_000)/1_000,
        n%1_000,
    )
}
```

---

### 4. `resolveProjectDir` does not validate user-supplied path contains a DB (status.go) — UX Gap

**File:** `/root/projects/Interverse/hub/autarch/cmd/autarch/status.go`, lines 61–68

When `--project` is supplied by the user, `resolveProjectDir` returns the absolute path without checking whether `.clavain/intercore.db` exists there:

```go
if dir != "" {
    abs, err := filepath.Abs(dir)
    if err != nil {
        return "", fmt.Errorf("invalid project dir: %w", err)
    }
    return abs, nil
}
```

The existence check happens immediately after in `RunE`, so the failure message will still be correct. However, the separation creates a false contract: `resolveProjectDir`'s doc says "finds the project directory containing .clavain/intercore.db" but it only enforces that for the auto-discover path. A reader of `resolveProjectDir` alone cannot tell that validation is deferred.

Two options:
- Move the DB existence check into `resolveProjectDir` for both the explicit and the auto-discover paths. This makes the function honor its documented contract.
- Or rename the function to `resolveAbsProjectDir` and update the comment to remove the "containing .clavain/intercore.db" language, leaving validation entirely to the caller.

Either is fine; what matters is removing the mismatch between the function name/doc and its actual behavior.

---

### 5. `runIC` error message uses `args[0]` which panics on empty args (data.go) — Defensive Gap

**File:** `/root/projects/Interverse/hub/autarch/internal/status/data.go`, lines 190–193

```go
if exitErr, ok := err.(*exec.ExitError); ok {
    return "", fmt.Errorf("ic %s: exit %d: %s", args[0], exitErr.ExitCode(), string(exitErr.Stderr))
}
return "", fmt.Errorf("ic %s: %w", args[0], err)
```

If `runIC` is called with zero args (which cannot happen in the current codebase, but is not enforced), this panics. More importantly, `args[0]` only captures the subcommand (e.g., `"run"`) when the intent is to identify the full invocation. For the exit error case, `exitErr.Stderr` is not trimmed: a stderr line ending with a newline will produce `ic run: exit 1: error: something\n` with a trailing newline embedded in the error string.

Minor but worth fixing:
```go
return "", fmt.Errorf("ic %s: exit %d: %s", args[0], exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
```

---

### 6. `FetchRuns` and `FetchAllRuns` share identical bodies (data.go) — Duplication

**File:** `/root/projects/Interverse/hub/autarch/internal/status/data.go`, lines 80–109

The two functions are identical except for whether `--active` is appended:

```go
func FetchRuns(ctx context.Context, projectDir string) ([]Run, error) {
    out, err := runIC(ctx, projectDir, "run", "list", "--active", "--json")
    // ...identical body...
}

func FetchAllRuns(ctx context.Context, projectDir string) ([]Run, error) {
    out, err := runIC(ctx, projectDir, "run", "list", "--json")
    // ...identical body...
}
```

`FetchAllRuns` is not called anywhere in the reviewed files. If it is genuinely needed as a public API, the duplication is fine to tolerate short-term but should be collapsed into a private helper:

```go
func fetchRunsInternal(ctx context.Context, projectDir string, activeOnly bool) ([]Run, error) {
    args := []string{"run", "list", "--json"}
    if activeOnly {
        args = append([]string{"run", "list", "--active", "--json"}, args[2:]...)
    }
    // ...
}
```

Or, more simply, give `FetchRuns` an `activeOnly bool` parameter — the same pattern already used by `FetchDispatches`. This is consistent with the project's own idiom. If `FetchAllRuns` is dead code, remove it.

---

### 7. Magic numbers in layout calculation (model.go) — Maintainability

**File:** `/root/projects/Interverse/hub/autarch/internal/status/model.go`, lines 170–176

```go
available := m.height - 7 // header + footer + gaps
```

The constant `7` is derived from the comment but is not verifiable without counting manually. The 30/30/remaining split is also computed with integer division in a way that means the three panes may sum to `available - 1` in some cases (for example, `available=10`: `10*30/100 = 3`, `3+3 = 6`, `eventsH = 4`, sum = 10, OK; `available=11`: `11*30/100 = 3`, same; `available=20`: `20*30/100 = 6`, `6+6=12`, `eventsH=8`, sum=20, OK). This is benign but opaque.

Define the constants:
```go
const (
    headerLines = 2
    footerLines = 2
    paneGaps    = 3 // two blank separator lines + 1 margin
    layoutOverhead = headerLines + footerLines + paneGaps
)
```

Then `available := m.height - layoutOverhead`. This is not a bug, but the magic 7 will mislead the next person who adds a banner or status bar.

---

### 8. Lipgloss style allocation in hot rendering paths (runs.go, dispatches.go, events.go) — Performance

**Files:** `runs.go` line 116–117, `dispatches.go` lines 102–104, `events.go` lines 134–144

Each call to `renderRunRow`, `renderDispatchRow`, `renderEventRow`, and `eventTypeStyle` allocates new `lipgloss.Style` values. For a 3-second poll interval this is not a practical performance problem — but `eventTypeStyle` is called once per event row and constructs a new style inside a switch every time.

This is a common pattern in Bubble Tea codebases and does not warrant immediate action. However, the pane types are good candidates to cache their styles as struct fields or package-level variables, which is the prevailing convention in the broader `pkg/tui` package (check `styles.go`). Consider aligning `internal/status` with whatever pattern `pkg/tui` establishes.

---

### 9. `fetchData` captures `runID` before the model processes the response (model.go) — Logic subtlety

**File:** `/root/projects/Interverse/hub/autarch/internal/status/model.go`, lines 245–247

```go
func (m Model) fetchData() tea.Cmd {
    projectDir := m.projectDir
    runID := selectedRunID(m.runs)
    return func() tea.Msg {
```

`runID` is captured at the time `fetchData` is called, not when the goroutine executes. This is correct and intentional — it prevents a race between user input and the goroutine reading the model. The comment on line 258 (`// If no run selected yet, use first active run`) shows the author understood this.

The logic on line 280:
```go
if len(filtered) == 0 {
    filtered = dispatches
}
```

This fallback means that if no dispatch has a matching `ScopeID`, all active dispatches are shown. This is a reasonable degradation strategy but is undocumented. A comment explaining the intent ("dispatches may pre-date scope assignment; show all rather than nothing") would prevent a future reader from treating it as a bug.

---

### 10. Test quality: `TestRenderProgressBar` asserts nothing meaningful (runs_test.go) — Test Gap

**File:** `/root/projects/Interverse/hub/autarch/internal/status/runs_test.go`, lines 8–29

```go
for _, tt := range tests {
    bar := renderProgressBar(tt.phase, phases, tt.barWidth)
    if bar == "" {
        t.Errorf("renderProgressBar(%q) returned empty string", tt.phase)
    }
    // Just verify it's not panicking and returns something
    _ = bar
}
```

The test defines `wantFill int` in the table but never uses it (the comment `// approximate filled chars` implies it was planned). The assertion is only `bar != ""`, which does not verify the progress bar shows the correct fill level. The test for "done" phase (full bar) vs "brainstorm" (1/5 filled) should produce observably different output.

A more useful check: strip ANSI sequences and count the filled block character `█`:

```go
// strip lipgloss styling to get raw text
plain := stripANSI(bar) // requires a simple regexp or ansi strip helper
filledCount := strings.Count(plain, "█")
if filledCount != tt.wantFill {
    t.Errorf("renderProgressBar(%q): filled = %d, want %d", tt.phase, filledCount, tt.wantFill)
}
```

The `TestRenderProgressBarNoPhases` test is correct and tests the right thing. The cursor tests in `runs_test.go` are solid: they cover initial state, clamping, and preservation on update — good table of cases.

---

### 11. `formatEventTime` tries RFC3339 then RFC3339Nano unnecessarily (events.go) — Minor Idiom

**File:** `/root/projects/Interverse/hub/autarch/internal/status/events.go`, lines 117–132

RFC3339Nano is a superset of RFC3339 — `time.Parse(time.RFC3339Nano, s)` will successfully parse an RFC3339 string with no nanoseconds. The two-attempt approach is unnecessary; trying `RFC3339Nano` first (or only) is sufficient:

```go
func formatEventTime(timestamp string) string {
    t, err := time.Parse(time.RFC3339Nano, timestamp)
    if err != nil {
        if len(timestamp) > 8 {
            return timestamp[:8]
        }
        return timestamp
    }
    return t.Local().Format("15:04:05")
}
```

This removes the false impression that the two formats are mutually exclusive. Low priority but clarifies intent.

---

### 12. `FetchEvents` silently skips malformed JSON lines (data.go) — Observability

**File:** `/root/projects/Interverse/hub/autarch/internal/status/data.go`, line 159

```go
if err := json.Unmarshal([]byte(line), &ev); err != nil {
    continue // skip malformed lines
}
```

Silently skipping malformed lines is reasonable for a streaming tail, but there is no count or indicator that lines were dropped. If the `ic events tail` output format changes, the events pane silently empties. Consider tracking a skip count and, if non-zero, appending a dim indicator line such as `(N lines skipped — unexpected format)` to the events pane.

This is low priority for a stable CLI but worth noting for robustness.

---

## Summary of Findings by Priority

| # | File | Finding | Priority |
|---|------|---------|----------|
| 1 | model.go | Silent error discard on secondary fetches | High |
| 2 | dispatches.go | Naming inconsistency: `DispatchPane` vs `RunsPane`/`EventsPane` | Medium |
| 3 | model.go | `formatNumber` incorrect for n >= 1B | Medium |
| 4 | status.go | `resolveProjectDir` contract mismatch (doc vs behavior) | Medium |
| 5 | data.go | `runIC` stderr not trimmed; `args[0]` panic on empty args | Low |
| 6 | data.go | `FetchAllRuns` duplicates `FetchRuns` body; possibly dead code | Low |
| 7 | model.go | Magic number `7` in layout calculation | Low |
| 8 | runs/dispatches/events.go | Style allocation in hot path | Informational |
| 9 | model.go | Dispatch scope-ID fallback logic lacks a comment | Informational |
| 10 | runs_test.go | `TestRenderProgressBar` asserts nothing about fill level | Medium |
| 11 | events.go | Two-attempt RFC3339/RFC3339Nano parse is unnecessary | Low |
| 12 | data.go | Silent skip of malformed event lines, no observability | Low |

---

## What Is Working Well

- **Bubble Tea model structure** is correct. `Update` uses value receivers, messages are typed cleanly, `tickCmd` correctly re-arms without double-firing during long fetches (the `!m.fetching` guard on line 75 prevents stacking tick callbacks).
- **`runIC` error wrapping** with `%w` is correct for the non-exit-error path. The exit error case correctly extracts stderr.
- **Cursor management in `RunsPane`** is correct: bounds-check on both `SetRuns` and `CursorDown`; `SelectedRun` returns nil safely for empty state.
- **Data model types** are appropriately lean with nullable fields modeled as `*string`/`*int64`. The `DisplayName`/`DisplayModel` accessor pattern on `Dispatch` is idiomatic and keeps nil-guard logic out of the rendering layer.
- **`fetchData` cmd closure** correctly captures `projectDir` and `runID` by value before the goroutine runs, which is the correct Bubble Tea concurrency pattern.
- **`resolveProjectDir` walk** handles the filesystem-root termination condition correctly (`parent == dir`), which is a common off-by-one in path-walk code.
- **Test suite for `data_test.go`** is well-structured: uses real JSON that matches the documented CLI output, covers nullable fields, and includes a proper table-driven test for `formatNumber`.
- **`cmd/autarch/status.go`** is clean and does exactly what a Cobra command file should: validate preconditions, construct the model, run the program, return the error.
