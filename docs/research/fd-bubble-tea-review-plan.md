# Bubble Tea v1 Specialist Review: Acceptance Criteria Plan

**Reviewer:** Claude Opus 4.6 (BT v1 specialist)
**Date:** 2026-02-06
**Plan reviewed:** `docs/plans/2026-02-05-acceptance-criteria-plan.md`
**Verdict:** 4 findings require plan amendments, 2 are informational

---

## Codebase Context

| Artifact | Version/Path |
|----------|-------------|
| Bubble Tea | v1.3.10 (`bubbletea v1.3.10`) |
| Lipgloss | v1.1.1-pre (`v1.1.1-0.20250404203927-76690c660834`) |
| Bubbles | v0.21.0 |
| SplitLayout | `pkg/tui/splitlayout.go` |
| ShellLayout | `pkg/tui/shelllayout.go` |
| ArbiterView | `internal/gurgeh/arbiter/tui/arbiter_view.go` |
| Tab bar | `internal/tui/tabs.go` |
| App | `internal/tui/app.go` |
| Keys | `pkg/tui/keys.go` |
| Command picker | `pkg/tui/command_picker.go` |
| Sidebar | `pkg/tui/sidebar.go` |
| LogPane | `pkg/tui/logpane.go` |
| PollardView | `internal/tui/views/pollard.go` |

---

## Finding 1: No BT v2 Feature Assumptions Detected

**Status:** PASS

The plan does not reference `KeyPressMsg`, `KeyReleaseMsg`, key-up events, or any Bubble Tea v2-specific API. All keybinding references in the plan use BT v1 patterns:

- `tea.KeyMsg` with `.String()` matching (used throughout `arbiter_view.go` lines 184-221)
- `key.Matches()` with `key.Binding` structs (used in `shelllayout.go` lines 136-148, `app.go` lines 177-213)
- `tea.KeyUp`, `tea.KeyDown`, `tea.KeyEnter` enum constants (used in `command_picker.go` lines 87-107)

The BT v1.3.10 `tea.KeyMsg` type supports `.Type` (enum) and `.String()` (human-readable). The codebase correctly uses both patterns and no AC assumes capabilities beyond this.

One minor note: the plan references `Ctrl+Left` and `Ctrl+Right` for tab cycling (line 286, "chat-first keybindings" institutional learning). This is already implemented in `app.go` line 208-211 using `msg.String() == "ctrl+left"`. In BT v1, these arrive as CSI sequences that BT parses into `"ctrl+left"`/`"ctrl+right"` strings. This works in most modern terminals (iTerm2, WezTerm, Kitty, Ghostty, Alacritty) but may not work in older terminals or tmux without `set -g extended-keys on`. The plan's AC-X.3 (terminal width >= 120) implicitly assumes a reasonably modern terminal, so this is acceptable.

---

## Finding 2: Badge Pulse Animation (AC-1.4) -- REQUIRES AMENDMENT

**Status:** CAUTION -- achievable but needs implementation guidance

**AC-1.4 states:** "Badge pulses after 5 minutes for unreviewed high-relevance findings (>0.8 score)"

### BT v1 Animation Capabilities

Bubble Tea v1 supports animations via `tea.Tick`:

```go
func (m Model) tick() tea.Cmd {
    return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}
```

This pattern is already used in the codebase:
- `pkg/shell/shell.go` lines 129-133: 2-second refresh tick
- `pkg/tui/loghandler.go` line 85: batch timer for log entries

### How Pulse Works in BT v1

A "pulse" can be implemented as a toggle between two visual states driven by `tea.Tick`:

1. Define a `badgePulseMsg` tick that fires every 500ms-1000ms
2. Toggle a `pulseOn bool` field on the tab bar or header component
3. Render the badge with alternating styles:
   - `pulseOn=true`: `Pollard (3)` with `Background(ColorWarning)` (bright)
   - `pulseOn=false`: `Pollard (3)` with `Background(ColorBgLight)` (dim)

This is standard BT v1 -- no v2 features needed. The `spinner` bubble uses exactly this pattern.

### Concern: Tick Lifecycle Management

The danger is orphaned ticks. When the user navigates away from the Pollard tab or reviews all findings, the pulse tick must stop. BT v1 has no "cancel command" mechanism -- the standard pattern is to return `nil` from the tick handler when the condition is no longer met:

```go
case badgePulseMsg:
    if !m.hasUnreviewedHighRelevance() || time.Since(m.firstUnreviewedAt) < 5*time.Minute {
        return m, nil // Don't re-tick
    }
    m.pulseOn = !m.pulseOn
    return m, tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
        return badgePulseMsg(t)
    })
```

### Recommendation

Add implementation note to AC-1.4: "Pulse implemented via `tea.Tick` at 500ms-1s interval toggling badge style between highlight and dim. Tick self-terminates when no unreviewed high-relevance findings remain. Must not leak ticks on tab switch."

The 5-minute delay before pulsing starts also needs a one-shot `tea.Tick(5*time.Minute, ...)` that checks if findings are still unreviewed before starting the pulse cycle. This is straightforward in BT v1.

**Severity:** LOW (achievable, needs implementation clarity to avoid tick leaks)

---

## Finding 3: Keybinding Ctrl+ Convention -- PASS with Notes

**Status:** PASS

The plan's institutional learning #4 (line 286) correctly states: "Chat-first keybindings: Ctrl+ prefixes only (no single-letter shortcuts). 50/50 split layout. Slash command picker on `/`."

### Current Keybinding Inventory

From the codebase, current Ctrl+ bindings:
| Key | Action | Source |
|-----|--------|--------|
| `ctrl+c` | Quit (double-press) | `keys.go:33`, `arbiter_view.go:186` |
| `ctrl+f` | Search | `keys.go:42` |
| `ctrl+r` | Refresh | `keys.go:74` |
| `ctrl+b` | Toggle sidebar | `shelllayout.go:139` |
| `ctrl+l` | Toggle log pane | `app.go:199` |
| `ctrl+p` | Command palette | `app.go:204` |
| `ctrl+a` | Accept draft | `arbiter_view.go:199` |
| `ctrl+e` | Edit draft | `arbiter_view.go:202` |
| `ctrl+left` | Previous tab | `app.go:208` |
| `ctrl+right` | Next tab | `app.go:210` |

The plan's ACs do not introduce any new keybindings. All interaction is through slash commands (`/accept`, `/vision`, `/1`, etc.) and existing Ctrl+ bindings. This is correct for BT v1 chat-first design.

### Note on Sidebar j/k Navigation

The Sidebar component (`sidebar.go` lines 96-101) uses single-letter `j`/`k` for navigation when focused. This is the only deviation from the Ctrl+ convention. It is acceptable because the sidebar only receives key events when explicitly focused (via Tab), and `j`/`k` are vim-standard navigation that don't conflict with the chat composer (which captures all keys when focused). The plan does not reference these keys.

---

## Finding 4: Lipgloss Height() Assumptions for 3-Pane Layout -- REQUIRES AMENDMENT

**Status:** WARNING -- known gotcha applies to multiple ACs

### The Known Lipgloss Height() Issue

From project MEMORY.md:
> **lipgloss `Height()` is a floor, not a ceiling**: If content + padding exceeds `Height(n)`, the block silently expands.

This directly affects:

1. **AC-1.5** (Pollard 3-pane layout): The Pollard view renders via `ShellLayout` which uses `SplitLayout(0.5)` for doc+chat. The sidebar uses `lipgloss.Height(s.height - 2)` at `sidebar.go:132`. If sidebar items exceed `height-2`, the sidebar silently grows and breaks horizontal alignment with the doc+chat split.

2. **AC-X.3** (3-pane layout at >= 120 columns): The `ShellLayout.Render()` at `shelllayout.go:192` uses `lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, sep, splitView)`. If any component overflows its declared height, `JoinHorizontal` produces misaligned output.

3. **LogPane** (`logpane.go:105-106`): Uses `Height(p.height)` on the outer style. The viewport inside is sized to `height-1`, but the header line + padding can cause the outer style to silently exceed the declared height.

### Current Mitigation

The `SplitLayout.ensureSize()` function at `splitlayout.go:152-169` manually truncates/pads content to exact dimensions BEFORE lipgloss rendering. This is the correct approach -- it bypasses lipgloss Height() by controlling line count directly.

However, the sidebar at `sidebar.go:128-132` uses `borderStyle.Height(s.height - 2)` which wraps content AFTER lipgloss border rendering. If content exceeds the height, the border box expands silently.

### When This Bites

With the planned Pollard enhancements (AC-1.5), the sidebar will show Inbox/Accepted/Rejected/Deferred filter groups plus hunter status icons. If a user has 20+ findings, the sidebar item count could easily exceed `height-2`. The border box expands, `JoinHorizontal` misaligns, and the right side of the screen wraps or disappears.

### Recommendation

Add to AC-X.3 verification: "Verify sidebar content is truncated to available height before lipgloss border rendering. Use `ensureSize` or equivalent line-count clamping, not `lipgloss.Height()`, as the primary height constraint."

Also from MEMORY.md:
> **Always test lipgloss layout math empirically**: Write a quick `go run` script that counts `strings.Count(rendered, "\n")+1` for each section and compares total to terminal height.

Add this as a test strategy note for AC-X.3.

**Severity:** MEDIUM (layout corruption for users with many findings)

---

## Finding 5: 50/50 Split Layout -- PASS

**Status:** PASS -- already implemented correctly

### Current Implementation

The `ShellLayout` creates its `SplitLayout` with ratio `0.5`:

```go
// shelllayout.go:39
splitLayout: NewSplitLayout(0.5),
```

This produces equal-width doc and chat panes. The `SplitLayout.LeftWidth()` calculates:
```go
// splitlayout.go:48
return int(float64(l.width) * l.leftRatio) - 2 // Account for separator
```

For 120 columns with sidebar (28 chars + 2 separator = 30), content area = 90 chars. Left pane = `int(90 * 0.5) - 2 = 43`. Right pane = `90 - 45 - 1 = 44`. The 1-char difference comes from integer truncation but is visually imperceptible.

### Plan References

AC-1.5 references the "50/50 split layout" from institutional learning #4. The `DefaultLayoutConfig` in `layout.go:51` also defaults to `LeftRatio: 0.5`.

### Stacked Fallback

For narrow terminals, `SplitLayout.IsStacked()` triggers at `width < 100` (configurable via `SetMinWidth`). The plan's AC-X.4 requires graceful degradation below 120 columns. The current `MinShellWidth = 100` in `shelllayout.go:11` means:
- 120+ columns: full 3-pane layout (sidebar + 50/50 split)
- 100-119 columns: shell layout works but split is tight
- <100 columns: error message displayed

This aligns with AC-X.3 (>= 120) and AC-X.4 (< 120 degradation). No issues.

---

## Finding 6: Slash Command Alias Conflicts -- REQUIRES AMENDMENT

**Status:** WARNING -- potential collision with planned Pollard triage commands

### Current Alias Registry

From `command_picker.go` `GlobalCommands()` (lines 311-327):
| Command | Aliases |
|---------|---------|
| help | h |
| quit | q, exit |
| settings | config |
| model | m |
| palette | p |
| refresh | r |
| back | b |
| bigend | big |
| gurgeh | gur |
| coldwine | cold |
| pollard | pol |
| signals | sig |
| logs | log, l |

From `SprintCommands()` (lines 340-355):
| Command | Aliases |
|---------|---------|
| accept | a |
| 1, 2, 3 | (none) |
| vision | vis |
| problem | prob |
| users | usr |
| features | feat |
| cujs | cuj |
| reqs | req |
| scope | scp |
| acceptance | ac |

From `KickoffCommands()` (lines 330-336):
| Command | Aliases |
|---------|---------|
| scan | s |
| new | n |
| delete | d |

### Plan's Implied New Commands

AC-1.6 says the agent pane accepts natural language triage ("reject--not our market"). But AC-1.7 mentions "Accept action opens edit preview." The plan implies Pollard will need triage slash commands similar to sprint commands. Likely candidates:

- `/accept` or `/a` -- **COLLISION** with SprintCommands `accept`/`a`
- `/reject` or `/rej`
- `/defer` or `/def`
- `/dive` or `/deep` (for Deep Dive in AC-3.5)

The collision on `/accept` is context-dependent: Sprint commands are active in the Gurgeh tab, while Pollard triage commands would be active in the Pollard tab. The `UnifiedApp` (`internal/tui/unified_app.go` lines 363-421) dispatches slash commands first to global handlers, then to view-specific handlers. Since `/accept` is registered in `SprintCommands()` (not `GlobalCommands()`), it only applies when the sprint view is active. A Pollard-specific `/accept` would need its own `PollardCommands()` function.

### Recommendation

1. Add a Pollard-specific command set (`PollardCommands()`) with triage commands: `/accept` (`/a`), `/reject` (`/rej`), `/defer` (`/def`), `/dive` (`/deep`).
2. Verify no alias collisions with GlobalCommands. Current potential collision: `/d` is used by both KickoffCommands (delete) and potential Pollard (defer). Since these are view-scoped, not global, this is acceptable but should be documented.
3. Add to plan: "Slash command aliases for Pollard triage must be checked against `GlobalCommands()` in `pkg/tui/command_picker.go` before implementation."

**Severity:** LOW (view-scoped commands avoid true collisions, but documentation needed)

---

## Finding 7: LogPane Height in 3-Pane + Log Layout -- INFORMATIONAL

**Status:** INFORMATIONAL

### Current Log Pane Sizing

The `App.View()` at `app.go:275-279` calculates:
```go
logPaneHeight := 0
if a.logPaneVisible {
    logPaneHeight = 10
}
contentHeight := a.height - 4 - logPaneHeight
```

The log pane is a fixed 10 rows. The `4` accounts for tab bar (1-2 lines) + footer (1-2 lines).

### Impact on Pollard 3-Pane

When log pane is visible, `contentHeight` is reduced by 10. This propagates to the shell layout via `WindowSizeMsg` (the app re-sends a size message after toggling). The Pollard view at `pollard.go:71` subtracts 4 more: `v.height = msg.Height - 4`. In a 40-row terminal with log pane visible:
- `contentHeight = 40 - 4 - 10 = 26`
- Pollard's usable height = `26 - 4 = 22`
- Sidebar height = `22 - 2 = 20` (border)
- SplitLayout height = `22`

This is tight but functional. The plan's AC-1.4 badge pulse, AC-1.5 3-pane layout, and AC-1.16 log pane hunter activity all need to work simultaneously. At 22 rows, the 50/50 split gives each pane 22 rows -- enough for meaningful content.

### Potential Issue

The `LogPane.View()` uses `Height(p.height)` which is the lipgloss floor-not-ceiling issue. At exactly 10 rows, if header + viewport + padding = 11, the log pane steals 1 extra row from content. This is the same class of bug as Finding 4.

**No plan amendment needed**, but implementation should use line-count clamping for the log pane, not lipgloss `Height()`.

---

## Finding 8: 3-Pane Pollard Layout is a New ShellLayout, Not a Modified SplitLayout -- INFORMATIONAL

**Status:** INFORMATIONAL

### Current Architecture

The plan's Pollard 3-pane (AC-1.5) describes: sidebar (Inbox/Accepted/Rejected/Deferred + hunter status), doc pane (finding detail), agent pane (triage conversation).

This is exactly the existing `ShellLayout` pattern: sidebar + `SplitLayout(0.5)` for doc + chat. The `PollardView` already uses this at `pollard.go:36`:
```go
shell: pkgtui.NewShellLayout(),
```

And renders via:
```go
return v.shell.Render(sidebarItems, document, chat)
```

The plan's Pollard 3-pane is NOT a new layout -- it reuses the existing `ShellLayout` with different sidebar items and different doc/chat content. The sidebar items would change from insight titles to filter groups (Inbox/Accepted/Rejected/Deferred), and the doc pane would show finding detail instead of insight detail.

This is a data change, not a layout change. No new layout code needed.

---

## Summary of Required Amendments

| # | Finding | Severity | AC Affected | Action |
|---|---------|----------|-------------|--------|
| 2 | Badge pulse needs `tea.Tick` implementation guidance | LOW | AC-1.4 | Add implementation note about tick lifecycle management |
| 4 | Lipgloss Height() floor-not-ceiling affects sidebar + log pane | MEDIUM | AC-1.5, AC-X.3 | Add verification requirement for line-count clamping |
| 6 | Pollard triage slash commands need alias collision check | LOW | AC-1.6, AC-1.7 | Add requirement to check against `GlobalCommands()` |
| 7 | LogPane Height() same class of bug as sidebar | INFO | (implicit) | Implementation note only |

### Items That Pass Without Issues

- No BT v2 feature assumptions (Finding 1)
- All keybindings use Ctrl+ convention (Finding 3)
- 50/50 split layout already implemented correctly (Finding 5)
- 3-pane Pollard layout reuses existing ShellLayout (Finding 8)
