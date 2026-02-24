# Correctness Review: TUI Kernel Validation Plan

**Plan:** `/root/projects/Interverse/docs/plans/2026-02-20-tui-kernel-validation.md`
**Reviewer:** Julik (Flux-drive Correctness Reviewer)
**Date:** 2026-02-20

---

## Invariants Under Review

Before examining individual tasks, these invariants must hold for the test suite to be meaningful:

1. A `strings.Contains` check on `View()` output finds literal text even when lipgloss ANSI codes are present — because ANSI codes wrap text, they do not split it.
2. `len()` on styled lipgloss output is NOT a reliable measure of visual character count. Under TrueColor profile, `len("\x1b[1;38;2;121;162;247mDISPATCHES\x1b[0m") == 35`, not 10.
3. `lipgloss.Width()` measures visual columns correctly by stripping ANSI — it is the right tool for visual-width assertions.
4. In a `go test` environment with no TTY, lipgloss defaults to the Ascii (no-color) profile. ANSI codes are not emitted. Invariant 1 holds trivially in that case. If `TERM` or `COLORTERM` is set in CI, ANSI codes are emitted but Invariant 1 still holds.
5. Truncation limits are byte-based (`len()`, not `lipgloss.Width()`). For ASCII-only fields this is the same. For the em-dash "—" (3 bytes, 1 visual column) in duration output, byte-based padding via `fmt.Sprintf("%-7s", "—")` yields 7 bytes (1 em-dash + 4 spaces = 5 visual columns), not 7 visual columns. This is a pre-existing display quirk.
6. `SetSize(w, h)` must be called before `View()` for `maxRows` and `goalWidth` computations to reflect realistic dimensions. With zero dimensions, `maxRows` clamps to 1 and `goalWidth` clamps to 10.

---

## P0 — Blocks Correct Execution

### P0.1: `TestRunsPaneRenderSelectedHighlight` — assertion is untestable with `strings.Contains`

**Location:** Plan Task 3, test 3.

**What the plan says:** "cursor on second run → verify the second run has visual indicator (cursor marker)"

**What the code does:**

```go
// runs.go, renderRunRow()
if selected {
    row = lipgloss.NewStyle().
        Background(tui.ColorBgLighter).
        Width(width).
        Render(row)
}
```

The selected indicator is **background color only**. There is no text cursor symbol (">" or "▶") added anywhere. The visual difference is purely ANSI background escape codes.

**Test environment failure:** Under the default `go test` no-TTY environment (Ascii/no-color profile), `lipgloss.NewStyle().Background(...).Render(row)` strips the color and returns the same text as the unselected row (plus trailing padding spaces to meet `Width`). No ANSI codes are emitted at all. A `strings.Contains` check for any cursor marker will always fail because none exists in the text. A check for the run's content will pass for both the selected and unselected rows — the test cannot distinguish them.

**Interleaving/sequence:**

```
Test: SetSize(80, 20)
Test: SetRuns([run1, run2])
Test: CursorDown()     → cursor = 1 (run2 selected)
Test: view = View()
Test: strings.Contains(view, ">") → FALSE  (no ">" in code)
Test: strings.Contains(view, "▶") → FALSE  (no "▶" in code)
Test: ???              → no correct assertion exists
```

**Fix:** Either (a) change the plan to assert that calling `View()` twice — once with cursor on run1, once on run2 — produces different output (i.e., `view1 != view2`), or (b) assert that the second run's content appears in the output at all (content-presence, not highlight-presence). Option (a) confirms the behavior changed but does not confirm which run was highlighted. Option (b) does not test highlighting at all. The most robust approach: force TrueColor profile in `TestMain` or in the test itself via `lipgloss.SetColorProfile(termenv.TrueColor)`, then check for the background ANSI sequence, but this couples tests to the specific hex color value `#292e42`.

The plan's stated assertion is not realizable with `strings.Contains` on text content. This test, as described, will either always pass trivially (wrong assertion) or always fail (right assertion with wrong method).

---

## P1 — Should Fix Before Executing

### P1.1: `TestEventsPaneRenderMalformedTimestamp` — "raw timestamp shown" is false; it's truncated to 8 chars

**Location:** Plan Task 2, test 4.

**What the plan says:** "event with non-RFC3339 timestamp → no panic, raw timestamp shown"

**What the code does:**

```go
// events.go, formatEventTime()
func formatEventTime(timestamp string) string {
    t, err := time.Parse(time.RFC3339, timestamp)
    if err != nil {
        t, err = time.Parse(time.RFC3339Nano, timestamp)
        if err != nil {
            // Fall back to showing raw (truncated)
            if len(timestamp) > 8 {
                return timestamp[:8]
            }
            return timestamp
        }
    }
    return t.Local().Format("15:04:05")
}
```

For a malformed timestamp like `"not-a-real-timestamp"` (20 chars), `formatEventTime` returns `"not-a-re"` — the first 8 bytes. The plan says the test should verify "raw timestamp shown", which a test writer would naturally interpret as checking `strings.Contains(view, "not-a-real-timestamp")`. That assertion is **false**. The string in the view is `"not-a-re"`.

**Concrete failure:** If the test uses `timestamp = "not-a-real-timestamp"` and asserts `strings.Contains(view, "not-a-real-timestamp")`, the test fails. If it uses a timestamp exactly 8 characters long (e.g., `"badvalue"`), it passes. If it uses one shorter than 8 (e.g., `"bad"`), it passes. The plan's wording creates a false pass/fail risk depending on test data choice.

**Fix:** The plan should say: "raw timestamp is truncated to 8 chars and shown." The test assertion must be `strings.Contains(view, timestamp[:8])` and `!strings.Contains(view, timestamp)` for inputs longer than 8 characters. Or, the plan should specify a ≤8-char malformed input.

---

### P1.2: `TestAgentBadgeKnownTypes` — case mismatch: `AgentBadge("claude")` renders `"Claude"` not `"claude"`

**Location:** Plan Task 4, test 3.

**What the plan says:** "verify each known agent returns non-empty badge containing the name"

**What the code does:**

```go
// components.go
func AgentBadge(agentType string) string {
    switch agentType {
    case "claude", "claude-code":
        return BadgeClaudeStyle.Render("Claude")   // capital C
    case "codex", "codex-cli":
        return BadgeCodexStyle.Render("Codex")     // capital C
    case "aider":
        return BadgeAiderStyle.Render("Aider")     // capital A
    case "cursor":
        return BadgeCursorStyle.Render("Cursor")   // capital C
    default:
        return BadgeStyle.Render(agentType)        // raw input
    }
}
```

The known agent types render capitalized display names. If a test does:

```go
badge := AgentBadge("claude")
if !strings.Contains(badge, "claude") { t.Errorf(...) }
```

This **fails** because the badge contains `"Claude"` (capital C), not `"claude"`. Similarly for `"codex"`, `"aider"`, and `"cursor"`.

The plan's requirement "containing the name" is ambiguous about case. The test as naturally written will produce a false failure for all four known agent types.

**Fix:** The assertion must use the capitalized form: `strings.Contains(badge, "Claude")`. Alternatively, use `strings.EqualFold` or `strings.ToLower`. The plan should specify the expected badge text explicitly: `AgentBadge("claude")` → badge contains `"Claude"`.

---

### P1.3: `TestEventsPaneRenderWithData` — timestamp assertions are timezone-dependent

**Location:** Plan Task 2, test 2.

**What the plan says:** "verify output contains: event types, state transitions ('brainstorm → strategized'), formatted timestamps"

**What the code does:**

```go
// events.go, formatEventTime()
return t.Local().Format("15:04:05")
```

`t.Local()` converts the parsed UTC timestamp to the machine's local timezone. Per `CLAUDE.md`, the server timezone is `America/Los_Angeles` (Pacific Time, UTC-8 in winter / UTC-7 in summer).

If the test uses `timestamp = "2026-02-20T09:15:00Z"` and asserts `strings.Contains(view, "09:15:00")`, it **fails** on the Pacific timezone machine because:

- UTC `09:15:00Z` → PST `01:15:00` (UTC-8 in February)
- The view contains `"01:15:00"`, not `"09:15:00"`

**Concrete interleaving:**

```
Event.Timestamp = "2026-02-20T09:15:00Z"
formatEventTime → time.Parse RFC3339 OK → t = 2026-02-20 09:15:00 UTC
t.Local() → 2026-02-20 01:15:00 PST
view contains: "01:15:00"
test asserts: Contains("09:15:00") → FALSE  ← test fails in PST, passes in UTC
```

The test as written is a CI/environment-sensitive false failure — it passes only on UTC machines.

**Fix options:**
- Assert on any `HH:MM:SS` pattern via regex: `regexp.MustCompile(`\d{2}:\d{2}:\d{2}`).MatchString(view)`.
- Use a timestamp that is the same in UTC and local by anchoring to noon UTC (`"2026-02-20T20:00:00Z"` → `"12:00:00"` PST), though this is brittle.
- Use `time.Local()` in the test to compute the expected display value rather than hardcoding it.
- Best approach: assert only that the view contains SOME formatted time by checking the colon-separated pattern, not a specific hour value.

---

### P1.4: `TestChatPanelSprintSlashCommands` — `"/phase brainstorm"` is not in `SprintCommands()`; test name is misleading

**Location:** Plan Task 5, test 1.

**What the plan says:** "table-driven: '/status', '/sprint', '/phase brainstorm' → all parsed correctly as SlashCommandMsg with expected command and args"

**What `SprintCommands()` actually contains:**

```go
// command_picker.go
func SprintCommands() []SlashCommandDef {
    return []SlashCommandDef{
        {Command: "accept", ...},
        {Command: "1", ...},
        {Command: "2", ...},
        {Command: "3", ...},
        {Command: "vision", ...},
        {Command: "problem", ...},
        {Command: "users", ...},
        {Command: "features", ...},
        {Command: "cujs", ...},
        {Command: "reqs", ...},
        {Command: "scope", ...},
        {Command: "acceptance", ...},
    }
}
```

There is no `"phase"`, `"status"`, or `"sprint"` command in `SprintCommands()`. Nor do they appear in `GlobalCommands()`. The test name claims to test "sprint slash commands that come from kernel context," but `ParseSlashCommand("/phase brainstorm")` works for ANY slash-prefixed text — it is entirely generic. The test is actually testing `ParseSlashCommand` in isolation.

This is a **correctness problem for the plan**: the test as described cannot validate that sprint commands are correctly handled end-to-end through `CommandPicker`, because the commands being tested do not exist in any command pool. A `CommandPicker` populated with `SprintCommands()` would NOT match `/phase` at all. The test exercises `ParseSlashCommand` (which parses arbitrary slash commands) but labels it as a sprint command integration test — creating a semantic false pass.

**What the plan should say:** Either (a) test sprint commands that actually exist in `SprintCommands()` such as `"/accept"`, `"/vision"`, `"/scope"`, or (b) explicitly frame this as testing `ParseSlashCommand` with arbitrary inputs, separate from the `SprintCommands()` pool.

---

## P2 — Nice to Have / Traps for Implementors

### P2.1: ID and model truncation traps — using untruncated values in assertions causes silent false failures

**Location:** Plan Tasks 1 and 3.

The plan specifies feeding dispatch data with IDs and model names but does not specify their lengths. The rendering code truncates:

```go
// dispatches.go
id := d.ID
if len(id) > 8 {
    id = id[:8]   // ID truncated at 8 bytes
}

model := d.DisplayModel()
if len(model) > 12 {
    model = model[:12]   // model truncated at 12 bytes
}
```

If the test uses `ID = "dispatch-abc-001"` (16 chars) and asserts `strings.Contains(view, "dispatch-abc-001")`, the assertion is **false** — only `"dispatch"` (8 chars) appears in the view. The view does contain the 8-char prefix, but the full string does not appear.

Similarly, if the test uses `Model = "claude-opus-4.5-haiku"` and checks for the full name, only `"claude-opus-"` (12 chars) appears.

**Recommendation:** Use short IDs (≤8 chars, e.g., `"d1"`, `"d2"`) and short model names (≤12 chars, e.g., `"opus"`, `"sonnet"`) in test fixtures. Then `strings.Contains` assertions on the full value are reliable.

---

### P2.2: "name ≤ 20 chars in output" is an ambiguous assertion requirement

**Location:** Plan Task 1, test 3 (`TestDispatchPaneRenderLongName`).

The plan says "verify truncation (name ≤ 20 chars in output)" but does not specify the assertion method. A `View()` output string includes the header, status symbols, IDs, padding, and ANSI codes (if any). You cannot verify "name ≤ 20 chars" by `len(view)`.

The only correct assertion pattern is:
```go
longName := "this-is-a-very-long-dispatch-name" // 34 chars
// ...setup dispatch with this name...
view := pane.View()
// Correct:
if strings.Contains(view, longName) {
    t.Error("full 34-char name should not appear (should be truncated to 20)")
}
if !strings.Contains(view, longName[:20]) {
    t.Error("first 20 chars should appear")
}
```

The plan's wording is likely to produce an implementor writing `len(extractedName) <= 20`, which requires extracting the name substring from the ANSI-decorated, padded row — a fragile and error-prone operation. The plan should state the assertion pattern explicitly.

---

### P2.3: `SetSize` must be called before `View()` for all dimensional tests; plan is inconsistent

**Location:** Plan Tasks 1, 2, 3.

The plan's Pattern note says "Create DispatchPane, call `SetSize(80, 20)`, SetDispatches(runID, dispatches), check View()." This is correct for dimensional tests. But the plan does not uniformly enforce this for all tests.

Specific risk: `TestEventsPaneRenderWithData` (Task 2, test 2) — if `SetSize` is not called (height=0), then `maxRows` = 0-1 = -1, clamped to 1. With 3 events and maxRows=1, only `events[2]` (the last one) is shown. The test expecting to find event types from all 3 events would then fail for events 0 and 1.

The plan should be explicit: every test in Tasks 1-3 that checks row content **must** call `SetSize(80, 20)` (or a height tall enough to display all fed data) before calling `View()`. This is especially critical for:

- `TestEventsPaneRenderWithData` (3 events need height ≥ 4 to show all)
- `TestRunsPaneRenderWithData` (2 runs need height ≥ 3 to show both)
- `TestDispatchPaneRenderWithData` (2 dispatches need height ≥ 3 to show both)

---

### P2.4: `ChatPanel.View()` uses glamour markdown rendering — plain text only in content for `strings.Contains`

**Location:** Plan Task 5, test 2 (`TestChatPanelRenderAgentMessages`).

The plan says: "add messages with user, agent, system roles → verify View() contains the content text and appropriate role formatting"

`ChatPanel` uses the `glamour` markdown renderer for message content. Glamour transforms Markdown syntax:

- `"**bold**"` becomes ANSI bold `"bold"` (the `**` markers are consumed)
- `"# heading"` becomes padded, colored heading text (the `#` is consumed)
- Code blocks gain ANSI color highlighting

If a test adds a message with `"**important task**"` and asserts `strings.Contains(view, "**important task**")`, the assertion is **false** — glamour ate the `**`.

Plain prose without Markdown syntax survives: `"build the feature"` → `"build the feature"` still appears. The plan should specify using plain text message content to keep assertions reliable.

---

### P2.5: EventsPane tail truncation test needs 5 distinct event contents to be verifiable

**Location:** Plan Task 2, test 3 (`TestEventsPaneRenderTruncation`).

The plan says "height=3 → only last 2 events shown (newest-last tail behavior)." The assertion must verify:
- events[3] and events[4] ARE in the output
- events[0], [1], [2] are NOT in the output

This is only verifiable if each event has **unique, distinguishable content** in the rendered row. If the test uses 5 events with the same `Type` (e.g., all `"advance"`), checking `!strings.Contains(view, "advance")` fails for the wrong reason, and `strings.Contains(view, "advance")` does not verify WHICH events are shown.

The test must use unique Type strings per event (e.g., `"type-alpha"`, `"type-beta"`, `"type-gamma"`, `"type-delta"`, `"type-epsilon"`) and then verify that `"type-alpha"` (events[0]) is absent and `"type-delta"` + `"type-epsilon"` are present.

The current plan description uses "phase advance, dispatch start, error" — only 3 named types for 5 events, leaving 2 events undescribed. The test specification is incomplete.

---

## ANSI Safety Summary

The root question — "will ANSI codes interfere with `strings.Contains` checks?" — has a nuanced answer:

**No, for text-presence checks.** ANSI escape sequences wrap text; they do not split it. `"\x1b[1;38;2;121;162;247mDISPATCHES\x1b[0m"` contains the literal string `"DISPATCHES"`. Under both no-color (test default) and TrueColor profiles, `strings.Contains(output, "DISPATCHES")` returns true.

**Yes, for length/position checks.** `len(output)` includes ANSI bytes. `output[20:40]` may slice through an ANSI sequence and produce garbage. The plan correctly uses only `strings.Contains` for assertions, which avoids this. The one exception is P0.1 (selected-row highlight), where the only difference between selected and unselected is background ANSI codes — not findable by text search.

**Verified empirically:** `lipgloss.NewStyle().Foreground(...).Render("DISPATCHES")` produces `"\x1b[1;38;2;121;162;247mDISPATCHES\x1b[0m"` under TrueColor profile. `strings.Contains(above, "DISPATCHES")` = true. `len(above)` = 35, not 10.

---

## Issue Index

| ID | Severity | Test | Issue |
|----|----------|------|-------|
| P0.1 | P0 | `TestRunsPaneRenderSelectedHighlight` | Background-only highlight has no text marker; assertion cannot distinguish selected vs unselected in no-color env |
| P1.1 | P1 | `TestEventsPaneRenderMalformedTimestamp` | Code truncates to 8 chars; plan says "raw timestamp shown" — false |
| P1.2 | P1 | `TestAgentBadgeKnownTypes` | `AgentBadge("claude")` renders `"Claude"` not `"claude"`; case-sensitive Contains fails |
| P1.3 | P1 | `TestEventsPaneRenderWithData` | `formatEventTime` uses `t.Local()` — UTC input gives PST output, hardcoded assertion fails on this machine |
| P1.4 | P1 | `TestChatPanelSprintSlashCommands` | `"/phase"`, `"/status"`, `"/sprint"` are not in `SprintCommands()`; tests the generic parser, not sprint integration |
| P2.1 | P2 | Tasks 1, 3 | IDs >8 chars and models >12 chars are silently truncated; assertions on full strings produce false failures |
| P2.2 | P2 | Task 1 test 3 | "name ≤ 20 chars in output" is ambiguous; must use `!Contains(full) + Contains([:20])` pattern |
| P2.3 | P2 | Tasks 1–3 | `SetSize` must be called before `View()` for all dimensional tests; plan is inconsistent |
| P2.4 | P2 | Task 5 test 2 | ChatPanel uses glamour — Markdown syntax in message content is consumed; use plain text only |
| P2.5 | P2 | Task 2 test 3 | 5 events need 5 distinct Type values to verify tail behavior; plan describes only 3 named types |

---

## Source File Reference

- `/root/projects/Autarch/internal/status/dispatches.go` — `DispatchPane.View()`, `renderDispatchRow()`, `dispatchDuration()`
- `/root/projects/Autarch/internal/status/events.go` — `EventsPane.View()`, `renderEventRow()`, `formatEventTime()`
- `/root/projects/Autarch/internal/status/runs.go` — `RunsPane.View()`, `renderRunRow()`
- `/root/projects/Autarch/internal/status/data.go` — `Run`, `Dispatch`, `Event` types, `DisplayName()`, `DisplayModel()`
- `/root/projects/Autarch/pkg/tui/components.go` — `StatusSymbol()`, `AgentBadge()`, `PriorityBadge()`
- `/root/projects/Autarch/pkg/tui/command_picker.go` — `CommandPicker`, `SprintCommands()`, `GlobalCommands()`
- `/root/projects/Autarch/pkg/tui/chatpanel.go` — `ChatPanel`, `ParseSlashCommand()`
- `/root/projects/Autarch/pkg/tui/colors.go` — color constants
