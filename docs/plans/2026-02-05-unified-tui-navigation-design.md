# Unified TUI Navigation Design

**Date:** 2026-02-05
**Status:** Approved (revised after review)
**Author:** Human + Claude

## Overview

Simplify Autarch TUI navigation by always showing tool tabs and removing the separate "onboarding" mode. Add direct keybindings and slash commands for tool switching.

## Goals

1. Always-visible tabs for the 4 core tools (Bigend, Gurgeh, Coldwine, Pollard)
2. Direct keybindings (`Ctrl+1-4`) for instant tab switching
3. Slash commands (`/bigend`, `/gurgeh`, etc.) for discoverability
4. Move Signals from a tab to an overlay (accessible via `Ctrl+Shift+S` or `/signals`)
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
| `Ctrl+1` | Switch to Bigend |
| `Ctrl+2` | Switch to Gurgeh |
| `Ctrl+3` | Switch to Coldwine |
| `Ctrl+4` | Switch to Pollard |
| `Ctrl+Shift+S` | Toggle Signals overlay |

> **Conflict resolved:** `Ctrl+1/2/3` was previously used by the Arbiter view for selecting
> alternative proposals, but this was removed (redundant with `/1`, `/2`, `/3` slash commands
> and `up/down` + `enter` navigation). `Ctrl+N` is the familiar browser/IDE pattern.

**Existing (unchanged):**

| Key | Action |
|-----|--------|
| `Ctrl+Left` / `Ctrl+Right` | Cycle tabs |
| `Ctrl+PgUp` / `Ctrl+PgDn` | Cycle tabs (fallback) |
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
When Ctrl+Shift+S pressed:
┌─────────────────────────────────────────────────────┐
│  Bigend  │ [Gurgeh] │  Coldwine  │  Pollard        │
├─────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────┐ │
│ │ Signals                    [Ctrl+Shift+S close] │ │
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
slash commands and `up/down` + `enter` navigation. This frees `Ctrl+1-4` for tab switching.

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
- Add `Ctrl+1-4` keybindings for direct tab switching
- Add `/bigend`, `/gurgeh`, `/coldwine`, `/pollard`, `/signals` slash commands
- Tab switching during onboarding exits onboarding and enters dashboard for the selected tool
- Palette (`Ctrl+P`) works in onboarding mode too (currently gated to `ModeDashboard`)
- Remove Signals from the dashboard tab list (4 tabs instead of 5)

**Files:**

| File | Changes |
|------|---------|
| `internal/tui/unified_app.go` | Always render tabs in `View()`; add `Ctrl+1-4` tab handlers; allow palette in onboarding; handle tab-switch-during-onboarding; remove Signals from tab list |
| `pkg/tui/command_picker.go` | Add tool-switching slash commands to `GlobalCommands()` |
| `cmd/autarch/main.go` | Remove SignalsView from dashboard views factory; update `--tool` flag docs |

**Estimate:** ~100-150 lines changed, 3 files.

### Phase 2: Gurgeh Absorbs Onboarding (Large refactor)

**Goal:** Eliminate `ModeOnboarding`/`ModeDashboard` distinction. Gurgeh becomes a self-contained
view that manages kickoff → sprint → spec → epics → tasks internally.

**Changes:**
- Move onboarding state machine from `UnifiedApp` into a new `GurgehOnboardingView` or expand
  `GurgehView` to manage sub-views
- Move all onboarding message handlers out of `UnifiedApp.Update()`
- Move breadcrumb into Gurgeh's content area
- Merge `App` and `UnifiedApp` into a single implementation
- Remove `--skip-onboard` flag (no-op with deprecation warning)
- Rewire view factories in `cmd/autarch/main.go`

**Files:**

| File | Changes |
|------|---------|
| `internal/tui/unified_app.go` | Remove onboarding state, breadcrumb, ~400 lines of transition handlers |
| `internal/tui/views/gurgeh.go` | Absorb onboarding orchestration, breadcrumb, view factories |
| `internal/tui/onboarding.go` | Move into gurgeh or restructure |
| `internal/tui/breadcrumb.go` | Move into gurgeh view |
| `cmd/autarch/main.go` | Simplify; remove separate `--skip-onboard` path |
| Tests | Update for new structure |

**Estimate:** ~800-1200 lines changed, 5-8 files. Design spike recommended before implementation.

### Phase 3: Signals Overlay (Deferred)

**Goal:** Convert Signals from a removed tab to an overlay accessible via `Ctrl+Shift+S`.

**Changes:**
- Create a new `SignalsOverlay` component (simplified rendering, no 3-pane layout)
- Add toggle state and rendering in `UnifiedApp`
- Wire `Ctrl+Shift+S` keybinding and `/signals` slash command to toggle

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
| Direct keybindings | `Ctrl+1-4` | Familiar browser/IDE pattern; Arbiter `Ctrl+1-3` conflict resolved by removal |
| Slash aliases | 3-letter (`/big`, `/gur`) | Single-letter aliases collide with `/back`, `/palette`, `/group` |
| Onboarding mode | Removed in Phase 2 | Gurgeh manages its own flow; simpler mental model |
| Implementation | 3 phases | Phase 1 is low-risk and delivers most UX value immediately |
