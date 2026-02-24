# UX Review: Acceptance Criteria Plan (2026-02-05)

**Reviewer:** UX Reviewer (CLI/TUI Specialist)
**Date:** 2026-02-05
**Plan reviewed:** `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md`
**Verdict:** Net positive with 12 specific issues requiring attention

---

## UX Assessment

### User Workflows Affected

This plan defines acceptance criteria for five core user journeys (CUJs) across the Autarch unified TUI. It touches:

1. **Research-enriched PRD creation** (CUJ-1) -- the primary workflow. Users create a PRD through an 8-phase sprint while Pollard research runs in the background and surfaces findings.
2. **Research triage** -- a new sub-workflow within the Pollard tab where users Accept/Reject/Defer/Deep Dive findings using natural language in the agent pane.
3. **Task breakdown** (CUJ-2) -- reviewing a proposed task hierarchy and accepting/editing items.
4. **Parallel development** (CUJ-4) -- monitoring file reservations and agent status in the TUI.
5. **Multi-project dashboard** (CUJ-5) -- scanning multiple projects, drilling into details.

### Overall Impact

The plan **improves** the user experience in several ways:

- It forces correctness-vs-performance separation for timing criteria, which directly prevents "flaky CI makes the TUI seem broken" feedback loops.
- It defines graceful degradation for Agent Teams and Intermute unavailability, so users are never stuck at a dead end.
- It specifies a button fallback for the agent pane, preserving usability when the agent is unavailable.
- The badge pulse and research surfacing design uses progressive disclosure -- findings arrive non-disruptively in the background.

However, there are **12 specific UX issues** ranging from underspecified interaction details to terminal compatibility gaps that could cause user frustration if not addressed before implementation.

---

## Specific Issues

### Issue 1: Badge Pulse Animation Is Not Testable As Specified

- **Location:** AC-1.4 ("Badge pulses after 5 minutes for unreviewed high-relevance findings")
- **Problem:** "Pulse animation" in a terminal has extremely narrow implementation options. Bubble Tea v1 renders at ~60 FPS, but terminal "pulse" typically means alternating between two visual states (e.g., bold/dim or two colors). The criterion says to "observe pulse animation" but does not specify: (a) what the visual states are, (b) the pulse frequency, (c) how it degrades on terminals that lack color support, or (d) how to verify it programmatically. Manual testers will disagree on whether a given visual effect counts as "pulsing."
- **Suggestion:** Define the pulse as a concrete two-state alternation: e.g., the badge background alternates between `ColorWarning` (#e0af68) and `ColorBgLighter` (#292e42) at 1-second intervals. For terminals without 256-color support, degrade to bold/normal alternation. The test criterion should be: "badge style changes at least once per 2 seconds when findings are unreviewed for >5 minutes." Add a `--reduce-motion` flag or environment variable (`NO_ANIMATION=1`) that replaces the pulse with a static indicator, consistent with accessibility best practices. This is especially relevant because Autarch users working over SSH or in tmux may find rapid redraws distracting.

### Issue 2: The 5-Minute Pulse Delay Might Be Too Long or Too Short

- **Location:** AC-1.4 and the Timing Thresholds table ("Badge pulse for unreviewed: 5 min")
- **Problem:** 5 minutes is described as "nudge without pressure," but the plan does not account for workflow context. During an active sprint phase (which lasts 2-10 minutes per phase), 5 minutes means the pulse might never activate before the user advances. Conversely, during a deep-dive research review session, 5 minutes of ignoring a finding is normal -- the user is reading other findings. The fixed 5-minute threshold does not adapt to what the user is doing.
- **Suggestion:** The 5-minute default is reasonable as an initial value. Make it configurable via `.pollard/config.yaml` and document that it is advisory. More importantly, AC-1.4 should clarify that the pulse activates per-finding (5 minutes since that specific finding arrived) rather than globally (5 minutes since any finding arrived). Per-finding timers prevent the badge from pulsing continuously once research is actively producing results, which would train users to ignore it.

### Issue 3: 3-Pane Layout Width Discrepancy Between Plan and Code

- **Location:** AC-X.3 ("TUI 3-pane layout renders correctly at terminal width >=120 columns") vs. `MinShellWidth` in `/root/projects/Autarch/pkg/tui/shelllayout.go` (line 11: `const MinShellWidth = 100`)
- **Problem:** The plan states a requirement of >=120 columns, but the codebase enforces a minimum of 100 columns. The plan's "Assumptions" section also says "terminal width >=120 columns for 3-pane layout." Meanwhile, the `SplitLayout` has its own `minWidth` of 100 for the stacked fallback. There are two inconsistencies: (a) which width is actually required for the 3-pane layout, and (b) what happens between 100-119 columns.

  At 120 columns with the sidebar expanded (28 columns + 2 separator = 30), the remaining content area is 90 columns split 50/50 = 45 columns per pane. That is very tight for a doc pane displaying PRD content and a chat pane displaying conversation. At 100 columns the content area would be 70 columns, yielding 35 columns per pane -- barely usable.

- **Suggestion:** Reconcile the threshold. If 120 is the real requirement, update `MinShellWidth` in the code. If 100 is acceptable, update the plan. I recommend 120 for the full 3-pane layout and adding a 2-pane fallback (doc + chat, sidebar hidden) for 100-119 columns, which the `SplitLayout` stacked mode already partially supports. AC-X.3 should test at 100, 119, 120, and 200 columns, covering the boundary conditions.

### Issue 4: Terminal Width <120 "Graceful Degradation" Is Underspecified

- **Location:** AC-X.4 ("Terminal width <120 columns displays graceful degradation message")
- **Problem:** Showing a static "terminal too narrow" message when the user has a 100-column terminal (which is extremely common in side-by-side tmux panes) is not graceful degradation -- it is a dead end. The current `renderWidthError()` in `shelllayout.go` shows "Terminal too narrow / Minimum width: 100 characters," which is a full blocker. Users who routinely work in 80- or 100-column terminals will see this immediately and have no recourse except resizing, which may not be possible (e.g., tiling window manager, iPad terminal, split tmux).
- **Suggestion:** Define three tiers: (1) >=120 columns: full 3-pane layout. (2) 100-119 columns: auto-collapse sidebar, show doc+chat only. The user can still accomplish all tasks; they just lose the navigation sidebar (they can use slash commands instead). (3) <100 columns: stacked layout (doc above chat) with a one-line banner "narrow mode -- /help for commands." (4) <60 columns: show the error message. This matches how professional TUIs like lazygit, k9s, and btop handle small terminals -- they drop panes progressively rather than showing an error.

### Issue 5: Agent Pane Natural Language Triage Is Unreliable for Deterministic Testing

- **Location:** AC-1.6 ("Agent pane accepts natural language triage ('reject--not our market') and logs action to `.pollard/feedback.yaml`")
- **Problem:** Natural language interpretation is inherently non-deterministic. The acceptance criterion requires that typing "reject--not our market" results in a reject action logged to feedback.yaml. But the agent is an LLM. It might interpret "not our market" as a reason to defer rather than reject. It might hallucinate additional actions. The verification method ("Issue triage command, verify YAML append with action + reasoning") assumes reliable intent extraction. This creates a test that can pass or fail based on model temperature, prompt engineering quality, or API latency -- not based on code correctness.
- **Suggestion:** Split into two criteria. (1) **Deterministic criterion (AC-1.6a):** The triage API/function accepts structured input `{action: "reject", reasoning: "not our market", finding_id: "F-001"}` and logs it correctly. This tests the plumbing. (2) **Agent integration criterion (AC-1.6b):** The agent interprets "reject--not our market" and produces the correct structured triage call. This tests agent quality and should have a softer pass threshold (e.g., "correct intent extraction in >80% of 10 attempts") and should be clearly marked as requiring human judgment to evaluate. Mixing deterministic and non-deterministic behavior in a single acceptance criterion is a common source of flaky tests.

### Issue 6: Button Fallback UX Is Not Specified in Enough Detail

- **Location:** AC-X.6 ("Agent pane unavailability falls back to button-based triage in doc pane") and Open Question #2
- **Problem:** The plan mentions a "button fallback" but does not define what a "button" looks like in a TUI. Bubble Tea has no built-in button component. The plan's open question #2 recommends a "mandatory dropdown (Wrong Market / Already Addressed / Defer to V2 / Other)" but dropdowns are also not a standard TUI primitive. The word "button" implies a clickable element, but Autarch's TUI is keyboard-driven (mouse support is not mentioned anywhere in the codebase).

  The real question is: what does the triage interaction look like without the agent? The user needs to perform Accept/Reject/Defer/Deep Dive on a finding. In a keyboard-driven TUI, these are most naturally mapped to keybindings or inline slash commands.

- **Suggestion:** Define the fallback explicitly. When the agent pane is unavailable, the doc pane footer shows action hints: `[A]ccept  [R]eject  [D]efer  [/]Deep Dive`. Pressing R opens a reason picker (using the existing `CommandPicker` pattern) with options like "Wrong Market", "Already Addressed", "Defer to V2", "Other." This reuses existing UI patterns (the slash command fuzzy picker is already built) and avoids inventing a new "button" or "dropdown" component. AC-X.6 should specify: "doc pane shows triage action hints; each action is executable via single keypress; reject/defer actions require reason selection from a picker."

### Issue 7: Edit Preview "Modal" Conflicts with Fullscreen TUI Conventions

- **Location:** AC-1.7 ("Accept action opens edit preview showing affected spec section with suggested changes; user can modify before confirming")
- **Problem:** The criterion says "preview modal with editable diff." In a fullscreen Bubble Tea TUI, modals are overlays that consume the entire update loop. An "editable diff" requires a text editor component, which is significantly complex -- it needs cursor movement, selection, copy-paste, undo, and ANSI-aware rendering. The plan does not specify whether this is a read-only diff view (show the change, accept/reject) or a full inline editor (modify the suggested text).

  If it is a full inline editor, this is a major UI component that does not exist in the codebase. Bubble Tea's `textarea` component handles basic text input but not diff-style editing with highlighted additions/deletions.

- **Suggestion:** For v1, define the edit preview as a read-only diff view with three actions: "Apply", "Edit in $EDITOR" (opens the file in the user's configured editor, like `$EDITOR` or `vim`), or "Cancel." The diff view uses the existing `diff.go` in `pkg/tui/` (which already exists). This avoids building an inline editor while still giving the user full editing capability via their preferred tool. Reword AC-1.7 to: "Accept action shows a read-only diff of suggested changes; user can apply directly, open in external editor for modification, or cancel."

### Issue 8: 2-Second Confidence Update Target Conflicts with UX Perception

- **Location:** AC-1.8 ("Confidence score ... updates within 2 seconds of triage action") and the Timing Thresholds table
- **Problem:** 2 seconds is described as reasonable for "local computation." But after the user performs a triage action (which feels instant -- they press a key), a 2-second delay before the confidence score updates creates a perception gap. The user acts, then waits, then sees the result. This makes the system feel sluggish for an operation that should feel instantaneous.

  Conversely, if the confidence calculation is truly local (no network, no LLM), 2 seconds is an extremely generous budget that suggests the calculation is more complex than it needs to be. If it involves an LLM call, 2 seconds may be too tight.

- **Suggestion:** Split the UX into two stages. (1) Immediately after triage action (<100ms): show a "recalculating..." indicator on the confidence score, confirming the action was received. (2) Within 2 seconds: show the updated score. This removes the perception of sluggishness. If the calculation is purely local, target <200ms instead of <2s. AC-1.8 should be: "Confidence score shows 'updating' indicator within 100ms of triage action and displays recalculated value within 2 seconds."

### Issue 9: Research Triage Workflow (Accept/Reject/Defer/Deep Dive) Needs Keyboard Flow Design

- **Location:** CUJ-1 (AC-1.5 through AC-1.9), the Pollard 3-pane layout description
- **Problem:** The plan describes the Pollard tab as having a 3-pane layout with sidebar (filter categories), doc pane (finding detail), and agent pane (triage conversation). But the keyboard flow for triaging a finding is not specified. Consider the typical workflow:

  1. User is in Pollard tab, sidebar focused on "Inbox"
  2. User navigates down to a finding (arrow keys in sidebar)
  3. Finding detail appears in doc pane
  4. User reads the detail
  5. User decides to reject it

  At step 5: How does the user trigger "reject"? Options: (a) Tab to agent pane, type "reject," press Enter. (b) Press a keybinding. (c) Use a slash command `/reject`. The plan implies (a) via natural language, but that is 3+ keystrokes (Tab, type, Enter) vs. 1-2 for a keybinding. For triaging 10+ findings, the natural-language-only path is tedious.

- **Suggestion:** Add Pollard-specific slash commands analogous to the existing Sprint and EpicReview commands in `command_picker.go`. For example:
  ```
  /accept (/a) -- Accept current finding
  /reject (/rej) -- Reject current finding
  /defer (/def) -- Defer current finding
  /dive (/dd) -- Deep dive on current finding
  ```
  These complement the natural language path. Fast triagers use slash commands; deliberative users use the agent pane for nuanced decisions. This matches the existing pattern where sprint phases have both slash commands (`/accept`) and chat-based refinement. Add AC-1.5a: "Pollard tab provides slash commands for Accept, Reject, Defer, Deep Dive actions as alternatives to agent pane natural language."

### Issue 10: Log Pane Hunter Activity (AC-1.16) Needs Rate Limit UX

- **Location:** AC-1.16 ("Rate-limited hunter displays retry countdown in log pane")
- **Problem:** The log pane streams hunter activity messages. When a hunter hits a rate limit, the plan says to show a "retry countdown." But a countdown that updates every second in the log pane will produce a flood of log lines ("GitHub Scout: retrying in 58s... 57s... 56s...") that drowns out other hunter activity. Log panes are append-only by convention; in-place updates require cursor manipulation that conflicts with the scrollback model.

  The existing `LogPane` in `/root/projects/Autarch/pkg/tui/logpane.go` is an append-only log viewer. In-place updates would require a different rendering model.

- **Suggestion:** Show a single log line when the rate limit is hit ("GitHub Scout: rate limited, retrying at 14:32:05") and a single line when the retry succeeds or fails. Do not show a ticking countdown. If a visual countdown is important, show it as a status indicator in the sidebar's hunter status icon (e.g., the icon changes from the running indicator to a clock icon with remaining time), where in-place updates are natural. Reword AC-1.16 to: "Rate-limited hunter logs a single message with retry timestamp; hunter status in sidebar shows rate-limited state; other hunters continue in parallel."

### Issue 11: Copy-Paste Concerns for Fullscreen TUI

- **Location:** Cross-cutting -- affects all CUJs
- **Problem:** The plan does not address copy-paste. The TUI runs in fullscreen (alternate screen buffer) by default. Users who want to copy a finding summary, a competitor name, or an error message cannot easily do so. Terminal copy requires mouse selection (which may conflict with mouse-enabled Bubble Tea), or the user must switch to inline mode (`--inline`). The plan has no acceptance criterion for copying content out of the TUI.

  This is particularly relevant for the research triage workflow: users will want to copy URLs from findings, copy competitor names for further research, or paste finding summaries into external documents.

- **Suggestion:** Add AC-X.11: "Users can copy the currently displayed finding or doc pane content to the system clipboard via a slash command (`/copy`) or keybinding (`Ctrl+Y`)." Implementation can use OSC 52 escape sequences (supported by most modern terminals including tmux with `set -g set-clipboard on`). For terminals that do not support OSC 52, fall back to writing to a temp file and logging the path. This is a significant quality-of-life feature for a research-focused tool.

### Issue 12: Stacked Degradation Matrix Lacks TUI-Specific Criteria

- **Location:** Degradation Matrix section
- **Problem:** The degradation matrix covers Agent Teams x Intermute availability combinations but does not address TUI-specific degradation scenarios:
  - What happens when the user's agent (Claude Code, Codex) is unavailable? The agent pane is central to CUJ-1 and CUJ-3.
  - What happens when WebSocket connections fail mid-session? AC-5.3 says updates within 2 seconds but does not specify reconnection behavior visible to the user.
  - What happens when Pollard hunters all fail (no network)? The user is in a sprint expecting research that will never arrive.

  AC-X.6 covers "agent pane unavailability" with a button fallback, but this is a single criterion for what could be multiple failure modes (agent crashes, agent disconnects, agent is slow, agent is misconfigured).

- **Suggestion:** Add TUI-specific degradation criteria. AC-X.12: "When all Pollard hunters fail, the Research component of confidence shows 0% with an explanation ('no research available -- network error') and the sprint continues without blocking." AC-X.13: "When WebSocket connection drops, the dashboard shows a 'reconnecting...' indicator and automatically reconnects within 10 seconds; stale data is visually marked." AC-X.14: "Agent pane shows connection status (connected/disconnected/reconnecting) in its header bar."

---

## Terminal-Specific Concerns

### Color Accessibility

The plan relies on color-coded indicators throughout:
- Sidebar icons: running, partial, complete
- Badge states: normal vs. pulsing
- Finding icons: competitive, trends, user
- Severity levels: info/warning/critical

The Tokyo Night palette (`/root/projects/Autarch/pkg/tui/colors.go`) uses 24-bit hex colors (`#7aa2f7`, etc.). Terminals that only support 16 or 256 colors will render these incorrectly. The plan has no acceptance criterion for color fallback.

**Recommendation:** Add AC-X.15: "All status indicators are distinguishable by shape/icon alone, not only by color. For example, running=spinner, partial=half-circle, complete=checkmark. When `TERM` does not support true color, the TUI falls back to the terminal's default 16-color palette using lipgloss's adaptive color feature." The existing sidebar items already use distinct icons (`SidebarItem.Icon`), so this is partially addressed -- just needs to be made an explicit criterion.

### Screen Real Estate at Minimum Width

At 120 columns (the plan's stated minimum) with the sidebar expanded:
- Sidebar: 28 columns
- Separator: 2 columns
- Content area: 90 columns
- Split 50/50: 45 columns per pane (doc + chat)
- Minus separator: ~44 usable columns per pane

44 columns is very tight for:
- Displaying PRD content with section headers and nested lists
- Displaying chat conversation with code blocks
- Showing research findings with source URLs

At this width, line wrapping will be aggressive and readability will suffer.

**Recommendation:** Consider using a 60/40 split for the doc/chat panes instead of 50/50 at narrower widths. The `SplitLayout` already supports configurable ratios (the constructor takes `leftRatio`). At 120 columns: doc pane gets 54 columns (readable), chat pane gets 36 columns (tight but workable for a chat interface). Add adaptive ratio switching based on terminal width.

### Inline vs. Fullscreen

The plan does not distinguish between `--inline` mode and fullscreen mode for any acceptance criteria. The `--inline` flag is documented in `AGENTS.md` and `CLAUDE.md` as preserving terminal scrollback. Some criteria (particularly AC-1.4 badge pulse, AC-X.3 layout) may behave differently in inline mode because inline mode cannot control the full terminal viewport.

**Recommendation:** Add a note to the Test Categories section: "AC-1.4, AC-X.3, and AC-X.4 should be tested in both fullscreen (default) and inline (`--inline`) modes. Inline mode may have different rendering constraints for animation and full-screen layout."

---

## Assessment of Specific Review Questions

### 1. Are the TUI interaction criteria (badge pulse, 3-pane layout, button fallback) testable and well-specified?

**Partially.** The 3-pane layout (AC-1.5) is well-specified -- "sidebar (Inbox/Accepted/Rejected/Deferred + hunter status), doc pane (finding detail), agent pane (triage conversation)" gives clear structural requirements. However:
- Badge pulse (AC-1.4) is not testable without a concrete visual definition (see Issue 1).
- Button fallback (AC-X.6) references a UI primitive that does not exist in the TUI framework (see Issue 6).
- The 3-pane layout does not specify keyboard navigation flow between panes for the Pollard tab specifically (see Issue 9).

### 2. Do timing criteria (5-minute pulse, 2s updates) make UX sense?

**Mostly.** The plan's research-informed timing separation (correctness targets vs. performance budgets in the Timing Thresholds Summary) is excellent UX practice. Specific concerns:
- The 5-minute pulse timer is reasonable but should be per-finding, not global (see Issue 2).
- The 2-second confidence update should include an immediate "calculating" indicator (see Issue 8).
- The <60 second "first finding visible" target is aggressive given that hunters rely on free API tiers with rate limits. It would be more honest to set the correctness target as "finding visible after hunter returns result" and the performance budget as <60s for the fastest hunter.

### 3. Is the research triage workflow (Accept/Reject/Defer/Deep Dive) ergonomic for a terminal?

**Not yet.** The natural language agent pane is a powerful interaction model, but it is too slow as the *only* path for routine triage decisions (see Issue 9). The workflow needs:
- Slash commands for fast triage (`/accept`, `/reject`, `/defer`, `/dive`)
- A reason picker for reject/defer (reusing the `CommandPicker`)
- Keybinding hints in the doc pane footer when the agent is unavailable
- The ability to batch-triage (e.g., "reject all findings from HackerNews older than 7 days") via the agent pane

The mix of fast keybindings for routine decisions and natural language for complex reasoning is the right UX pattern. The plan just needs to explicitly specify both paths.

### 4. Does terminal resize handling (>=120 cols) cover edge cases?

**No.** The plan specifies only two states: >=120 columns (full layout) and <120 columns (degradation message). The codebase has `MinShellWidth = 100`. There is no intermediate state. Edge cases not covered:
- 100-119 columns (common in split tmux panes)
- Dynamic resize during active sprint (Bubble Tea sends `tea.WindowSizeMsg` but the plan does not specify smooth re-layout)
- Height constraints (the plan only discusses width; very short terminals like 24 rows may truncate critical UI elements)

See Issue 3 and Issue 4 for detailed recommendations.

### 5. Are agent pane interactions (natural language triage) realistic for a TUI?

**Conditionally.** The concept is sound -- the agent pane is a chat interface, and natural language triage ("reject -- not our market") is intuitive. But there are practical concerns:
- LLM latency (1-5 seconds per response) makes the workflow feel slower than keyboard shortcuts for simple decisions.
- Non-deterministic interpretation makes testing unreliable (see Issue 5).
- The agent pane requires a running agent session (Claude Code, Codex, etc.) which adds a dependency that may not always be available.

The plan correctly addresses the third concern with AC-X.6 (button fallback), but the first two remain. The recommendation in Issue 5 (split deterministic from non-deterministic criteria) and Issue 9 (add slash command alternatives) addresses these.

---

## Summary

**Overall UX impact: Improvement with caveats.**

The acceptance criteria plan is thorough on the backend (degradation matrix, race condition testing, data integrity) and makes several excellent UX decisions (non-disruptive research surfacing, progressive confidence scoring, advisory-not-blocking export). However, it is underspecified on the TUI interaction layer -- the space between "what the user sees" and "what the system does."

### Top 3 Changes for Better UX

1. **Add Pollard triage slash commands** (Issue 9). Without `/accept`, `/reject`, `/defer`, `/dive`, the research triage workflow forces all decisions through natural language, which is slow for routine triage. This is the single highest-impact UX change because triage is the most repetitive user interaction in the plan.

2. **Define progressive width degradation** (Issues 3 and 4). Replace the binary "works / shows error" behavior with three tiers (full 3-pane / 2-pane with collapsed sidebar / stacked). Users in split tmux panes or smaller terminals should still be able to use the tool, just with fewer visible panes. This directly affects how many developers can actually use Autarch in their real terminal setup.

3. **Split AC-1.6 into deterministic and agent-quality criteria** (Issue 5). Mixing LLM interpretation testing with plumbing correctness testing will produce flaky acceptance criteria that erode confidence in the test suite. The plumbing should be rock-solid; the agent quality should be measured separately.

---

## Files Referenced

| File | Relevance |
|------|-----------|
| `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md` | The plan under review |
| `/root/projects/Autarch/pkg/tui/shelllayout.go` | 3-pane layout implementation; `MinShellWidth = 100` conflicts with plan's 120-column requirement |
| `/root/projects/Autarch/pkg/tui/splitlayout.go` | Doc+chat split layout; supports configurable ratios and stacked fallback |
| `/root/projects/Autarch/pkg/tui/command_picker.go` | Slash command system; `GlobalCommands()` already has tool-switching; missing Pollard triage commands |
| `/root/projects/Autarch/pkg/tui/sidebar.go` | Sidebar implementation; 28-column fixed width; icon-based items |
| `/root/projects/Autarch/pkg/tui/colors.go` | Tokyo Night palette using 24-bit hex colors; no adaptive fallback for 16/256-color terminals |
| `/root/projects/Autarch/pkg/tui/logpane.go` | Append-only log pane; countdown timer (AC-1.16) conflicts with append model |
| `/root/projects/Autarch/docs/tui/SHORTCUTS.md` | Keyboard shortcut conventions; "printable keys are not bound anywhere" policy |
| `/root/projects/Autarch/docs/plans/2026-02-05-unified-tui-navigation-design.md` | Navigation plan context; Phase 1 done, Phase 2 pending |
| `/root/projects/Autarch/AGENTS.md` | Development guide; TUI keybindings, sprint timing, slash command reference |
