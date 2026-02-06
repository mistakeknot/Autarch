# Unified TUI Navigation Design

**Date:** 2026-02-05
**Status:** Approved (revised after review)
**Author:** Human + Claude

## Flux Drive Enhancement Summary

Reviewed by 7 agents (6 codebase-aware, 1 generic) on 2026-02-06.

### Key Findings
- **P0: `Ctrl+Shift+S` is dead in BT v1** — BT v1 cannot distinguish `Ctrl+Shift+S` from `Ctrl+S` (XOFF). Drop the keybinding; use `/sig` only. (fd-bubble-tea, fd-user-experience, fd-architecture)
- **P0: Agent ownership transfer unspecified** — `codingAgent`, `agentSelector`, and `selectedAgent` must move to Gurgeh or be exposed via interface. Without these, epic/task generation breaks. (fd-gurgeh-specs)
- **P1: Phase 2 message routing contract undefined** — 12+ message types need explicit classification: which stay at shell vs. become Gurgeh-internal. Design spike required. (fd-architecture, fd-bubble-tea, fd-code-quality, fd-gurgeh-specs)
- **P1: Phase 2 should be split into sub-phases** — Dead code cleanup (2a), App/UnifiedApp merge (2b), onboarding relocation (2c). Bundling creates unreviewable PRs. (code-simplicity, fd-code-quality)
- **P1: Tab switch during sprint silently exits onboarding** — `switchToTab()` calls `enterDashboard()` which discards sprint state. Needs confirmation or state preservation. (fd-user-experience)

### Issues to Address
- [ ] Drop `Ctrl+Shift+S` from plan, use `/sig` only (P0 — fd-bubble-tea, fd-user-experience)
- [ ] Add `codingAgent`/`agentSelector`/`selectedAgent` to Phase 2 field migration list (P0 — fd-gurgeh-specs)
- [ ] Define message routing contract before Phase 2 implementation (P1 — fd-architecture, fd-bubble-tea)
- [ ] Split Phase 2 into sub-phases: 2a (dead code), 2b (App merge), 2c (onboarding relocation) (P1 — code-simplicity)
- [ ] Delete `OnboardingOrchestrator` — unused dead code (P1 — code-simplicity)
- [ ] Fix `sendToCurrentView` dropping returned `tea.Cmd` (P1 — code-simplicity, fd-bubble-tea)
- [ ] Address tab-switch-during-sprint UX (P1 — fd-user-experience)
- [ ] Specify Signals overlay content model and height cap (P1 — fd-performance, fd-user-experience)
- [ ] Preserve lazy view initialization in merged App (P2 — fd-performance)
- [ ] Fix `Ctrl+Right` collision between tab cycling and sprint accept (P2 — fd-bubble-tea)
- [ ] Add signal notification indicator to tab bar (P2 — fd-user-experience)
- [ ] Verify WindowSizeMsg height subtraction isn't doubled after App merge (P2 — fd-bubble-tea)
- [ ] Ensure goroutines in moved code capture GurgehView pointer, not copy (P2 — fd-bubble-tea)
- [ ] Improve slash command descriptions to describe tools, not actions (P2 — fd-user-experience)

## Overview

Simplify Autarch TUI navigation by always showing tool tabs and removing the separate "onboarding" mode. Add direct keybindings and slash commands for tool switching.

## Goals

1. Always-visible tabs for the 4 core tools (Bigend, Gurgeh, Coldwine, Pollard)
2. Slash commands (`/big`, `/gur`, `/cold`, `/pol`) for instant tab switching
3. Slash commands (`/bigend`, `/gurgeh`, etc.) for discoverability
4. Move Signals from a tab to an overlay (accessible via `/signals`)

> **Flux Drive** (fd-bubble-tea, fd-user-experience): Drop `Ctrl+Shift+S` — BT v1 cannot distinguish it from `Ctrl+S` (XOFF). `/sig` slash command is the portable, consistent access method.
5. Eliminate `ModeOnboarding`/`ModeDashboard` distinction — Gurgeh manages its own internal flow

## Design

### Tab Bar Always Visible

**Tabs:** 4 core tools
- Bigend — Mission control
- Gurgeh — PRD generation (includes kickoff/spec flow internally)
- Coldwine — Task orchestration
- Pollard — Research intelligence

**Signals:** Moved to overlay (not a tab)

**Visual:**
```
┌─────────────────────────────────────────────────────┐
│  Bigend  │ [Gurgeh] │  Coldwine  │  Pollard        │  ← tabs (Gurgeh active)
├─────────────────────────────────────────────────────┤
│ Project › Interview › Spec › Epics › Tasks         │  ← breadcrumb inside Gurgeh
│                                                     │
│ ┌─────────────────┐ ┌─────────────────────────────┐ │
│ │ Sidebar         │ │ What do you want to build?  │ │
│ │ ○ Vision        │ │                             │ │
│ │ ○ Problem       │ │ [Chat composer...]          │ │
│ └─────────────────┘ └─────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

### Keybindings

**New:**

| Key | Action |
|-----|--------|
| `/bigend` (`/big`) | Switch to Bigend |
| `/gurgeh` (`/gur`) | Switch to Gurgeh |
| `/coldwine` (`/cold`) | Switch to Coldwine |
| `/pollard` (`/pol`) | Switch to Pollard |
| `/signals` (`/sig`) | Toggle Signals overlay |
| `Ctrl+Left/Right` | Cycle tabs |

> **Note:** Direct keybindings (Ctrl+N, Alt+N) were explored but dropped due to Bubble Tea v1
> limitations — Ctrl+number has no standard terminal encoding, and Alt+number requires
> per-terminal configuration (macOS Option-as-Alt, tmux escape-time). Slash commands are
> portable and discoverable.

**Existing (unchanged):**

| Key | Action |
|-----|--------|
| `Ctrl+Left` / `Ctrl+Right` | Cycle tabs |
| `Ctrl+PgUp` / `Ctrl+PgDn` | Cycle tabs (fallback) |

> **Flux Drive** (fd-bubble-tea): `Ctrl+Right` collides with SprintView's "accept draft" binding (`sprint_view.go:303`). UnifiedApp intercepts it first for tab cycling, so the sprint accept is dead in the unified TUI. Fix during Phase 2 by removing it from SprintView or using a different key for accept.
| `Ctrl+P` | Command palette |
| `/` | Slash command picker |

### Slash Commands

Add to `GlobalCommands()`:

| Command | Aliases | Description |
|---------|---------|-------------|
| `/bigend` | `/big` | Switch to Bigend |
| `/gurgeh` | `/gur` | Switch to Gurgeh |
| `/coldwine` | `/cold` | Switch to Coldwine |
| `/pollard` | `/pol` | Switch to Pollard |
| `/signals` | `/sig` | Toggle Signals overlay |

> **Why 3-letter aliases instead of single-letter?** Single-letter aliases collide with
> existing global commands: `/b` = `/back`, `/p` = `/palette`, `/g` = `/group` (task review).
> Three-letter prefixes are unique and still quick to type.

### Simplified Mode Architecture

**Remove:**
- `ModeOnboarding` / `ModeDashboard` distinction
- App-level `onboardingState` and `breadcrumb`

**New behavior:**
- Always in "dashboard" mode with tabs visible
- Gurgeh tab manages its own internal state (kickoff → interview → spec → epics → tasks)
- Breadcrumb lives inside Gurgeh's content area

**Startup:**
```
autarch tui                    → Gurgeh tab active (kickoff or last state)
autarch tui --tool=bigend      → Bigend tab active
autarch tui --tool=coldwine    → Coldwine tab active
autarch tui --skip-onboard     → Deprecated (no-op with warning)
```

### Signals Overlay

Convert SignalsView from a full tab to a toggleable overlay:

```
When /sig entered (or Esc to dismiss):
┌─────────────────────────────────────────────────────┐
│  Bigend  │ [Gurgeh] │  Coldwine  │  Pollard      * │  ← * = unread signals
├─────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────┐ │
│ │ Signals (3 new)                     [Esc close] │ │
│ │ ⚠ Competitor shipped: Acme v2.0          5m ago │ │
│ │ • Task completed: COLD-042              12m ago │ │
│ │ ! Assumption decayed: "users prefer X"   1h ago │ │
│ └─────────────────────────────────────────────────┘ │
│                                                     │
│ [Underlying view content continues below...]        │
└─────────────────────────────────────────────────────┘
```

---

## Review Findings

Thorough review of the codebase uncovered several issues with the original flat implementation plan.

### Finding 1: Keybinding Conflict (Resolved)

`internal/gurgeh/arbiter/tui/arbiter_view.go:201-203` previously used `Ctrl+1`, `Ctrl+2`, `Ctrl+3`
for selecting alternative proposals during sprint phases.

**Resolution:** Removed `Ctrl+1-3` from ArbiterView. These were redundant with `/1`, `/2`, `/3`
slash commands and `up/down` + `enter` navigation.

### Finding 2: Slash Command Alias Collisions

Original single-letter aliases collide with existing commands:
- `/b` = `/back` (global)
- `/p` = `/palette` (global)
- `/g` = `/group` (task review)

**Resolution:** Use 3-letter prefixes: `/big`, `/gur`, `/cold`, `/pol`, `/sig`.

### Finding 3: Onboarding is Deeply Entangled in UnifiedApp

The plan to "remove ModeOnboarding and have Gurgeh manage its own flow" is far larger than estimated:

- `unified_app.go` has **~400 lines** of onboarding transition handlers (`handleProjectCreated`,
  `handleInterviewComplete`, `handleSpecAccepted`, `handleEpicsGenerated`, `handleTasksAccepted`,
  `scanCodebase`, etc.)
- The `UnifiedApp` struct holds onboarding-specific fields: `projectID`, `projectName`,
  `interviewAnswers`, `generatedEpics`, `generatedTasks`, `onboardingState`, `breadcrumb`
- **12+ message types** are routed through `UnifiedApp.Update()` for onboarding transitions
- `cmd/autarch/main.go` injects **6 view factories** for the onboarding flow

Moving this into the Gurgeh view is an **800-1200 line refactor**, not 400-600.

### Finding 4: Two Separate Code Paths

`--skip-onboard` uses `tui.RunWithOpts()` (the `App` struct) while normal mode uses
`tui.RunUnifiedWithOpts()` (the `UnifiedApp` struct). These are completely separate
implementations with different tab management, key handling, and view routing.

### Finding 5: Signals Overlay Scope

Converting `SignalsView` from a full 3-pane `View` (using `ShellLayout` with sidebar, document,
chat) to an overlay is non-trivial UI work that can be deferred.

---

## Phased Implementation

Given the review findings, implementation is split into three phases to reduce risk.

### Phase 1: Tabs Always Visible + Navigation (Small, low-risk)

**Goal:** Users can always see and switch between tool tabs, even during onboarding.

**Changes:**
- Show tab bar in both `ModeOnboarding` and `ModeDashboard` (change View() rendering only)
- Add slash command handlers for tab switching (`/bigend`, `/gurgeh`, `/coldwine`, `/pollard`)
- Add `/bigend`, `/gurgeh`, `/coldwine`, `/pollard`, `/signals` slash commands
- Tab switching during onboarding exits onboarding and enters dashboard for the selected tool

> **Flux Drive** (fd-user-experience): This silently discards mid-sprint state. Add a confirmation prompt ("Sprint in progress. Switch tab?") or preserve onboarding state so returning to Gurgeh resumes the sprint.
- Palette (`Ctrl+P`) works in onboarding mode too (currently gated to `ModeDashboard`)
- Remove Signals from the dashboard tab list (4 tabs instead of 5)

**Files:**

| File | Changes |
|------|---------|
| `internal/tui/unified_app.go` | Always render tabs in `View()`; add slash command tab handlers; allow palette in onboarding; handle tab-switch-during-onboarding; remove Signals from tab list |
| `pkg/tui/command_picker.go` | Add tool-switching slash commands to `GlobalCommands()` |
| `cmd/autarch/main.go` | Remove SignalsView from dashboard views factory; update `--tool` flag docs |

**Estimate:** ~100-150 lines changed, 3 files.

### Phase 2: Gurgeh Absorbs Onboarding (Large refactor)

**Goal:** Eliminate `ModeOnboarding`/`ModeDashboard` distinction. Gurgeh becomes a self-contained
view that manages kickoff → sprint → spec → epics → tasks internally.

> **Flux Drive** (code-simplicity, fd-code-quality): Split Phase 2 into three independently-shippable sub-phases to reduce risk and enable incremental review.

**Phase 2a: Delete dead code (~150 LOC removed, zero behavior change)**
- Delete `OnboardingOrchestrator` (`onboarding.go:111-247`) — unused, never wired into UnifiedApp
- Delete `renderFooter` deprecated wrapper (`unified_app.go:2012-2015`)
- Delete `onboardingHeader` unused method (`unified_app.go:1980-1995`)
- Replace hand-rolled string helpers with `strings.*` (`types.go:136-185`)
- Fix `sendToCurrentView` to return `tea.Cmd` instead of discarding it (`unified_app.go:1280-1287`)

**Phase 2b: Merge App and UnifiedApp (~300-400 lines changed)**
- Keep `UnifiedApp` as the survivor (owns log pane, chat settings, agent lifecycle, palette)
- When `--skip-onboard` is used, skip to `enterDashboard()` immediately in `Init()`
- Delete `app.go` entirely and the `RunWithOpts` function
- Preserve lazy view initialization (do NOT eager-init all views at startup — Bigend/Coldwine/Pollard may open SQLite or Intermute connections)
- `--skip-onboard` prints stderr deprecation warning with migration instructions

> **Flux Drive** (fd-performance): Eager view init would add 100-500ms to startup. Preserve UnifiedApp's lazy pattern.
> **Flux Drive** (fd-user-experience): Deprecation warning must be actionable: "Use --tool=gurgeh or omit the flag."

**Phase 2c: Move onboarding into Gurgeh (~400-600 lines changed)**

**Changes:**
- Create `GurgehOnboardingView` as a separate type (do NOT cram into the 313-line GurgehView)
- `GurgehView` becomes a container that switches between `GurgehOnboardingView` and spec browser
- Move all onboarding message handlers out of `UnifiedApp.Update()`
- Move breadcrumb into Gurgeh's content area
- Move `codingAgent`, `agentSelector`, `selectedAgent` to Gurgeh (or expose via `AgentProvider` interface)
- Rewire view factories in `cmd/autarch/main.go` via a `GurgehConfig` struct

> **Flux Drive** (fd-gurgeh-specs): P0 — `codingAgent` and `agentSelector` must be explicitly transferred. Without them, epic/task generation fails with `AgentNotFoundMsg`.
> **Flux Drive** (fd-architecture): Define message routing contract before coding. Messages like `ProjectCreatedMsg`, `SprintCompleteMsg` become Gurgeh-internal. Only `GurgehOnboardingDoneMsg` escapes to the shell.
> **Flux Drive** (fd-bubble-tea): GurgehView.Update() MUST have default pass-through forwarding unhandled messages to active sub-view. Otherwise `SprintConflictMsg` from consistency engine gets swallowed.
> **Flux Drive** (fd-gurgeh-specs): `SprintStateProvider` type assertion will fail after Phase 2c — GurgehView must implement it by delegating to internal SprintView, or handle `SprintCompleteMsg` entirely internally.
> **Flux Drive** (fd-gurgeh-specs): Capture `projectPath` at kickoff time and store as a field. Do not re-read CWD when resuming.
> **Flux Drive** (fd-bubble-tea): WindowSizeMsg double-subtraction risk — UnifiedApp subtracts header/footer before forwarding to views, but views also subtract their own margins. After merge, verify content area height is correct.
> **Flux Drive** (fd-bubble-tea): Goroutine ownership — `scanCodebase` and `generateEpicsWithAgent` capture the `UnifiedApp` pointer. When moved to GurgehView, goroutines must capture the GurgehView pointer. Ensure `GurgehView.Update()` returns the same pointer (not a copy).

**Design spike required before Phase 2c:**
1. Message routing table (shell-level vs. Gurgeh-internal) — fd-code-quality provides a concrete three-way partition of all 25+ message types
2. `GurgehConfig` struct (factory injection API) — replaces 6-positional-argument `SetViewFactories()`, matching `SprintViewOpts` pattern
3. Merge strategy for `OnboardingState` and `Breadcrumb` — **cannot move to `views/` package** due to import cycle (`views/` → `tui/` → `views/`). Keep types in `internal/tui/`, migrate only usage.
4. Agent dependency injection approach (`AgentProvider` interface vs. direct ownership)
5. `scanCodebase()` + 280 lines of conversion helpers need explicit home (`gurgeh_scan.go` or push down to `internal/gurgeh/exploration/`)
6. Define 4 intermediate commits (extract → reroute → merge → cleanup), each independently testable

**Files:**

| File | Changes |
|------|---------|
| `internal/tui/unified_app.go` | Remove onboarding state, breadcrumb, ~400 lines of transition handlers |
| `internal/tui/views/gurgeh.go` | Become container; switch between onboarding and spec browser |
| `internal/tui/views/gurgeh_onboarding.go` | New: absorb onboarding orchestration, breadcrumb, view factories |
| `internal/tui/onboarding.go` | Delete `OnboardingOrchestrator`; keep `OnboardingState` (moves to gurgeh) |
| `internal/tui/breadcrumb.go` | Move into gurgeh view |
| `cmd/autarch/main.go` | Simplify; wire `GurgehConfig` instead of `SetViewFactories()` |
| Tests | Update for new structure; add test for `SprintConflictMsg` reaching SprintView |

**Estimate:** ~800-1200 lines changed across 2a+2b+2c, 5-8 files.

### Phase 3: Signals Overlay (Deferred)

**Goal:** Convert Signals from a removed tab to an overlay accessible via `/signals` (`/sig`).

> **Flux Drive** (fd-bubble-tea, fd-user-experience): `Ctrl+Shift+S` dropped — unreliable in BT v1. `/sig` is the sole access method, consistent with tab navigation model.

**Changes:**
- Create a new `SignalsOverlay` component (simplified rendering, no 3-pane layout)
- Add toggle state and rendering in `UnifiedApp`
- Wire `/signals` slash command to toggle

> **Flux Drive** (fd-performance): Cap overlay at 10 lines + 2 border = 12 lines max. Cache signal data, refresh on timer (5s) or Intermute event — NOT on every toggle. Pre-render from cache makes toggle instant (zero SQLite I/O).
> **Flux Drive** (fd-user-experience): Define overlay as read-only notification list. No filtering, no interaction beyond scroll + dismiss (Esc). For full interaction, user opens full SignalsView via Enter or `/signals --full`.
> **Flux Drive** (fd-performance): Signals overlay WebSocket should stay connected while hidden (persist component, toggle visibility). Creating new connections per toggle is wasteful.
> **Flux Drive** (fd-user-experience): Add a signal notification indicator (`*` or count badge) to the tab bar header. Without it, users have no passive awareness of new signals.

**Estimate:** ~200-300 lines, 2-3 files.

---

## Deferred Work (Beyond Phases 1-3)

- **Bigend Agent Intelligence** — Agent reviews signals and surfaces suggestions, alerts, and
  clarifying questions to the user. Separate design needed.

## Decision Log

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Tab count | 4 (not 5) | Signals is infrastructure, not a workspace |
| Signals access | Overlay (Phase 3) | Keeps tabs focused on tools users work *in* |
| Signals keybinding | `/sig` only (no Ctrl+Shift+S) | BT v1 can't distinguish Ctrl+Shift+S from Ctrl+S (XOFF). Consistent with slash-only navigation. (Flux Drive 2026-02-06) |
| Slash commands | `/big`, `/gur`, `/cold`, `/pol` | Portable across all terminals; discoverable via command picker; no modifier key issues |
| Slash aliases | 3-letter (`/big`, `/gur`) | Single-letter aliases collide with `/back`, `/palette`, `/group` |
| Onboarding mode | Removed in Phase 2 | Gurgeh manages its own flow; simpler mental model |
| Implementation | 3 phases (Phase 2 split into 2a/2b/2c) | Phase 1 is low-risk and delivers most UX value immediately. Phase 2 split reduces review/revert risk. (Flux Drive 2026-02-06) |
| App merge direction | Delete `App`, simplify `UnifiedApp` | UnifiedApp has log pane, palette, chat settings that App lacks. NOT a two-way merge. (Flux Drive 2026-02-06) |
| GurgehView structure | Container with `GurgehOnboardingView` sub-view | Don't cram 800 lines into the 313-line spec browser. (Flux Drive 2026-02-06) |
