# Merge Sprint Tab into Coldwine as a Mode Toggle

**Bead:** iv-oguc3
**Date:** 2026-02-25
**Status:** Brainstorm complete

## What We're Building

Merge the Sprint tab (RunDashboardView, tab index 3) into Coldwine (tab index 2), reducing the tab count from 5 to 4. Coldwine gains a mode toggle between Epics view and Runs view, plus two additional layout options (inline expansion and split pane) that the user can configure.

### Current State

**Sprint tab (RunDashboardView)** — 883 lines:
- Sidebar: list of all Intercore runs (active + inactive), icon by status
- Detail panel: phase timeline, budget bar, gate conditions, dispatches list, event log (last 8)
- Actions: advance phase (`a`), cancel (`c`), toggle auto-advance, research spec
- Slash commands: `/sprint status|advance|cancel|list|create`, `/dispatch list|spawn`

**Coldwine tab** — 909 lines:
- Sidebar: list of epics
- Detail panel: epic header, stories list, tasks list (with dispatch annotations)
- Actions: create epic/story/task, dispatch task (`d`), create sprint (scoped to epic), sync to Intermute
- Sprint integration: brief inline annotation `Sprint: <id> Phase: <phase>` in epic header
- Slash commands: same `/sprint` and `/dispatch` via shared SprintCommandRouter

**Overlap:** Both share SprintCommandRouter slash commands. Both show sprint info — Sprint shows full detail, Coldwine shows a one-liner. Both react to `DispatchCompletedMsg`.

## Why This Approach

The Sprint tab duplicates information already partially shown in Coldwine. Users context-switch between tabs to see their epics and their sprint execution status. Merging them means the user can see task orchestration AND execution monitoring in one place.

Three layout modes give the user control over information density vs. simplicity:

1. **Mode toggle** (default) — sidebar switches between Epics and Runs lists. Clean separation, same screen real estate.
2. **Inline expansion** — sprint detail appears below tasks in the epic detail panel. No mode switching, everything visible at once, but requires scrolling on small terminals.
3. **Split pane** — epic detail on the left, sprint detail on the right. Maximum information density, requires ~120 cols.

## Key Decisions

1. **All three layout modes will be built.** User selects via setting or command palette. Default: mode toggle.

2. **Mode toggle is primary.** Sidebar shows `[Epics]` / `[Runs]` toggle buttons at the top. When in Runs mode, the sidebar lists runs and the detail panel shows the full Sprint view (phase timeline, budget, gate, dispatches, events). When in Epics mode, shows the current Coldwine view.

3. **Orphan runs get a pseudo-epic.** Runs without an epic association are grouped under a synthetic "Unscoped Sprints" entry in the Epics sidebar. This ensures no runs are invisible in Epics mode.

4. **Sprint keybindings activate contextually.** In Runs mode: `a` (advance), `c` (cancel) work as they do today. In Epics mode: `d` (dispatch) works as today. `a` and `c` are inactive in Epics mode to avoid accidental phase changes.

5. **Tab navigation shortcuts preserved.** `/sprint` and `/spr` switch to Coldwine tab AND activate Runs mode. `/coldwine` and `/cold` switch to Coldwine tab in Epics mode. This maintains muscle memory.

6. **Sprint tab removed from unified_app.go.** Tab indices shift: Bigend=0, Gurgeh=1, Coldwine=2, Pollard=3. The `--tool=sprint` CLI flag becomes an alias for `--tool=coldwine --mode=runs`.

7. **Auto-advance stays in Coldwine.** The `DispatchCompletedMsg` → `tryAutoAdvance` flow moves into Coldwine's Runs mode handler.

## Layout Mode Details

### Mode Toggle (default)
- Sidebar header: two clickable/focusable labels `[Epics]` `[Runs]`
- Keyboard: `m` key to toggle mode (when sidebar focused), or command palette "Switch Mode"
- Sidebar items update on toggle — Epics list vs Runs list
- Detail panel renders EpicDetailView or RunDetailView depending on mode
- State preserved independently — switching to Runs and back keeps your epic selection

### Inline Expansion
- Epic detail panel gains a collapsible "Sprint" section below Tasks
- Only visible when the epic has an associated run
- Section shows: phase timeline, budget summary, gate status, dispatch count
- Collapsed by default; `s` key toggles sprint section visibility
- Dispatches and events truncated to 3 items each (expand via `/dispatch list`)

### Split Pane
- Document area splits vertically: left = epic detail, right = sprint detail
- Only activates when terminal is ≥120 cols; degrades to inline expansion below that
- Left/right panels independently scrollable
- Sprint panel auto-selects the run associated with the current epic (if any)

## Migration Path

1. Move RunDashboardView's rendering logic into a reusable `RunDetailPanel` component
2. Integrate `RunDetailPanel` into ColdwineView for all three layout modes
3. Move Sprint-specific keybindings and command palette items into ColdwineView (mode-gated)
4. Add mode toggle UI to sidebar
5. Update unified_app.go: remove Sprint tab, adjust tab indices
6. Update `/sprint` slash command to activate Runs mode within Coldwine
7. Add layout mode setting (persisted in `.coldwine/config.yaml` or similar)

## Open Questions

1. **Should the mode toggle be visible in the sidebar or in the document header?** Sidebar placement is more discoverable but costs vertical space. Header placement is cleaner but less obvious.
2. **Event polling in Runs mode:** Currently Sprint polls events independently. Should Coldwine poll only when in Runs mode to avoid unnecessary Intercore queries in Epics mode?
3. **Chat handler persona:** Sprint uses generic `ClaudeChatHandler`; Coldwine uses `ColdwineChatHandler` with a "Coldwine" persona. The merged view should probably use ColdwineChatHandler always, but should it adjust its system prompt when in Runs mode to be more sprint-aware?
