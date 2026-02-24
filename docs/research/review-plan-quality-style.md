# Review: TUI Kernel Validation Plan — Go Test Quality & Style

**Plan file:** `/root/projects/Interverse/docs/plans/2026-02-20-tui-kernel-validation.md`
**Review date:** 2026-02-20
**Reviewer:** Flux-drive Quality & Style Reviewer
**Scope:** Go test naming, table-driven patterns, idiom conformance, anti-patterns, alignment with existing codebase test conventions

---

## Executive Summary

The plan is well-scoped and directionally sound — it correctly identifies that render-path coverage is missing, that pure struct tests already exist, and that the natural next layer is View() output verification. However, several test names deviate from established codebase conventions, a few proposed assertions are brittle or test implementation details, and one proposed test file placement is inconsistent with the package boundaries actually used in the codebase.

---

## Findings by Priority

### P0 — Blocks Correct Execution

#### P0-1: `TestStatusSymbolKernelStatuses` proposed for wrong package

**Location:** Task 4, proposed file `pkg/tui/components_test.go`

`StatusSymbol` and `StatusIndicator` are in `package tui` (`/root/projects/Autarch/pkg/tui/components.go`). The plan says "NEW FILE" at `pkg/tui/components_test.go`, which is the right directory. However, the concern here is subtler: `StatusSymbol` renders lipgloss-styled output. The string returned is NOT a plain symbol character — it is the symbol wrapped in a lipgloss ANSI escape sequence. For example:

```go
case "running", "working":
    return StatusRunning.Render("●")
```

In a headless test environment (no TTY, no `TERM`), lipgloss may strip all ANSI and return a plain string, which would make assertions like `strings.Contains(output, "●")` work. But under CI with `TERM=dumb` or lipgloss's `NoColor` mode, the same assertion would work for a different reason (plain fallback). This is fragile not because of Go conventions but because the plan proposes asserting "non-empty string and doesn't return `?`" without accounting for lipgloss rendering context.

**Concrete problem:** `TestStatusSymbolUnknown` asserts that unknown status returns `"?"` symbol. But `StatusSymbol` returns `StatusIdle.Render("?")`, not the raw string `"?"`. The assertion `output == "?"` will fail in any environment where lipgloss adds styles. The correct assertion is `strings.Contains(lipgloss.NewStyle().Render(output), "?")` — or better, strip ANSI before asserting. The existing codebase tests (e.g., `chatpanel_test.go`, `shelllayout_test.go`) use `strings.Contains(view, "literal text")` which works because view strings include the styled-but-readable text embedded in longer output. Asserting equality to a raw symbol will fail silently or noisily.

**Fix:** The test should call `lipgloss.Width(StatusSymbol("unknown")) > 0` or use `strings.Contains(StatusSymbol("running"), "●")`. The plan must specify how symbol tests handle ANSI wrapping — just saying "returns non-empty string" avoids the problem but then the test provides almost no value.

---

### P1 — Should Fix Before Implementation

#### P1-1: Test naming does not follow established codebase pattern

**Pattern from existing tests:**

The existing test files consistently use the form `Test<TypeName><Scenario>`, NOT `Test<TypeName><Method><Scenario>`. Examples:

- `TestRunsPaneCursor` (not `TestRunsPaneCursorDown`)
- `TestRunsPaneEmpty` (not `TestRunsPaneSelectedRunEmpty`)
- `TestCommandPickerFuzzyMatch` (not `TestCommandPickerFuzzyMatchExact`)
- `TestChatPanelHidesSystemRoleLabel` — describes observable behavior, not method call
- `TestAgentSelectorToggleAndSelect` — scenario description

The plan proposes names like:
- `TestDispatchPaneRenderWithData` — acceptable, close to convention
- `TestDispatchPaneRenderNilFields` — acceptable
- `TestEventsPaneRenderMalformedTimestamp` — fine
- `TestRunsPaneRenderSelectedHighlight` — fine

But these also appear:
- `TestStatusSymbolKernelStatuses` — "KernelStatuses" is not a scenario, it is a group of inputs. This should either be a table-driven test with `t.Run` subtests named per status, or the function should be named `TestStatusSymbolKnownStatuses`.
- `TestAgentBadgeKnownTypes` — same issue; "KnownTypes" is a category, not a scenario.
- `TestGlobalCommandsPoolNoDuplicates` — "Pool" is not a type name in the codebase. The actual function is `GlobalCommands()`. The name should be `TestGlobalCommandsNoDuplicates`.
- `TestSprintCommandsPool` — same issue; should be `TestSprintCommandsAllHaveCommand` or `TestSprintCommandsNonEmpty`.
- `TestChatPanelSprintSlashCommands` — the existing `TestParseSlashCommand` test already covers this function with a table-driven test for generic inputs. The new test adds sprint-specific inputs but would duplicate the structure. A better approach is extending `TestParseSlashCommand`'s table in `chatpanel_test.go` with sprint cases, not a new top-level function.

**Go convention reference:** The Go testing documentation and effective Go guidelines say test names should be `Test<FunctionUnderTest>` or `Test<Type>_<Scenario>` when using subtests. The underscore variant is common for disambiguating scenarios; the codebase uses descriptive CamelCase instead. The plan's names are mostly acceptable but inconsistent.

#### P1-2: Table-driven tests proposed without `t.Run` for multi-case scenarios

The plan proposes:
- `TestStatusSymbolKernelStatuses` — "table-driven: verify each kernel-emitted status"
- `TestChatPanelSprintSlashCommands` — "table-driven: '/status', '/sprint', '/phase brainstorm'"

The existing codebase uses `t.Run` for table-driven tests in `command_picker_test.go`:

```go
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        if got := fuzzyMatch(tt.target, tt.query); got != tt.want {
```

But `data_test.go` and `runs_test.go` do NOT use `t.Run` — they inline the loop:

```go
for _, tt := range tests {
    got := formatNumber(tt.n)
    if got != tt.want {
        t.Errorf("formatNumber(%d) = %q, want %q", tt.n, got, tt.want)
    }
}
```

Both styles exist in the codebase. The plan should specify which style is expected for each test, since `t.Run` subtests give individual failure reporting and are recommended by the Go testing documentation for table-driven tests where case isolation matters. For status symbol verification where individual statuses should be independently reportable, `t.Run` with the status string as subtest name is the better choice. The plan does not specify, leaving implementers to make inconsistent choices.

**Recommended language for the plan:** "Use `t.Run(status, ...)` for each row in the status table so failing cases are individually identified."

#### P1-3: `TestDispatchPaneRenderLongName` — brittle truncation assertion

The plan says: "verify truncation (name ≤ 20 chars in output)".

Looking at the actual rendering code in `dispatches.go`:

```go
name := d.DisplayName()
if len(name) > 20 {
    name = name[:20]
}
name = fmt.Sprintf("%-20s", name)
```

The rendered row also goes through `fmt.Sprintf("  %s %s %s %s %s %s", sym, idStyle.Render(...), name, ...)`. After lipgloss applies ANSI codes to other fields, `lipgloss.Width(view)` may not equal `len(view)`. The assertion "name ≤ 20 chars in output" conflates byte length and display width, and it also conflates extracting a field from the full row string versus checking the full View() output.

**Correct approach:** Assert that `strings.Contains(view, longName[:20])` AND that `!strings.Contains(view, longName)` (the full 30-char name is absent). This tests observable behavior (name was truncated) without measuring internal field width, which is an implementation detail.

#### P1-4: `TestEventsPaneRenderTruncation` — test name describes implementation, not behavior

The plan says: "event with height=3 (header + 2 rows) but 5 events → only last 2 events shown (newest-last tail behavior)".

The test name `TestEventsPaneRenderTruncation` is misleading — it is not testing truncation (text truncation) but **windowing / tail behavior**. A clearer name would be `TestEventsPaneTailWindow` or `TestEventsPaneShowsNewestWhenOverHeight`. The current name would cause a reader to confuse this with text truncation (like the dispatch name test), reducing the plan's clarity.

Also: the plan proposes `height=3 (header + 2 rows)`. Looking at the actual implementation:

```go
maxRows := p.height - 1
if maxRows < 1 {
    maxRows = 1
}
start := 0
if len(p.events) > maxRows {
    start = len(p.events) - maxRows
}
```

With `SetSize(80, 3)`, `maxRows = 2`. So 5 events → `start = 3`, showing events[3] and events[4]. The assertion must verify that the first 3 events (indices 0-2) are NOT in the output and the last 2 (indices 3-4) ARE. The plan says "only last 2 events shown" which is correct but the verification strategy of "verify strings" must use distinct strings per event — the test data must use unique, non-overlapping event type strings for this to be reliable. The plan should specify distinct event type values.

#### P1-5: `TestRunsPaneRenderSelectedHighlight` — risks testing lipgloss internals

The plan says: "verify the second run has visual indicator (cursor marker)".

Looking at the render code:

```go
if selected {
    row = lipgloss.NewStyle().
        Background(tui.ColorBgLighter).
        Width(width).
        Render(row)
}
```

The "cursor marker" is a background color, not a character like `>` or `*`. In a headless test with `NO_COLOR=1` or `TERM=dumb`, the background style will be stripped, and both selected and unselected rows will render identically. The plan should acknowledge this and propose either:

1. A structural test: `view != viewWithCursorOnFirst` (the output changes when cursor moves)
2. Checking for a ANSI background code (fragile, OS-dependent)
3. Adding a cursor character `▶` to the implementation if testability is a goal (this would require a component change, which is out of scope per the plan)

The safe approach is option 1: assert that the View() output differs between cursor positions, not that any specific marker string is present. The plan should specify this.

#### P1-6: `TestCommandPickerFilterSprintCommands` — tests filtered internal state, not behavior

The plan says: "filter 'pha' → matches 'phase' command".

`SprintCommands()` does not contain a "phase" command. Looking at the actual `SprintCommands()` implementation:

```go
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

There is no "phase" command in `SprintCommands()`. Filtering "pha" would match "phase" only if "phase" exists. With the actual data, "pha" would match nothing — or possibly "acceptance" via fuzzy matching ("pha" in "acceptance"? No. "a", "c", "c", "e", "p", "t", "a", "n", "c", "e" — the chars p-h-a are not in order in "acceptance"). This test as described will produce zero matches and fail to cover what the plan intends.

**This is a factual error in the plan.** The test name and filter value must be corrected to use an actual command from `SprintCommands()`. For example: filter "vis" → matches "vision", or filter "acc" → matches "accept" and "acceptance".

#### P1-7: Missing `t.Helper()` guidance for repeated setup patterns

All tasks share the same construction pattern: create pane, call SetSize, call SetX, call View(). The plan does not mention helper functions. The existing tests in the codebase do not yet use `t.Helper()` or shared setup helpers (each test is standalone), but for six tasks adding render tests across three files, repeating the setup verbatim is a maintenance liability. The plan should recommend either:

- A package-level helper `func newTestDispatchPane(t *testing.T, w, h int) *DispatchPane` marked with `t.Helper()`
- Or explicit acknowledgment that each test is intentionally standalone

Without guidance, different implementers will make different choices, producing an inconsistent test file.

---

### P2 — Nice to Have

#### P2-1: `TestChatPanelRenderAgentMessages` — overly broad for a render test

The plan proposes verifying "content text and appropriate role formatting" for user, agent, and system roles in one test. This combines three independent behavioral scenarios (system role hidden, user role shown, agent role shown) into a single assertion sequence. The existing `TestChatPanelHidesSystemRoleLabel` already covers the system case. The new test should be split into at least two cases or should explicitly state it extends coverage for roles not already covered. As written, it risks duplicating the existing test's assertions.

#### P2-2: `TestGlobalCommandsNoDuplicates` — useful but should also cover `KickoffCommands()` and `SprintCommands()`

If the goal is "no duplicate Command strings", the same check is valuable for all command pools. The plan only calls out `GlobalCommands()`. A table-driven test covering all four pools (`GlobalCommands()`, `KickoffCommands()`, `SprintCommands()`, `EpicReviewCommands()`, `TaskReviewCommands()`) would be more complete and is no harder to write.

#### P2-3: `TestDispatchPaneRenderNilFields` — description says "no panic" but doesn't specify the "—" assertion

The plan says: "no panic, renders '—' for duration". The "no panic" goal is tested by simply running the code. The "—" assertion is a meaningful correctness check. The plan should be explicit that both goals require distinct assertion lines — a test that only checks for no panic (via deferred recover or just by running) is weaker than one that also checks the "—" output. Implementers should assert `strings.Contains(view, "—")`.

#### P2-4: No guidance on `SetSize` values across tests

Tests in `internal/status/` call `SetSize(80, 20)` uniformly per the plan, but the rendering code uses `height` to compute `maxRows`. For `TestEventsPaneRenderTruncation`, height=3 is deliberate. For `TestDispatchPaneRenderWithData` with 2 dispatches, height=20 gives plenty of rows. The plan should note that height matters and that tests requiring pagination behavior must explicitly document why a non-default height is chosen, to prevent future maintainers from "fixing" the height and breaking the test intent.

#### P2-5: Task 5 overlap with existing `TestParseSlashCommand`

`TestChatPanelSprintSlashCommands` proposes testing "/status", "/sprint", "/phase brainstorm". But neither "/status" nor "/sprint" nor "/phase" appear in `SprintCommands()` or `GlobalCommands()`. These would be arbitrary slash commands parsed by `ParseSlashCommand()`, which is already tested. The plan is conflating two different surfaces: the slash command parser (already covered) and the command picker pool contents. If the intent is to verify that a user typing "/status" in a sprint context produces a `SlashCommandMsg`, that test belongs in the existing `TestParseSlashCommand` table, not as a new function. The new function name implies testing something specific to `ChatPanel` behavior with sprint commands, but the plan's description just calls `ParseSlashCommand` with specific inputs.

---

## Summary Table

| Finding | ID | Severity | Task |
|---|---|---|---|
| `StatusSymbol` returns lipgloss-wrapped output; raw symbol equality will fail | P0-1 | P0 | Task 4 |
| "phase" command missing from `SprintCommands()`; filter "pha" matches nothing | P1-6 | P1 | Task 6 |
| Test naming inconsistency: "Pool" suffix, "KernelStatuses" category-as-name | P1-1 | P1 | Tasks 4, 6 |
| Table-driven tests lack `t.Run` guidance for individual failure isolation | P1-2 | P1 | Tasks 4, 5 |
| Long-name truncation assertion conflates byte length with display width | P1-3 | P1 | Task 1 |
| "Truncation" test name describes windowing/tail behavior, not text truncation | P1-4 | P1 | Task 2 |
| Selected-highlight test checks background color, stripped in headless envs | P1-5 | P1 | Task 3 |
| No `t.Helper()` guidance for shared setup patterns | P1-7 | P1 | All |
| `TestChatPanelRenderAgentMessages` partially duplicates existing system-role test | P2-1 | P2 | Task 5 |
| Duplicate command check should cover all command pools | P2-2 | P2 | Task 6 |
| "No panic" goal vs. "—" assertion should be explicitly split | P2-3 | P2 | Task 1 |
| SetSize height semantics not documented for pagination tests | P2-4 | P2 | Tasks 1-3 |
| Task 5 tests parser, not ChatPanel+sprint integration | P2-5 | P2 | Task 5 |

---

## Recommended Corrections to the Plan

### Task 1 — DispatchPane

Replace: "verify truncation (name ≤ 20 chars in output)"
With: "assert `strings.Contains(view, fullName[:20])` is true AND `strings.Contains(view, fullName)` is false"

Replace: "no panic, renders '—' for duration"
With: "assert `strings.Contains(view, \"—\")` and that the test does not panic"

### Task 2 — EventsPane

Replace test name: `TestEventsPaneRenderTruncation`
With: `TestEventsPaneTailWindowDropsOldest`

Add to description: "Use distinct, non-overlapping strings for each event's type field so that absence/presence can be asserted independently per event."

### Task 3 — RunsPane

Replace: "verify the second run has visual indicator (cursor marker)"
With: "assert that View() output differs between cursor=0 and cursor=1 states"

### Task 4 — StatusSymbol/StatusIndicator

Replace test name: `TestStatusSymbolKernelStatuses`
With: `TestStatusSymbolKnownStatuses`

Add: "Use `t.Run(status, ...)` for each case. Assert `strings.Contains(result, expectedChar)` where `expectedChar` is the Unicode symbol (not raw equality, since lipgloss wraps the return value). Verify the `?` fallback with `strings.Contains(StatusSymbol("unknown"), "?")`."

Replace: `TestAgentBadgeKnownTypes`
With: `TestAgentBadgeKnownAgentTypes`

Replace: `TestGlobalCommandsPoolNoDuplicates`
With: `TestGlobalCommandsNoDuplicates`

### Task 6 — CommandPicker

Replace: "filter 'pha' → matches 'phase' command"
With: "filter 'vis' → matches 'vision' command" (or any valid `SprintCommands()` entry)

Replace test name: `TestSprintCommandsPool`
With: `TestSprintCommandsAllHaveCommandField`

---

## Alignment with Existing Patterns: Checklist

| Convention | Plan Alignment |
|---|---|
| `strings.Contains` for View() assertions | Correct — matches chatpanel_test, shelllayout_test |
| Construct pane → SetSize → SetX → View() pattern | Correct — matches runs_test approach |
| Flat function tests without `t.Run` for simple cases | Mixed — plan should specify when to use `t.Run` |
| `t.Run` for multi-case table-driven tests | Used in command_picker_test; plan should mandate it for status table |
| Meaningful error messages with `t.Errorf` format strings | Not specified in plan; existing tests do this well |
| No mocking of external calls | Correct — plan keeps `ic` CLI out of scope |
| Package-internal testing (same package) | Correct — all proposed files use `package status` and `package tui` |
