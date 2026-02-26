# PRD: Merge Sprint Tab into Coldwine

**Bead:** iv-oguc3

## Problem

Users context-switch between the Sprint tab (execution monitoring) and the Coldwine tab (task orchestration) to manage their work. Sprint's features (phase timeline, budget, gates, events) are disconnected from Coldwine's task/epic context, creating a fragmented workflow.

## Solution

Merge RunDashboardView into ColdwineView with three user-selectable layout modes: mode toggle (default), inline expansion, and split pane. Remove the Sprint tab entirely, reducing tab count from 5 to 4.

## Features

### F1: Extract RunDetailPanel Component
**What:** Refactor RunDashboardView's rendering logic into a reusable `RunDetailPanel` that can be embedded in any parent view.
**Acceptance criteria:**
- [ ] `RunDetailPanel` struct in `pkg/tui/` renders phase timeline, budget bar, gate conditions, dispatches list, event log
- [ ] Accepts `intercore.Run`, `[]intercore.Dispatch`, `*intercore.BudgetResult`, `[]intercore.Event`, `*intercore.GateResult` as input
- [ ] Handles its own `SetSize(width, height)` for responsive layout
- [ ] Existing RunDashboardView tests pass against the extracted component
- [ ] Auto-advance logic (`tryAutoAdvance`) extracted into a reusable function

### F2: Mode Toggle Layout
**What:** Add Epics/Runs mode toggle to ColdwineView sidebar, switching between epic list and run list with corresponding detail panels.
**Acceptance criteria:**
- [ ] Sidebar shows `[Epics]` / `[Runs]` toggle at top
- [ ] `m` key toggles mode when sidebar is focused
- [ ] In Runs mode: sidebar lists all Intercore runs, detail panel shows RunDetailPanel for selected run
- [ ] In Epics mode: existing Coldwine behavior preserved exactly
- [ ] Selection state preserved independently per mode (switching back restores prior selection)
- [ ] Sprint keybindings (`a` advance, `c` cancel) active only in Runs mode
- [ ] Epic keybindings (`d` dispatch) active only in Epics mode
- [ ] Command palette dynamically shows mode-appropriate commands

### F3: Inline Expansion Layout
**What:** In Epics mode, expand the epic detail panel to show sprint monitoring info below the tasks section.
**Acceptance criteria:**
- [ ] Collapsible "Sprint" section appears below Tasks when epic has an associated run
- [ ] `s` key toggles section visibility (collapsed by default)
- [ ] Shows phase timeline, budget summary, gate status, top 3 dispatches, top 3 events
- [ ] Hidden entirely for epics without an associated run
- [ ] Section respects available height — no overflow beyond terminal bounds

### F4: Split Pane Layout
**What:** Side-by-side epic detail and sprint detail in the document area.
**Acceptance criteria:**
- [ ] Document area splits vertically: left = epic detail, right = RunDetailPanel
- [ ] Activates only when terminal width >= 120 cols; degrades to inline expansion below that
- [ ] Both panels independently scrollable
- [ ] Sprint panel auto-selects the run associated with the current epic
- [ ] Falls back gracefully when epic has no associated run (right panel shows "No sprint")

### F5: Orphan Run Pseudo-Epic
**What:** Runs not associated with any epic appear under a synthetic "Unscoped Sprints" entry in the Epics sidebar.
**Acceptance criteria:**
- [ ] Synthetic "Unscoped Sprints" epic appears at bottom of Epics sidebar when orphan runs exist
- [ ] Selecting it shows a minimal view listing the orphan runs with status/phase
- [ ] Clicking a run in the list switches to Runs mode with that run selected
- [ ] Disappears when no orphan runs exist
- [ ] Not persisted to Coldwine DB — purely synthetic/runtime

### F6: Tab Removal and Rewiring
**What:** Remove Sprint tab from unified_app.go and rewire all navigation shortcuts.
**Acceptance criteria:**
- [ ] Tab bar: Bigend=0, Gurgeh=1, Coldwine=2, Pollard=3 (4 tabs total)
- [ ] `/sprint` and `/spr` switch to Coldwine tab AND activate Runs mode
- [ ] `/coldwine` and `/cold` switch to Coldwine tab in Epics mode
- [ ] `--tool=sprint` CLI flag maps to `--tool=coldwine` with Runs mode auto-activated
- [ ] RunDashboardView source file can be deleted (all logic in RunDetailPanel + ColdwineView)
- [ ] No references to the old Sprint tab index (3) remain in the codebase
- [ ] `DispatchCompletedMsg` handling (auto-advance) works within ColdwineView

### F7: Layout Mode Setting
**What:** Persist the user's layout mode preference and expose it via command palette.
**Acceptance criteria:**
- [ ] Layout mode stored in `.coldwine/config.yaml` (or equivalent)
- [ ] Command palette: "Layout: Mode Toggle / Inline / Split Pane" to switch
- [ ] Default: mode toggle
- [ ] Setting survives session restart
- [ ] Mode applied on ColdwineView initialization

## Non-goals

- **Real-time event streaming.** Events are fetched on-demand (existing poll pattern), not streamed via WebSocket.
- **Multi-run comparison.** Only one run is displayed at a time in the detail panel.
- **Epic creation from Runs mode.** Users switch to Epics mode to create epics. No new UI in Runs mode for this.
- **Responsive mode auto-switching.** The user explicitly chooses their layout mode. We don't auto-switch between modes based on terminal size (except split pane degradation).

## Dependencies

- `pkg/intercore` client (already exists in both views)
- `pkg/tui` shared components (sidebar, shell layout, chat panel — already shared)
- `.coldwine/` data directory (already exists)

## Open Questions

1. **Mode toggle UI placement:** Sidebar top vs document header? Leaning sidebar for discoverability.
2. **Lazy polling:** Should Intercore queries (runs, events, gate) only fire when in Runs mode or when sprint section is expanded?
3. **Chat persona:** Use ColdwineChatHandler for both modes with a mode-aware system prompt adjustment?
