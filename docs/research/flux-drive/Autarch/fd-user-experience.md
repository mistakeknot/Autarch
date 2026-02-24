---
agent: fd-user-experience
tier: 1
issues:
  - id: P0-1
    severity: P0
    section: "Keybinding Conflicts"
    title: "Ctrl+Right collision between tab cycling and sprint accept"
  - id: P0-2
    severity: P0
    section: "Keybinding Conflicts"
    title: "Ctrl+S conflicts with XON/XOFF flow control in many terminals"
  - id: P1-1
    severity: P1
    section: "Onboarding Flow"
    title: "Tab switch during active sprint silently discards state"
  - id: P1-2
    severity: P1
    section: "Error Experience"
    title: "Silent swallow of unknown slash commands with no user feedback"
  - id: P1-3
    severity: P1
    section: "Ctrl+C Double-Tap Quit"
    title: "No visual feedback on first Ctrl+C -- user thinks nothing happened"
  - id: P1-4
    severity: P1
    section: "Minimum Width Enforcement"
    title: "MinShellWidth 100 chars is too aggressive for typical terminal splits"
  - id: P1-5
    severity: P1
    section: "Dual TUI Implementation"
    title: "App and UnifiedApp diverge on Ctrl+C behavior creating inconsistent quit UX"
  - id: P1-6
    severity: P1
    section: "Document Scrolling"
    title: "Ctrl+U collision between doc panel half-page scroll and revert last run"
  - id: P2-1
    severity: P2
    section: "Command Discoverability"
    title: "Slash command descriptions describe actions not tools"
  - id: P2-2
    severity: P2
    section: "Footer Help Text"
    title: "Footer is overcrowded and truncates on narrow terminals"
  - id: P2-3
    severity: P2
    section: "Help Overlay"
    title: "Help overlay duplicate entries and inconsistent trigger documentation"
  - id: P2-4
    severity: P2
    section: "Log Pane"
    title: "g/G bindings in LogPane clash with printable-key policy when sidebar is focused"
  - id: P2-5
    severity: P2
    section: "Sidebar Navigation"
    title: "Sidebar uses j/k for navigation which conflicts with chat-focused printable-key policy"
  - id: P2-6
    severity: P2
    section: "Color Accessibility"
    title: "Hard-coded hex colors have no fallback for 16-color terminals"
improvements:
  - id: IMP-1
    title: "Add transient status bar for first Ctrl+C and unknown commands"
    section: "Error Experience"
  - id: IMP-2
    title: "Add signal notification badge/indicator to tab bar"
    section: "Tab Bar"
  - id: IMP-3
    title: "Lower MinShellWidth to 80 for tmux half-pane compatibility"
    section: "Layout"
  - id: IMP-4
    title: "Merge App and UnifiedApp into single codepath"
    section: "Architecture"
  - id: IMP-5
    title: "Add scroll position indicator to doc panel and chat history"
    section: "Information Hierarchy"
  - id: IMP-6
    title: "Contextual footer that only shows relevant shortcuts for current focus"
    section: "Footer Help Text"
verdict: needs-changes
---

# Autarch TUI User Experience Review

## Summary

Autarch presents an ambitious Bubble Tea TUI with a Cursor-style 3-pane layout (sidebar + document + chat), always-visible tabs for 4 tools, slash commands with fuzzy matching, and a multi-phase onboarding flow for PRD generation. The interaction model is thoughtfully designed around a chat-first paradigm with `Ctrl+` modifiers to avoid collisions with text input. However, the codebase carries significant UX debt from its dual-implementation architecture (`App` vs `UnifiedApp`), several keybinding conflicts that surface in real terminal environments (tmux, SSH, macOS Terminal), and an onboarding flow that is entangled with over 400 lines of transition handlers. The slash command system is well-conceived but needs better error feedback. The layout mathematics documented in project memory (lipgloss Height gotchas) suggest hard-won reliability, but the 100-character minimum width is too restrictive for typical developer workflows involving terminal splits.

## Section-by-Section Review

### 1. Keybinding Architecture (`pkg/tui/keys.go`, `internal/tui/unified_app.go`)

The keybinding design is fundamentally sound. The decision to use `Ctrl+` combinations exclusively (avoiding single-letter shortcuts) is correct for a chat-focused TUI where the composer captures printable keys. The project memory correctly identifies that Ctrl+number keybindings are dead in BT v1 and that Alt+number works via ESC prefix -- this constraint is respected in the current codebase.

The `CommonKeys` struct in `/root/projects/Autarch/pkg/tui/keys.go` defines a clean shared vocabulary. The `HandleCommon` function at line 104 only handles quit and help, delegating everything else to views. This is appropriate for a tabbed architecture where views need different key handling.

**Concern:** The `Select` and `Toggle` bindings at lines 80-87 both map to `enter`, creating ambiguity in contexts where both actions are available. In practice this is not a problem because views choose one or the other, but it could confuse future contributors reading the code.

### 2. Keybinding Conflicts (Multiple Files)

Several keybinding collisions exist in the current implementation:

**Ctrl+Right** serves double duty:
- In `unified_app.go` line 513: Tab cycling (`ctrl+right` -> next tab)
- In `sprint_view.go` line 303: Accept current draft (`tea.KeyCtrlRight`)

When a user is in the sprint view (the primary interaction surface for PRD creation), pressing Ctrl+Right will be intercepted by `unified_app.go` first (line 513 is checked before the message reaches `currentView.Update`), so the sprint view's "accept" action is unreachable via its documented keybinding. The sprint view's `ShortHelp()` at line 371 advertises "ctrl+right accept" but this binding is dead in practice.

**Ctrl+S** is documented as "scan current directory" in kickoff view (`kickoff.go` line 568) and listed in the SHORTCUTS.md at `/root/projects/Autarch/docs/tui/SHORTCUTS.md` line 24 as needing careful handling because of XON/XOFF flow control. Many terminal emulators (especially over SSH) send XOFF on Ctrl+S, freezing terminal output. The mitigation F4 fallback exists but is not documented in the footer or help overlay.

**Ctrl+U** collides between:
- Sprint view doc panel: half-page scroll up (`sprint_view.go` line 271: `"ctrl+u"`)
- UnifiedApp: revert last run (`unified_app.go` line 508: `"ctrl+u"`)

The UnifiedApp handler runs first, so `Ctrl+U` in sprint view triggers revert, not scroll.

**Ctrl+B** behavior changes between modes:
- In onboarding mode: toggles breadcrumb navigation (`unified_app.go` line 492)
- In dashboard mode: falls through to shell layout which toggles sidebar (`shelllayout.go` line 139)

This is confusing because the same key does contextually different things without any visual indicator of which mode the user is in. The tabs are always visible, so there is no obvious "onboarding vs dashboard" distinction.

### 3. Slash Command System (`pkg/tui/command_picker.go`, `pkg/tui/chatpanel.go`)

The slash command implementation is well-built. The fuzzy matching algorithm at `/root/projects/Autarch/pkg/tui/command_picker.go` line 165 correctly checks for both substring contains and ordered-character matching, with prefix matches sorted first. The command picker UI has proper scrolling with a "more below" indicator.

The command registration is cleanly organized by context: `GlobalCommands()`, `KickoffCommands()`, `SprintCommands()`, `EpicReviewCommands()`, `TaskReviewCommands()` -- each view sets its own commands via `SetViewCommands()`.

**Concern about command picker dismissal**: At `/root/projects/Autarch/pkg/tui/chatpanel.go` line 146, the picker hides when the query contains a space OR is 20+ characters. The space check means typing `/accept my reasoning` would dismiss the picker after `/accept `, which is correct. But the 20-character limit seems arbitrary and could cause the picker to vanish mid-typing for long command names.

**Concern about unknown commands**: When a user types an unrecognized slash command like `/fixbug`, the flow in `unified_app.go` line 422 silently returns nil. No feedback is given -- the command just disappears. This violates the principle of actionable error feedback.

### 4. Onboarding Flow (`internal/tui/onboarding.go`, `internal/tui/unified_app.go`)

The onboarding flow manages 9 states (defined at `/root/projects/Autarch/internal/tui/onboarding.go` lines 16-25) with transitions handled by ~400 lines of message handlers in `unified_app.go`. The project memory correctly identifies this as "entangled" and notes the plan to merge onboarding into the Gurgeh tab (Phase 2 of the unified navigation plan).

**State loss on tab switch**: At `/root/projects/Autarch/internal/tui/unified_app.go` line 1876, `switchToTab()` calls `enterDashboard()` when in onboarding mode. This immediately creates dashboard views and replaces the current view, losing all onboarding state (sprint progress, interview answers, generated epics). There is no confirmation dialog and no way to recover.

The breadcrumb component at `/root/projects/Autarch/internal/tui/breadcrumb.go` is well-designed -- it correctly tracks unlocked steps and provides keyboard navigation (Ctrl+B to enter, left/right to move, enter to jump, esc to cancel). However, the breadcrumb is only visible in onboarding mode, and the Ctrl+B toggle to enter navigation mode is not documented in the footer help text.

### 5. Dual TUI Implementations (`internal/tui/app.go` vs `internal/tui/unified_app.go`)

The `App` struct (used by `--skip-onboard`) and `UnifiedApp` struct (normal flow) implement the same logical interface but diverge in several UX-significant ways:

- **Ctrl+C behavior**: `App.Update()` at line 177 immediately quits on Ctrl+C. `UnifiedApp.Update()` at line 426 implements double-tap (two Ctrl+C within 500ms). Users who switch between `--skip-onboard` and normal mode will have inconsistent expectations.

- **Help overlay**: `App` uses the `HelpOverlay` from `pkg/tui/help.go` which renders a centered modal with `Background(ColorBgDark)`. `UnifiedApp` renders its own help at line 2018 with `Background(ColorBgLight)`. The visual styles differ.

- **Slash command handling**: `App` does not handle `SlashCommandMsg` at all -- only keybindings work. `UnifiedApp` has full slash command routing. This means `/big`, `/gur`, `/cold`, `/pol` work in normal mode but not with `--skip-onboard`.

- **Window size propagation**: `App` passes raw `WindowSizeMsg` to views (line 166). `UnifiedApp` subtracts header/footer/logpane height and creates a new message (line 347). Views receiving the message cannot tell which path they are under, potentially causing layout miscalculations.

### 6. Layout and Screen Real Estate

**Minimum width**: The `MinShellWidth` constant at `/root/projects/Autarch/pkg/tui/shelllayout.go` line 11 is set to 100 characters. A standard tmux split gives each pane ~80 characters on a 1920-wide display. This means users cannot use Autarch in a tmux split alongside another tool -- a common developer workflow. The error message at line 211 ("Terminal too narrow / Minimum width: 100 characters") is clear but does not suggest actionable remediation.

**Height accounting**: The `UnifiedApp.View()` method at line 1913 manually subtracts header (3 or 4), footer (3), and logpane (10) from total height. This is fragile -- the project memory entry about "lipgloss Height is a floor not a ceiling" at `/root/projects/Autarch/.claude/projects/-root-projects-Autarch/memory/MEMORY.md` warns about this exact pattern. The same subtraction happens independently in `Update()` at line 348, creating a maintenance risk if the two diverge.

**Stacked layout fallback**: The `SplitLayout` at `/root/projects/Autarch/pkg/tui/splitlayout.go` line 79 falls back to vertical stacking below 100 chars. This is good progressive disclosure, but the stacked mode gives only 40% to the doc panel (`LeftHeight` at line 63), which may be too little for the phase content during sprint review.

**Log pane height**: Fixed at 10 lines regardless of terminal size. On a 24-line terminal, this leaves only 8 lines for content after headers and footers are subtracted. The log pane should scale or at minimum hide at very small terminal heights.

### 7. Tab Bar (`internal/tui/tabs.go`)

The tab bar implementation is minimal and functional. Four tabs with clear Tokyo Night styling -- active tab gets `Background(ColorPrimary)` with bold text, inactive tabs use `ColorFgDim`. The tab names are joined without separators, which is clean.

**Missing features**: No notification indicators. When the Signals overlay is eventually implemented (Phase 3), there is no mechanism to show that new signals have arrived while on another tab. A badge or dot indicator would communicate pending information without requiring the user to check each tab.

The tab bar does not show its keybinding (Ctrl+Left/Right or /big /gur etc.) anywhere within itself. Users must discover these from the footer or help overlay.

### 8. Chat Panel and Composer (`pkg/tui/chatpanel.go`, `pkg/tui/composer.go`)

The chat panel at `/root/projects/Autarch/pkg/tui/chatpanel.go` is well-structured. Message roles (user/agent/system) get distinct styling with Tokyo Night colors. Word wrapping at line 437 handles basic cases correctly.

The composer at `/root/projects/Autarch/pkg/tui/composer.go` has a 2000 character limit (line 38), which is reasonable for chat-style input. The hint line ("enter: send  ctrl+j: newline") provides essential guidance. However, `ctrl+j` for newline is undocumented in the help overlay and SHORTCUTS.md.

**Mouse escape sequence filtering**: The `isMouseEscapeSequence` function at line 215 catches raw mouse escape sequences that BT v1 sometimes leaks through when mouse cell motion is enabled (`tea.WithMouseCellMotion()` at `unified_app.go` line 2171). This is a good defensive measure.

**Scroll behavior**: The chat panel scrolls from the bottom (most recent) upward. The `scroll` field at line 41 tracks offset from the bottom, which is the right default for a chat interface. However, there is no scroll position indicator -- users cannot tell how far back in history they have scrolled or how many messages remain above/below.

### 9. Document Panel (`pkg/tui/docpanel.go`)

The doc panel correctly handles scroll indicators at line 138 ("more above" / "more below"). The focus indicator (green dot prefix at line 86) provides a clear visual cue for which pane is active.

**Scroll one-line-at-a-time**: `ScrollUp()` and `ScrollDown()` at lines 172 and 179 move by exactly one line. The sprint view adds PgUp/PgDn handlers that scroll 3 lines at a time (sprint_view.go lines 312-322). This inconsistency between the doc panel's native scroll (1 line) and the sprint view's PgUp/PgDn (3 lines) means scrolling speed varies depending on which key the user presses, which is expected, but the doc panel itself offers no page-scroll method.

### 10. Sidebar (`pkg/tui/sidebar.go`)

The sidebar at `/root/projects/Autarch/pkg/tui/sidebar.go` uses `j/k` and `up/down` for navigation (line 97). This conflicts with the documented printable-key policy at `/root/projects/Autarch/docs/tui/SHORTCUTS.md` line 17: "Printable keys (letters, digits, symbols, space) are not bound anywhere in the unified three-pane UI." The sidebar's `j/k` bindings are printable keys.

The mitigation is that `j/k` only work when the sidebar is focused (guarded by line 89), and the sidebar only receives keys when `ShellLayout.focus == FocusSidebar` (line 152 of shelllayout.go). Since the chat composer captures all input in `FocusChat` mode, there is no practical collision. However, this is a documentation/policy inconsistency that could confuse contributors.

The sidebar width is fixed at 28 characters (`SidebarWidth` at line 12). Labels are truncated at 25 characters. This is tight for phase names like "Scope + Assumptions" or "Acceptance Criteria" which require up to 19 characters. It works but leaves little room for icons + padding.

### 11. Color Accessibility (`pkg/tui/colors.go`)

All colors are hard-coded hex values from the Tokyo Night palette at `/root/projects/Autarch/pkg/tui/colors.go`. There is no fallback path for terminals that only support 16 or 256 colors (common in SSH sessions to older servers, or in tmux without `set -g default-terminal "tmux-256color"`).

lipgloss does have some automatic degradation, but the specific hex values chosen (e.g., `#565f89` for muted text, `#3b4261` for borders) may collapse to the same ANSI color in 16-color mode, making UI elements indistinguishable. The project has not tested or documented degraded-color behavior.

### 12. Inline vs Fullscreen Mode

The `--inline` flag at `/root/projects/Autarch/internal/tui/unified_app.go` line 2168 controls whether the alt screen is used. When inline, log pane is visible by default and logs are dumped to scrollback on exit (line 2186). This is a well-considered UX decision -- inline mode preserves the terminal context that developers expect when debugging.

The alt screen mode (default) correctly enables mouse cell motion. Copy-paste from the TUI in alt screen mode is limited to terminal-native selection (no Autarch-level copy), which is standard but could frustrate users who want to copy spec content.

## Issues Found

### P0-1: Ctrl+Right collision between tab cycling and sprint accept

**Location:** `/root/projects/Autarch/internal/tui/unified_app.go` line 513 and `/root/projects/Autarch/internal/tui/views/sprint_view.go` line 303

**Problem:** `Ctrl+Right` is bound at two levels -- the parent `UnifiedApp` intercepts it for tab cycling before the sprint view can use it for "accept draft." The sprint view's `ShortHelp()` advertises "ctrl+right accept" but this binding is dead in practice because `UnifiedApp.Update()` handles the key at line 513 before forwarding to the current view.

**Suggestion:** Change the sprint accept binding to `/accept` (slash command, already implemented at line 502) and `Ctrl+A` (which is bound to "Accept" in CommonKeys at `/root/projects/Autarch/AGENTS.md` line 232 but not consumed by `UnifiedApp`). Remove the `Ctrl+Right` claim from sprint view's help text. Alternatively, suppress tab cycling when the sprint view is active and the user has unsaved progress.

### P0-2: Ctrl+S conflicts with XON/XOFF flow control

**Location:** `/root/projects/Autarch/internal/tui/views/kickoff.go` line 568

**Problem:** `Ctrl+S` triggers XOFF (suspend output) in terminals with flow control enabled, which is the default in many SSH and tmux configurations. The kickoff view binds `Ctrl+S` to "scan current directory." When a user presses Ctrl+S in a flow-control-enabled terminal, the terminal freezes and the scan never starts. The F4 fallback exists (line 568) but is not documented in the footer help text at line 993, the doc panel shortcuts section at line 246, or the help overlay at line 1002.

**Suggestion:** Lead with `/scan` (already implemented as a slash command) and F4 in all help text. Demote Ctrl+S to a secondary binding mentioned only in SHORTCUTS.md. Update the footer, doc panel, and help overlay to show "F4 or /scan" instead of "ctrl+s scan."

### P1-1: Tab switch during active sprint silently discards state

**Location:** `/root/projects/Autarch/internal/tui/unified_app.go` line 1876

**Problem:** When a user is mid-sprint (e.g., on phase 5 of 8 with 20 minutes invested), pressing Ctrl+Left/Right or typing `/big` switches to dashboard mode via `enterDashboard()`, which replaces all views and loses the sprint state. There is no confirmation dialog, no way to return to the sprint, and no autosave trigger before the switch. Sprint persistence (`.gurgeh/sprints/`) only saves on phase transitions, not on tab switches.

**Suggestion:** Before calling `enterDashboard()` from `switchToTab()`, check if the current view is a sprint view with unsaved progress. If so, either (a) trigger an autosave first and allow the switch, or (b) show a confirmation message in the chat panel: "Sprint in progress. Type /confirm to switch tabs, or press Esc to stay." Option (a) is simpler and preserves the user's expectation that tab switching is instant.

### P1-2: Silent swallow of unknown slash commands

**Location:** `/root/projects/Autarch/internal/tui/unified_app.go` line 422

**Problem:** When a user types an unrecognized slash command (e.g., `/fixbug`, `/export`, or a typo like `/gurge`), the handler at line 422 returns nil with no feedback. The user sees the input disappear and nothing happen. This is especially confusing because the command picker may have shown suggestions that the user navigated away from.

**Suggestion:** Add a system message to the chat panel: "Unknown command: /fixbug. Type / to see available commands." This can be implemented by having the fallback at line 422 check if `currentView` has a chat panel and posting a message, or by returning a `tea.Cmd` that produces a system message.

### P1-3: No visual feedback on first Ctrl+C

**Location:** `/root/projects/Autarch/internal/tui/unified_app.go` lines 426-441

**Problem:** The double-tap Ctrl+C behavior (first clears input, second quits within 500ms) provides no visual feedback on the first press. If the input is already empty, the user sees absolutely nothing happen and may assume the application is frozen. The pattern is borrowed from modern CLI tools (Claude Code itself uses it), but those tools display "Press Ctrl+C again to exit."

**Suggestion:** After the first Ctrl+C, display a transient footer message: "Press Ctrl+C again to quit." This can be implemented with a timer that clears the message after 1 second if no second Ctrl+C arrives.

### P1-4: MinShellWidth 100 chars too aggressive for terminal splits

**Location:** `/root/projects/Autarch/pkg/tui/shelllayout.go` line 11

**Problem:** `MinShellWidth = 100` means Autarch cannot run in a tmux pane when the terminal is split. A standard 1920x1080 display with a tmux 50/50 split gives each pane ~95 characters. Even on a 2560-wide display, a 3-way split produces ~85 chars per pane. The error message "Terminal too narrow" blocks all interaction with no degraded mode.

**Suggestion:** Lower `MinShellWidth` to 80 characters (the POSIX standard terminal width). The `SplitLayout` already has a stacked fallback mode that activates below `minWidth` (defaulting to 100). Adjust the stacked mode breakpoint to activate at 80-99 chars, and have the shell layout auto-collapse the sidebar below 80 chars. This gives three tiers: (a) below 80: error, (b) 80-99: stacked layout without sidebar, (c) 100+: full 3-pane layout.

### P1-5: App and UnifiedApp diverge on Ctrl+C quit behavior

**Location:** `/root/projects/Autarch/internal/tui/app.go` line 177 vs `/root/projects/Autarch/internal/tui/unified_app.go` line 426

**Problem:** `App.Update()` calls `tea.Quit` immediately on Ctrl+C. `UnifiedApp.Update()` requires double-tap. A user who normally uses `--skip-onboard` will develop muscle memory for single Ctrl+C, then accidentally quit the normal TUI on the second press of what they intended as "clear input."

**Suggestion:** Unify the quit behavior. The double-tap pattern is the safer default (prevents accidental data loss). Apply it to both implementations, or better yet, merge them as planned in Phase 2 of the navigation design.

### P1-6: Ctrl+U collision between doc scroll and revert

**Location:** `/root/projects/Autarch/internal/tui/views/sprint_view.go` line 271 and `/root/projects/Autarch/internal/tui/unified_app.go` line 508

**Problem:** The sprint view binds `Ctrl+U` for half-page-up scrolling in the document panel (Emacs/vim convention). The `UnifiedApp` binds `Ctrl+U` for reverting the last agent run. Since `UnifiedApp.Update()` processes keys at lines 485-509 before forwarding to the current view, the revert handler wins. The sprint view's doc panel scroll via Ctrl+U is dead.

**Suggestion:** Remove `Ctrl+U` from the `UnifiedApp` revert handler. Revert is an infrequent, destructive action that should require deliberate invocation via the command palette (`Ctrl+P` -> "Revert last run") or a slash command (`/revert`). Document panel scrolling, by contrast, is a continuous high-frequency action.

### P2-1: Slash command descriptions describe actions not tools

**Location:** `/root/projects/Autarch/pkg/tui/command_picker.go` lines 320-324

**Problem:** The tool-switching commands show "Switch to Bigend" / "Switch to Gurgeh" etc. without describing what each tool does. A new user encountering these commands for the first time has no context. The descriptions should match what appears in AGENTS.md line 7: "Multi-project agent mission control", "PRD generation and validation", etc.

**Suggestion:** Change descriptions to include the tool purpose:
- `/bigend` -> "Mission control -- multi-project overview"
- `/gurgeh` -> "PRD generation and validation"
- `/coldwine` -> "Task orchestration"
- `/pollard` -> "Research intelligence"

### P2-2: Footer help text overcrowded and truncates

**Location:** `/root/projects/Autarch/internal/tui/unified_app.go` line 2006

**Problem:** The footer concatenates view-specific help with global help: `help += "  |  /big /gur /cold /pol  ctrl+l logs  ctrl+p palette  ctrl+, settings  /help  ctrl+c x2 quit"`. At 80 characters width (a standard terminal), this wraps or gets truncated. The pipe separator, spaces, and long keybinding names consume space rapidly.

**Suggestion:** Show a contextual footer that adapts to width. At narrow widths, show only the 3 most relevant shortcuts plus "? help." At wide widths, show the full set. Alternatively, split the footer into two lines: view-specific on top, global on bottom.

### P2-3: Help overlay duplicate entries and inconsistent triggers

**Location:** `/root/projects/Autarch/internal/tui/unified_app.go` lines 2056-2065

**Problem:** The help overlay lists both `"?" -> "Show this help"` and `"/big /gur etc." -> "Switch tabs"` alongside `"/bigend, etc." -> "Switch to tool by name"`. The `?` key is listed but never bound anywhere -- it is an artifact. F1 is the actual help trigger (from `keys.go` line 37), but the help overlay says `?`. Additionally, `/big /gur etc.` and `/bigend, etc.` are redundant entries for the same feature.

**Suggestion:** Remove the `?` reference (it is not a real binding). Consolidate the two slash-command entries into one: `"/big, /gur, /cold, /pol" -> "Switch to tool tab"`. Ensure the help overlay documents F1 as the trigger for help, and mention `/help` as the slash command alternative.

### P2-4: g/G bindings in LogPane conflict with printable-key policy

**Location:** `/root/projects/Autarch/pkg/tui/logpane.go` lines 56-59

**Problem:** The log pane binds `g` (go to top) and `G` (go to bottom) -- these are vim-style bindings that use printable characters. While the log pane is only active when visible and focused, the keyboard routing does not enforce that the log pane is "focused" before passing keys to it. If a key leaks through while the user is typing, `g` or `G` could be consumed.

**Suggestion:** Replace `g`/`G` with `Home`/`End` which are already supported by the viewport component and align with the printable-key policy.

### P2-5: Sidebar j/k navigation conflicts with printable-key policy

**Location:** `/root/projects/Autarch/pkg/tui/sidebar.go` lines 97-98

**Problem:** The sidebar binds `j` (down) and `k` (up) in addition to arrow keys. These violate the stated printable-key policy in `SHORTCUTS.md`. The sidebar is guarded by focus state, but the policy document does not mention this exception, creating a documentation inconsistency.

**Suggestion:** Either remove the `j`/`k` bindings from the sidebar (arrow keys are sufficient) or document the exception in `SHORTCUTS.md` with the rationale that these only activate when the sidebar is explicitly focused via Tab.

### P2-6: No color fallback for 16-color terminals

**Location:** `/root/projects/Autarch/pkg/tui/colors.go`

**Problem:** All 15 color constants use hex values (`#7aa2f7`, `#1a1b26`, etc.) with no fallback for 16-color or 256-color terminals. In a 16-color terminal (common over SSH with basic TERM settings), lipgloss will attempt to find the closest ANSI color, which may make the muted text (`#565f89`) and border (`#3b4261`) indistinguishable from the background (`#1a1b26`), rendering the UI unusable.

**Suggestion:** Add a runtime check for terminal color capability (via `lipgloss.HasDarkBackground()` or checking `TERM`/`COLORTERM`). Provide an alternative 16-color palette that uses standard ANSI colors (bright blue for primary, magenta for secondary, green for success, etc.). This does not need to be pretty -- it just needs to be legible.

## Improvements Suggested

### IMP-1: Transient status bar for first Ctrl+C and unknown commands

Add a footer status message area that shows temporary feedback like "Press Ctrl+C again to quit" or "Unknown command: /fixbug". Auto-clear after 2 seconds. This addresses both P1-2 and P1-3.

### IMP-2: Signal notification badge in tab bar

Add an indicator (e.g., a dot or count) to the tab bar when the Signals overlay has unread notifications. This prepares the UI for Phase 3 (Signals overlay) and follows the pattern used by VS Code and similar tabbed interfaces where unsaved/new content is indicated inline.

### IMP-3: Lower MinShellWidth to 80 for tmux compatibility

Reduce `MinShellWidth` from 100 to 80. Use the stacked layout as an intermediate mode between 80-99 chars. Auto-collapse the sidebar below 90 chars. This makes Autarch usable in tmux splits and on smaller screens.

### IMP-4: Merge App and UnifiedApp into single codepath

This is already planned as Phase 2 of the navigation design. The UX benefits are significant: consistent Ctrl+C behavior, consistent slash command support, consistent help overlay styling, and a single WindowSizeMsg propagation path. The merge should preserve `App`'s simplicity (it does not need onboarding machinery) by making the dashboard the default starting point with onboarding moved into the Gurgeh tab.

### IMP-5: Scroll position indicators

Add a scroll position indicator to both the doc panel and chat history. A simple `[3/15]` or percentage in the scroll indicator line would help users understand where they are in long documents. The current "more above" / "more below" indicators in the doc panel are binary -- they do not convey distance.

### IMP-6: Contextual footer that adapts to focus and width

Replace the current footer (which concatenates everything) with a context-sensitive footer:
- When chat is focused: show chat-relevant shortcuts (enter send, /commands, ctrl+g model)
- When doc panel is focused: show scrolling shortcuts (pgup/pgdn, home/end)
- When sidebar is focused: show navigation shortcuts (up/down, enter select)
- Always: show the 2-3 most important global shortcuts (/help, ctrl+c quit)

This reduces visual clutter and ensures the most relevant shortcuts are always visible, even on narrow terminals.

## Overall Assessment

**Verdict: needs-changes**

The Autarch TUI has a strong architectural foundation: the chat-first paradigm, slash command fuzzy finder, Tokyo Night palette, and Cursor-style 3-pane layout create a distinctive and thoughtful interaction model. The documentation discipline (SHORTCUTS.md, printable-key policy, project memory) shows care for long-term maintainability.

However, several issues require attention before the TUI can be considered robust for daily use:

**Top 3 changes for better user experience:**

1. **Fix the Ctrl+Right collision (P0-1)**: This makes the primary interaction surface (sprint view) unable to use its documented keybinding for the most important action (accepting a phase). Users will discover this immediately and lose trust in the documented shortcuts.

2. **Add feedback for unknown commands and first Ctrl+C (P1-2, P1-3)**: Silent failures are the most frustrating UX pattern in terminal applications. A transient status message is cheap to implement and dramatically improves the error experience.

3. **Merge the dual implementations (IMP-4, P1-5)**: The App/UnifiedApp split creates subtle UX inconsistencies that surface as "this worked yesterday but not today" confusion. Users who alternate between `--skip-onboard` and normal mode will encounter different quit behavior, missing slash commands, and different help overlays.
