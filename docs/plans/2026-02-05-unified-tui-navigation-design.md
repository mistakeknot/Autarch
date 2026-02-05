# Unified TUI Navigation Design

**Date:** 2026-02-05
**Status:** Approved
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
| `/bigend` | `/b`, `/1` | Switch to Bigend |
| `/gurgeh` | `/g`, `/2` | Switch to Gurgeh |
| `/coldwine` | `/c`, `/3` | Switch to Coldwine |
| `/pollard` | `/p`, `/4` | Switch to Pollard |
| `/signals` | `/sig` | Toggle Signals overlay |

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

## Implementation

### Files to Modify

| File | Changes |
|------|---------|
| `internal/tui/unified_app.go` | Remove mode distinction; always show tabs; add `Ctrl+1-4` and `Ctrl+Shift+S` handlers; add signals overlay state |
| `pkg/tui/command_picker.go` | Add tool-switching slash commands to `GlobalCommands()` |
| `internal/tui/views/gurgeh.go` | Absorb kickoff view + breadcrumb as internal state |
| `internal/tui/views/signals.go` | Convert from View to overlay component |
| `cmd/autarch/main.go` | Simplify startup; deprecate `--skip-onboard` |
| `AGENTS.md` | Update keybindings documentation |
| `CLAUDE.md` | Update quick commands |

### Files to Remove/Deprecate

- `internal/tui/breadcrumb.go` — Move into Gurgeh or delete

### Rough Estimate

~400-600 lines changed across 6-8 files.

## Deferred Work

- **Bigend Agent Intelligence** — Agent reviews signals and surfaces suggestions, alerts, and clarifying questions to the user. Separate design needed.

## Decision Log

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Tab count | 4 (not 5) | Signals is infrastructure, not a workspace |
| Signals access | Overlay | Keeps tabs focused on tools users work *in* |
| Direct keybindings | `Ctrl+1-4` | Familiar pattern (browsers, IDEs) |
| Onboarding mode | Removed | Gurgeh manages its own flow; simpler mental model |
| Slash command aliases | Yes (`/g` for `/gurgeh`) | Keyboard efficiency for power users |
