# UX Review: Unified TUI Navigation Design

**Reviewer:** UX Reviewer (CLI/TUI Specialist)
**Date:** 2026-02-06
**Plan reviewed:** `/root/projects/Autarch/docs/plans/2026-02-05-unified-tui-navigation-design.md`
**Verdict:** Net positive with 7 specific issues, 1 at P0 severity

---

## Summary

The Unified TUI Navigation Design plan makes strong choices for Phase 1 (always-visible tabs, slash commands, Ctrl+Left/Right cycling). The slash command approach is portable and the decision to drop Ctrl+N/Alt+N keybindings reflects genuine terminal compatibility awareness. However, the plan has a P0 problem in Phase 3: `Ctrl+Shift+S` is not reliably detectable in Bubble Tea v1 across common terminal configurations, making it a dead keybinding. The Signals overlay section is the thinnest part of the plan and needs depth on dismiss behavior, focus trapping, and screen-size degradation. Phase 2 is well-scoped given the entanglement analysis but would benefit from specifying what happens to in-flight onboarding state during a tab switch.

---

## Section-by-Section Review

### Tab Bar Always Visible

The visual design is clear. Four tabs with the active tab highlighted in `ColorPrimary` with bold text is a conventional and effective pattern. The tab rendering code in `/root/projects/Autarch/internal/tui/tabs.go` uses `lipgloss.NewStyle().Background(pkgtui.ColorPrimary).Foreground(pkgtui.ColorBg).Bold(true).Padding(0, 2)` for the active tab, which is readable on dark backgrounds. Inactive tabs use `ColorFgDim`, creating adequate contrast.

The tab bar currently occupies one text line plus padding. The `View()` function at line 1904 of `/root/projects/Autarch/internal/tui/unified_app.go` calculates `headerHeight = 3` for dashboard mode (1 line content + Padding(1,3) from HeaderStyle = 3 total). This is compact. Good.

Removing Signals as a fifth tab is the right call. Signals is a monitoring surface, not a workspace. Users don't "work in" Signals the way they work in Gurgeh or Coldwine. Four tabs keep the tab bar scannable at a glance.

### Keybindings

The slash command approach (`/big`, `/gur`, `/cold`, `/pol`) is well-validated. I verified in `/root/projects/Autarch/pkg/tui/command_picker.go` (lines 320-324) that these are already implemented as `GlobalCommands()` entries with `"navigation"` category. The fuzzy matcher in the same file supports prefix matching, so typing `/b` will surface both `/back` and `/bigend`, with prefix matches ranked first (line 152-158). This is discoverable without being ambiguous.

The three-letter alias strategy is sound. The collision analysis in the plan (Finding 2) correctly identifies that `/b`, `/p`, `/g` are taken. The chosen aliases are mnemonic and fast to type.

`Ctrl+Left/Right` for tab cycling is already implemented at line 513-516 of `unified_app.go`, with `Ctrl+PgUp/PgDn` as fallbacks. These work in both onboarding and dashboard modes, which is important since Phase 1 keeps both modes alive.

### Slash Commands

The implementation in `GlobalCommands()` is clean. One concern: the `fuzzyMatch` function in `command_picker.go` (lines 165-190) matches on description text too, which means typing `/switch` would match all four navigation commands (they all say "Switch to..."). This is fine for discoverability but means the fuzzy picker will show 4 near-identical entries for generic queries. Not a blocking issue, but the descriptions could be more differentiated (e.g., "Mission control", "PRD generation", "Task orchestration", "Research intelligence" instead of "Switch to X") to help users distinguish them when browsing.

### Simplified Mode Architecture

The plan correctly identifies the two-path problem (Finding 4): `App` vs `UnifiedApp` are completely separate implementations. Phase 2 proposes merging them, which is the right long-term move.

The startup behavior change (defaulting to Gurgeh tab active, `--skip-onboard` becoming a no-op) is reasonable. The `--tool=` flag provides the escape hatch for users who want a different default. However, the plan does not specify what happens when a user is mid-onboarding (say, in Phase 5 of a sprint) and presses `Ctrl+Right` to switch tabs. The current `switchToTab()` at line 1876 calls `enterDashboard()` when in onboarding mode, which creates the dashboard views and switches to ModeDashboard. This means the user's in-progress sprint state is abandoned. The plan should state whether this is intentional or whether sprint state should be preserved (it is persisted to `.gurgeh/sprints/`, so the data is not lost, but the user might not expect to lose their place).

### Signals Overlay

This is the thinnest section and needs the most attention. The visual mockup shows a centered overlay, which matches the existing `overlay()` function at line 2103 of `unified_app.go`. The overlay function positions content at `(height - overlayHeight) / 4` vertically and horizontally centered. This is battle-tested for the palette and help overlay.

However, the section does not address:

1. **Dismiss behavior** -- Is the overlay dismissed on Esc? On any keypress? Only on the toggle key? The palette and help overlay both dismiss on Esc, which should be the convention.

2. **Focus trapping** -- When the overlay is visible, do keypresses go to the overlay or the underlying view? The palette traps focus (line 453-457 of `unified_app.go` checks `a.palette.Visible()` before passing keys to the view). The Signals overlay should do the same, but the plan does not specify whether users can interact with signals (navigate, filter, dismiss) or just read them.

3. **Size and content limits** -- The current `SignalsView` at `/root/projects/Autarch/internal/tui/views/signals.go` is a full 3-pane view with sidebar, doc, and chat areas. The plan says to create a "simplified rendering, no 3-pane layout" but does not specify what the overlay actually shows. Is it a list of recent signals with timestamps? A count badge? The ASCII mockup shows 3 signal entries, but what if there are 50? The overlay needs scrolling or truncation.

4. **Small terminal behavior** -- At 80x24 (the POSIX minimum), the overlay will dominate the screen. The plan should specify a maximum overlay height (e.g., `min(signalCount + 2, height / 2)`) to avoid obscuring the entire underlying view.

---

## Issues Found

### Issue 1 (P0): Ctrl+Shift+S Is Not Detectable in Bubble Tea v1

- **Location:** Phase 3 Keybindings and Signals Overlay sections
- **Problem:** The plan specifies `Ctrl+Shift+S` as the keybinding for toggling the Signals overlay. Bubble Tea v1 does not negotiate the Kitty keyboard protocol, which means it cannot distinguish `Ctrl+Shift+S` from `Ctrl+S` in most terminals. In the standard VT100/xterm encoding, `Ctrl+S` sends byte 0x13 (DC3/XOFF), and adding Shift does not change the byte value. Some terminals (Kitty, WezTerm, Ghostty) can encode the Shift modifier using CSI u sequences, but only if the application negotiates the protocol -- which BT v1 does not.

  I verified this by searching the entire Go codebase: zero files contain any handling of `ctrl+shift` combinations (`grep -ri "ctrl+shift" *.go` returns no matches). The project's own MEMORY.md already documents that "Ctrl+number keybindings don't work in Bubble Tea v1" for the same underlying reason -- BT v1's input parser does not support enhanced key encoding.

  This means `Ctrl+Shift+S` will either: (a) be indistinguishable from `Ctrl+S` (which is already bound to "Scan" in kickoff mode, line 161 of signals.go: `msg.Type == tea.KeyCtrlS`), or (b) be swallowed entirely by the terminal (XON/XOFF flow control on terminals that have not disabled it), or (c) work only on a narrow set of modern terminals that the team happens to test on.

- **Suggestion:** Replace `Ctrl+Shift+S` with one of: (a) `/signals` slash command only (already implemented as a no-op at line 411 of unified_app.go), which is the portable choice consistent with the Phase 1 approach; (b) `F8` or another unoccupied function key, which BT v1 handles reliably; (c) `Ctrl+\` (SIGQUIT, but can be trapped -- though this is risky). The strongest option is (a): make `/signals` (or `/sig`) the primary access method and drop the dedicated keybinding entirely. This is consistent with how tool-switching already works -- there is no `Ctrl+B` for Bigend or `Ctrl+G` for Gurgeh; they all use slash commands. Adding a dedicated keybinding only for Signals would be inconsistent with the navigation model established in Phase 1.

### Issue 2 (P1): Tab Switch During Onboarding Silently Exits Sprint

- **Location:** Simplified Mode Architecture section and `switchToTab()` implementation
- **Problem:** The `switchToTab()` function at line 1876 of `unified_app.go` calls `enterDashboard()` when `a.mode == ModeOnboarding`. This transitions the app out of onboarding mode and creates fresh dashboard views. If the user was mid-sprint (say, Phase 5 of 8 in the Gurgeh arbiter), pressing `Ctrl+Right` or typing `/cold` exits their sprint without warning. The sprint state is auto-saved to `.gurgeh/sprints/`, so data is not lost, but the user will need to manually resume it later.

  The plan does not address this transition. A user who just wants to glance at their Coldwine tasks should not lose their current position in the Gurgeh sprint flow.

- **Suggestion:** Add a confirmation prompt when switching tabs during an active onboarding sprint: "Sprint in progress. Switch tab? (sprint will be saved)" with Y/N. Alternatively, and more elegantly: when the user switches away from Gurgeh and later switches back, automatically resume the saved sprint. This requires that `switchToTab()` preserve the onboarding state rather than discarding it, and that returning to the Gurgeh tab restores the sprint view. The plan should specify which approach is used, and Phase 1 implementation should handle this case even if the full fix is deferred to Phase 2.

### Issue 3 (P1): Signals Overlay Content and Interaction Model Unspecified

- **Location:** Signals Overlay section (Phase 3)
- **Problem:** The plan's overlay mockup shows three signal entries, but the overlay's interaction model is completely unspecified. The existing `SignalsView` is a rich 3-pane view with sidebar navigation (Signals/Events/Conflicts), source filters (`Ctrl+S`), type filters (`Ctrl+T`), severity filters (`Ctrl+V`), and Intermute WebSocket integration. The plan says to create a "simplified rendering, no 3-pane layout" but does not define what "simplified" means. Questions left unanswered:

  - Can the user navigate the signal list (up/down)?
  - Can the user dismiss or acknowledge a signal?
  - Is there filtering?
  - What is the maximum number of signals shown?
  - Does the overlay auto-update via WebSocket while visible?
  - What happens when the overlay is opened and there are no signals?

  Without answering these, "200-300 lines" is an unreliable estimate because the scope could range from a read-only badge popup (100 lines) to a mini-dashboard with interaction (500+ lines).

- **Suggestion:** Define the overlay as a read-only notification list with a fixed scope: show the N most recent signals (where N = `min(10, availableHeight / 2)`), sorted by recency, with severity icon, title, and relative timestamp. No filtering, no interaction beyond scrolling and dismiss (Esc). If the user wants to interact with signals, they use the full-screen SignalsView (which could be accessible via `/signals --full` or by pressing Enter from the overlay to "expand" it into a full view). This keeps the overlay lightweight (closer to 200 lines) and defers interaction complexity.

### Issue 4 (P1): Footer Help Text Is Truncated on Narrow Terminals

- **Location:** `renderFooterContent()` at line 1997 of `unified_app.go`
- **Problem:** The footer concatenates the view's `ShortHelp()` with global navigation hints:
  ```
  [view help]  |  /big /gur /cold /pol  ctrl+l logs  ctrl+p palette  ctrl+, settings  /help  ctrl+c*2 quit
  ```
  At 80 columns, this string exceeds the available width. For example, the Signals view's `ShortHelp()` alone is 96 characters:
  ```
  updown navigate  ctrl+r refresh  ctrl+s source  ctrl+t type  ctrl+v severity  tab focus  ctrl+b sidebar
  ```
  Combined with the global suffix (78 characters including the separator), the total is 174+ characters. At 80 columns, the terminal will either wrap (breaking the single-line footer convention) or truncate (losing critical keybinding hints). The plan does not address this because it does not change the footer, but by adding four new slash commands to the footer text, it exacerbates an existing problem.

- **Suggestion:** Implement a two-tier footer. At >=120 columns, show the full text. At <120 columns, show only the most essential hints: `/help  ctrl+c quit`. The view-specific help and navigation commands are discoverable via `/help` overlay. Alternatively, use ANSI truncation with an ellipsis: if the footer exceeds the terminal width, truncate from the right and append "... /help for more". The existing `ansi.Truncate()` function (already used in `insertAt()` at line 2137) can do this.

### Issue 5 (P2): Tab Bar Does Not Indicate Notification State

- **Location:** Tab Bar Always Visible section
- **Problem:** The plan removes Signals from the tab bar, which means the user loses the one place where signal count was visible at all times. The plan proposes an overlay as the replacement, but the overlay is toggled -- the user must actively open it. Between overlay opens, there is no passive notification that new signals have arrived.

  Consider: the user is deep in a Gurgeh sprint, Phase 6 Requirements. A competitor ships a major release (signal: `competitor_shipped`). The user has no way to know this happened unless they remember to open the overlay. In the current 5-tab design, the Signals tab name could show a count badge ("Signals (3)").

- **Suggestion:** Add a notification indicator to the tab bar. When the Signals overlay has unread/unacknowledged signals, show a dot or count next to a tab or in the header area. For example, the tab bar could show `[Gurgeh] Coldwine Pollard *` where `*` is a signal indicator in `ColorWarning`. This is a lightweight change to the `TabBar.View()` function in `/root/projects/Autarch/internal/tui/tabs.go` (add a `notifications int` field and render it after the last tab). The indicator does not need to be on a specific tab since signals are cross-tool.

### Issue 6 (P2): --skip-onboard Deprecation Path Is Incomplete

- **Location:** Simplified Mode Architecture, Startup section
- **Problem:** The plan says `--skip-onboard` becomes a "no-op with warning" in Phase 2. But Phase 1 does not change `--skip-onboard` behavior at all, and Phase 2 removes `ModeOnboarding` entirely. This means `--skip-onboard` goes from "does something useful" (Phase 1, using the `App` code path) to "does nothing and prints a warning" (Phase 2) with no intermediate state. Users who have `--skip-onboard` in their shell aliases or scripts will experience a behavior change with no migration path.

  Furthermore, the plan does not specify what the warning looks like. Is it a stderr message before the TUI starts? A banner inside the TUI? A log entry? The user needs to know what to change their alias to.

- **Suggestion:** In Phase 2, print a clear stderr warning before launching the TUI: `Warning: --skip-onboard is deprecated and will be removed. Use --tool=gurgeh to start directly in Gurgeh, or omit the flag for the default experience.` This gives users an actionable migration instruction. In a subsequent release, remove the flag entirely with a hard error pointing to the docs. Do not silently ignore the flag -- users should know their invocation changed.

### Issue 7 (P2): Slash Command Descriptions Are Generic

- **Location:** Slash Commands section and `GlobalCommands()` in `/root/projects/Autarch/pkg/tui/command_picker.go`
- **Problem:** The current descriptions for navigation commands are "Switch to Bigend", "Switch to Gurgeh", etc. When the fuzzy picker shows these, they all look identical except for the tool name. For new users who do not know what each tool does, these descriptions provide no guidance. The picker already supports a `Description` field -- it should be used to describe the tool, not the action.

- **Suggestion:** Change descriptions to match the tool purposes:
  - `/bigend` -- "Mission control dashboard"
  - `/gurgeh` -- "PRD generation and validation"
  - `/coldwine` -- "Task orchestration"
  - `/pollard` -- "Research intelligence"
  - `/signals` -- "Signals and events overlay"

  These descriptions are already used in `AGENTS.md` line 1-13 and the tab descriptions in the plan itself. Using them in the command picker creates consistency and helps new users navigate without consulting documentation.

---

## Improvements Suggested

### Improvement 1: Add Active Tab Indicator in Slash Command Picker

When the picker shows navigation commands, it could indicate which tab is currently active. For example, if the user is on the Gurgeh tab and opens the picker, `/gurgeh` could show "(current)" appended to its description, or the entry could be dimmed/grayed out. This prevents the user from "switching" to the tab they are already on and provides spatial orientation. Implementation: pass the active tab index to `GlobalCommands()` and modify the description conditionally.

### Improvement 2: Preserve Tab State Across Switches

When the user switches from Gurgeh to Pollard and back, the Gurgeh tab should show whatever they were looking at before -- not reset to the kickoff screen. The current `switchDashboardTab()` preserves views in `a.dashViews[]` (indexed by tab), which is correct for dashboard mode. But in the Phase 2 merged architecture, ensure that Gurgeh's internal state (which sprint phase, which scroll position) is also preserved. This is standard behavior in tabbed interfaces (every browser preserves tab state).

### Improvement 3: Consider Ctrl+Tab as an Additional Tab Cycling Shortcut

While `Ctrl+Left/Right` works and is documented, `Ctrl+Tab` is a widely recognized tab-cycling shortcut from browsers and IDEs. Bubble Tea v1 may or may not receive this key combination reliably (it depends on whether the terminal sends a distinct sequence for it), but if it works, it would provide a more intuitive shortcut for users coming from GUI applications. This is a low-priority enhancement that could be tested empirically.

---

## Overall Assessment

**Overall UX impact: Improvement.**

The plan is well-reasoned, and its phased approach is pragmatic. Phase 1 delivers the highest-value UX changes (always-visible tabs, slash commands) with the lowest risk. The decision to use slash commands instead of modifier keybindings is correct given BT v1's input limitations and the diverse terminal environments the project targets (tmux, SSH, macOS Terminal).

**Top 3 changes for better user experience:**

1. **Replace `Ctrl+Shift+S` with `/signals` only (P0).** This keybinding will not work in most terminal configurations under BT v1. Using slash commands for all navigation keeps the model consistent and avoids a dead shortcut that erodes user trust.

2. **Add a signal notification indicator to the tab bar (P1).** Removing Signals from the tab bar eliminates the only passive notification surface. A small dot or count in the header area preserves awareness without requiring the user to actively open the overlay.

3. **Specify the Signals overlay interaction model (P1).** The overlay section is too thin to implement from. Define it as read-only with N most recent signals, Esc to dismiss, and Enter to expand to full view. This scopes the 200-300 line estimate and prevents scope creep.
