# Learnings: TUI layout patterns

**Source:** Agent result from session b817bfb0
**Task ID:** a97b9d6
**Date:** 2026-02-05

---

This is very revealing. Now I have all the information I need. Let me compile my analysis.

---

## Analysis: TUI Layout Learnings vs. Acceptance Criteria Plan

The three TUI bug documents are **highly relevant** to the acceptance criteria plan. They expose structural layout risks that the current ACs do not adequately cover. Here is the specific analysis for each of your four questions.

---

### 1. AC-X.3 says "3-pane layout renders correctly at >=120 columns" but the bugs showed dimension mismatches WITHIN valid widths

**Directly applicable.** AC-X.3 tests at 120, 150, and 200 column widths, but both the breadcrumb bug (`tui-breadcrumb-hidden-by-oversized-child-view-20260127.md`) and the dimension mismatch bug (`tui-dimension-mismatch-splitlayout-20260126.md`) occurred at standard terminal widths -- the terminal was wide enough, yet content overflowed because **parent chrome dimensions were not subtracted from child `WindowSizeMsg` values**.

The current AC-X.3 only says "renders correctly at >=120 columns" -- a purely visual spot check. It does not test the underlying arithmetic that caused both historical bugs: whether `leftWidth + separator + rightWidth == availableWidth` actually holds, whether child views receive dimensions post-padding, or whether the total rendered output matches the terminal height exactly.

**Evidence from current code:** The Pollard, Gurgeh, and Coldwine views all have `v.height = msg.Height - 4` in their `WindowSizeMsg` handlers (`/root/projects/Autarch/internal/tui/views/pollard.go:71`, `gurgeh.go:71`, `coldwine.go:86`). But the parent `unified_app.go:1896` applies `Padding(1, 3)` -- that is 6 horizontal and 2 vertical characters. These child views subtract 4 from height but subtract **nothing from width**. This is exactly the bug pattern documented in `tui-dimension-mismatch-splitlayout-20260126.md`, where the fix required subtracting 6 from width and 2 from height. The current child views may still have the width mismatch.

**Suggested AC additions:**

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-X.3a | Every line of rendered TUI output has `ansi.StringWidth(line) == terminal_width`; no line exceeds terminal width | Automated: capture full View() output, assert per-line ANSI-aware width |
| AC-X.3b | `WindowSizeMsg` passed to child views subtracts parent chrome (header, footer, padding) both horizontally and vertically; child `width` + parent horizontal chrome == terminal width | Unit test: inject WindowSizeMsg(120, 40), assert child receives (120-6, 40-headerH-footerH-verticalPadding) |
| AC-X.3c | `SplitLayout` invariant holds: `LeftWidth() + separator(1) + spacing(2) + RightWidth() <= contentWidth` at all tested widths (120, 150, 200) | Unit test: SetSize at each width, assert sum equality |

---

### 2. AC-1.5 describes the Pollard 3-pane layout -- do the criteria account for WindowSizeMsg propagation bugs?

**Not accounted for.** AC-1.5 says: "Pollard 3-pane layout renders: sidebar (Inbox/Accepted/Rejected/Deferred + hunter status), doc pane (finding detail), agent pane (triage conversation)." This is a visual presence check -- it verifies that three panes exist, not that their sizes are correct.

The breadcrumb bug document explicitly warns: **"In any Bubble Tea app with chrome (header, footer, sidebar), always subtract chrome dimensions from WindowSizeMsg before passing to child models."** The Pollard view currently has a 3-layer nesting: `UnifiedApp` (header + footer + padding) -> `PollardView` -> `ShellLayout` (sidebar + separator) -> `SplitLayout` (doc + chat). Each layer must correctly subtract its chrome before passing dimensions down. The current code at `/root/projects/Autarch/internal/tui/views/pollard.go:69-72` passes `msg.Width` directly to `v.width` and then to `shell.SetSize()` without subtracting the parent's `Padding(1, 3)` horizontal space. The `ShellLayout.SetSize()` at `/root/projects/Autarch/pkg/tui/shelllayout.go:46-68` does correctly subtract sidebar width and separator, but it receives an already-incorrect `width` value from the Pollard view.

**Suggested AC additions:**

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-1.5a | Pollard 3-pane layout: `sidebar_width + separator(2) + doc_width + split_separator(3) + chat_width == available_content_width` (content width = terminal width minus parent padding) | Unit test: trace width arithmetic through UnifiedApp -> PollardView -> ShellLayout -> SplitLayout |
| AC-1.5b | No horizontal overflow in Pollard panes: content does not wrap or truncate unexpectedly at 120-column terminal | Manual test with tmux capture-pane, verify no stray `│` characters outside separator columns |
| AC-1.5c | Pollard total rendered height == content area height (terminal height minus header, footer, log pane, padding); no vertical clipping of header or footer | Automated: count lines in View() output, assert == expected |

---

### 3. The scrolling bug doc shows focus state issues -- does the plan test keyboard navigation between panes?

**Partially addressed but insufficient.** AC-1.5's verification says "Navigate to Pollard tab, verify all panes present" -- this is a display test, not an interaction test. The scrolling bug (`tui-scrolling-keyboard-and-mouse.md`) documented three issues that affect any multi-pane layout:

1. **Focus state authority:** The bug showed that `chatPanel.Focused()` (component internal state) diverged from `shell.Focus()` (layout authority). Any test that checks focus by querying a component rather than the shell will miss this.

2. **Keyboard routing per focus state:** When focused on doc pane, arrow keys should scroll doc. When focused on chat, arrow keys should go to chat input. Tab should cycle focus. The acceptance criteria say nothing about which keys work in which pane.

3. **Mouse scroll routing:** Mouse wheel events must route to the correct pane based on shell focus state, not component focus state.

The plan's Manual Testing section lists AC-1.5 as "3-pane layout visual verification -- verify 50/50 split, Ctrl+ keybindings only, slash command picker." The "Ctrl+ keybindings only" note is a design constraint from `chat-first-tui-design`, but there is no AC that tests focus cycling (Tab/Shift+Tab) actually moves focus, or that keyboard/mouse events route correctly per focus state.

**Suggested AC additions:**

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-X.11 | Tab cycles focus through visible panes (sidebar -> doc -> chat -> sidebar); Shift+Tab reverses; collapsed sidebar is skipped | Manual + unit: press Tab repeatedly, verify focus indicator moves; collapse sidebar, verify it's skipped |
| AC-X.12 | Keyboard scrolling (Ctrl+N/P, PgUp/PgDn) routes to focused pane only; typing characters in chat focus does not trigger scroll | Manual: focus doc pane, verify scroll keys work; focus chat pane, verify same keys don't scroll doc |
| AC-X.13 | Mouse wheel events route to the pane indicated by `shell.Focus()`, not component internal focus state | Manual + integration: focus doc pane via Tab, mouse scroll, verify doc scrolls; focus chat pane, mouse scroll, verify chat scrolls |
| AC-X.14 | Focus indicator is visually distinct per pane (border highlight, title color, or cursor presence indicates which pane is active) | Manual: verify each focus state has visible distinction |

---

### 4. Should there be ACs specifically for child view sizing, padding accounting, and split layout proportions?

**Yes, absolutely.** The three bug documents collectively document a class of layout bugs that share a single root cause: **dimensions not correctly propagated through the component tree.** This is not a one-time bug -- it is a structural risk that recurs whenever:

- A new view is added (each new view must subtract parent chrome)
- Parent chrome changes (e.g., adding a log pane, changing header height, toggling inline mode)
- Layout nesting depth increases (each layer must propagate reductions)

The existing prevention checklists in the bug docs are good but are not captured as testable ACs. Given that the Pollard view is a new 3-pane layout (not yet fully built per the plan), and it nests 3 layers deep (UnifiedApp -> PollardView -> ShellLayout -> SplitLayout), this is the highest-risk area for layout bugs.

**Suggested structural AC additions:**

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-X.15 | **Layout dimension invariant:** For every view that receives `WindowSizeMsg`, the width/height passed to child components equals `msg.Width/Height` minus that view's chrome (borders, padding, separators). Document the arithmetic in a comment at the `WindowSizeMsg` handler. | Code review + unit test: for each view, inject WindowSizeMsg and assert child dimensions |
| AC-X.16 | **Split proportion consistency:** `SplitLayout` ratio matches design spec (50/50 for shell layouts per chat-first-tui-design). At 120 columns with sidebar visible: sidebar=~20 chars, separator=2, doc=~48, split-sep=3, chat=~47. Exact values may vary but must sum to available width. | Unit test: SetSize(120-6, h), assert LeftWidth() + RightWidth() + separator == contentWidth |
| AC-X.17 | **ANSI-aware width calculation:** All `padToWidth` and `ensureSize` calls use `ansi.StringWidth()`, not `runewidth.StringWidth()` or `len()`. No styled text causes line overflow. | Grep for non-ANSI width functions; fuzz test with styled input strings |
| AC-X.18 | **Resize handling:** Terminal resize (WindowSizeMsg) at any point during the flow recalculates all pane dimensions correctly. No stale dimensions persist after resize. | Manual: resize terminal during sprint, Pollard triage, and Coldwine task view; verify layout adapts without artifacts |

---

### Summary

The three TUI layout bug documents are directly relevant to the acceptance criteria plan. They expose a **class of dimensional propagation bugs** that the current ACs treat as a simple "does it look right at 120 columns" visual check. The specific gaps:

1. **AC-X.3** tests widths but not the arithmetic invariants that caused two real bugs.
2. **AC-1.5** tests pane presence but not pane sizing correctness. The Pollard view's current code at `/root/projects/Autarch/internal/tui/views/pollard.go:70` does not subtract parent horizontal padding, repeating the exact pattern from the 2026-01-26 bug.
3. **No AC tests keyboard/mouse routing per focus state**, despite a documented bug where focus state divergence caused scrolling to target the wrong pane.
4. **No structural AC** ensures dimensional propagation correctness across the component tree, despite three separate bugs all stemming from the same root cause.

The 14 suggested ACs above (AC-X.3a through AC-X.3c, AC-1.5a through AC-1.5c, AC-X.11 through AC-X.14, AC-X.15 through AC-X.18) would close these gaps. Most are automatable as unit tests, reducing reliance on manual visual inspection.