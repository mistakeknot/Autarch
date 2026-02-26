# Plan: Merge Sprint Tab into Coldwine

**Bead:** iv-oguc3
**Phase:** executing (as of 2026-02-26T00:54:04Z)
**PRD:** docs/prds/2026-02-25-merge-sprint-into-coldwine.md
**Date:** 2026-02-25

## Overview

Merge RunDashboardView (Sprint tab, index 3) into ColdwineView (index 2) with three user-selectable layout modes. Remove the Sprint tab. Tab count drops from 5 to 4.

**Execution order:** Tasks 1-7 are sequential — each builds on the prior task's output. Tasks within each step can be verified independently before proceeding.

---

## Task 1: Extract RunDetailPanel (F1)
**Bead:** iv-msfpx
**Phase:** executing (as of 2026-02-26T00:54:04Z)
**Files:** `internal/tui/views/run_detail_panel.go` (new), `internal/tui/views/run_actions.go` (new), `internal/tui/views/run_detail_panel_test.go` (new), `pkg/tui/run_helpers.go` (new)
**Estimated effort:** Medium (1-2 hours)

Extract the rendering logic from `run_dashboard.go` into a reusable, embeddable component. **Critical boundary rule:** `RunDetailPanel` goes in `internal/tui/views/`, NOT `pkg/tui/`. The `pkg/tui/` layer has zero domain imports — adding `pkg/intercore` would violate the shared-style boundary and force Bigend/Gurgeh/Pollard to compile against Intercore.

Only domain-free helpers (`FormatTokens`, `RenderRunStatusBadge`) go to `pkg/tui/`.

### 1.1 Create `internal/tui/views/run_detail_panel.go`

```go
// RunDetailPanel renders sprint run detail: phase timeline, budget bar,
// gate conditions, dispatches list, event log. Embeddable in any parent view.
type RunDetailPanel struct {
    run        *intercore.Run
    dispatches []intercore.Dispatch
    budget     *intercore.BudgetResult
    events     []intercore.Event
    gate       *intercore.GateResult
    width      int
    height     int
    statusMsg  string
    maxEvents  int // 0 = default (8), configurable for compact mode
}
```

**Methods to extract from `run_dashboard.go`:**
- `renderDocument()` → `RunDetailPanel.Render() string` (lines 575-636)
- `renderPhaseTimeline()` → `RunDetailPanel.renderPhaseTimeline()` (lines 657-685)
- `renderBudget()` → `RunDetailPanel.renderBudget()` (lines 687-733)
- `renderGateStatus()` → `RunDetailPanel.renderGateStatus()` (lines 735-782)
- `renderDispatches()` → `RunDetailPanel.renderDispatches()` (lines 784-831)
- `renderEvents()` → `RunDetailPanel.renderEvents()` (lines 833-864)
- `renderUnavailable()` → `RunDetailPanel.renderUnavailable()` (lines 866-870)

**Public API (note: `Render`/`CompactRender`, NOT `View`/`CompactView` — avoids collision with `pkg/tui.View` interface):**
```go
func NewRunDetailPanel() *RunDetailPanel
func (p *RunDetailPanel) SetData(run *intercore.Run, dispatches []intercore.Dispatch, budget *intercore.BudgetResult, events []intercore.Event, gate *intercore.GateResult)
func (p *RunDetailPanel) SetSize(width, height int)
func (p *RunDetailPanel) SetMaxEvents(n int) // for compact inline mode
func (p *RunDetailPanel) SetStatusMsg(msg string)
func (p *RunDetailPanel) Render() string       // full: phase + budget + gate + dispatches + events
func (p *RunDetailPanel) CompactRender() string // compact: phase + budget + gate only
```

**Loading state contract:** `Render()` must display a "Loading sprint detail..." placeholder when `run` is non-nil but `dispatches`, `budget`, `events`, and `gate` are ALL nil. This is the intended loading state shown between expand/select and detail load completion.

### 1.2 Create `pkg/tui/run_helpers.go` (domain-free helpers only)

```go
// RenderRunStatusBadge returns a styled status badge string.
// Accepts a plain status string — no domain type dependency.
func RenderRunStatusBadge(status string) string

// FormatTokens formats a token count for display (e.g., "12.3k").
func FormatTokens(n int64) string
```

These are the only items from this refactor that belong in `pkg/tui/`.

### 1.3 Create `internal/tui/views/run_actions.go`

```go
// ShouldAutoAdvance checks if a completed dispatch should trigger phase advancement.
func ShouldAutoAdvance(run *intercore.Run, d intercore.Dispatch) bool

// LoadRunDetail fetches full run detail from Intercore in a single batch.
// Returns a tea.Cmd that produces RunDetailLoadedMsg.
func LoadRunDetail(ic *intercore.Client, runID string, seq uint64) tea.Cmd

// LoadRuns fetches active + inactive runs from Intercore.
// Returns a tea.Cmd that produces RunsLoadedMsg.
func LoadRuns(ic *intercore.Client, seq uint64) tea.Cmd

// RunsLoadedMsg carries the result of LoadRuns.
type RunsLoadedMsg struct {
    Runs []intercore.Run
    Err  error
    Seq  uint64 // generation counter — handler ignores msg.Seq < v.runsLoadSeq
}

// RunDetailLoadedMsg carries the result of LoadRunDetail.
type RunDetailLoadedMsg struct {
    Run        *intercore.Run
    Dispatches []intercore.Dispatch
    Budget     *intercore.BudgetResult
    Events     []intercore.Event
    Gate       *intercore.GateResult
    Err        error  // non-nil if any fetch failed; partial data still present
    Seq        uint64 // generation counter — handler ignores stale messages
}
```

**Generation counters:** Both message types carry a `Seq` field. `ColdwineView` tracks `v.runsLoadSeq` and `v.detailLoadSeq`, incremented on each load launch. Handlers ignore messages where `msg.Seq < v.currentSeq`. This prevents stale-data overwrites from rapid mode toggles or navigation.

### 1.4 Extract sidebar rendering

Create `renderRunSidebarItems(runs []intercore.Run) []pkgtui.SidebarItem` in `internal/tui/views/run_detail_panel.go` — moves lines 531-573 from `run_dashboard.go`. Accepts pre-constructed items (no `intercore.Run` in the signature exposed to `pkg/tui`).

### 1.5 Write tests

`internal/tui/views/run_detail_panel_test.go`:
- `TestRunDetailPanel_RenderEmpty` — nil run shows "Select a sprint run"
- `TestRunDetailPanel_RenderWithRun` — populated data renders all sections
- `TestRunDetailPanel_RenderLoadingState` — run non-nil, all detail nil → "Loading sprint detail..."
- `TestRunDetailPanel_CompactRender` — compact mode omits dispatches/events
- `TestRunDetailPanel_BudgetExceeded` — red bar + "BUDGET EXCEEDED" text
- `TestRunDetailPanel_GatePassed` / `GateBlocked` — correct icons
- `TestRenderRunSidebarItems` — correct icons and labels per status
- `TestShouldAutoAdvance` — conditions: autoAdvance enabled, active run, exit 0

### 1.6 Verify

```bash
go test ./internal/tui/views/... -race -count=1
go test ./pkg/tui/... -race -count=1
go build ./cmd/autarch/
```

---

## Task 2: Add Mode Toggle to ColdwineView (F2 + F7 stub)
**Bead:** iv-z6s9c
**Phase:** executing (as of 2026-02-26T00:54:04Z)
**Files:** `internal/tui/views/coldwine.go` (modify), `internal/tui/views/coldwine_mode.go` (new), `internal/tui/views/coldwine_mode_test.go` (new)
**Estimated effort:** Medium-large (2-3 hours)

### 2.1 Add mode state, layout types, and two panel instances to ColdwineView

In `coldwine.go`, add to the struct. **Note:** `LayoutMode` type and constants are defined here (not Task 7) because Tasks 3 and 4 depend on them.

```go
type ColdwineMode int
const (
    ModeEpics ColdwineMode = iota
    ModeRuns
)

type LayoutMode int
const (
    LayoutToggle LayoutMode = iota
    LayoutInline
    LayoutSplit
)

// Add to ColdwineView struct:
mode            ColdwineMode
layoutMode      LayoutMode

// Runs mode data (loaded only when entering Runs mode or on DispatchCompletedMsg)
runs            []intercore.Run
selectedRun     int
runsRunDetail   *RunDetailPanel  // for Runs mode document panel
epicsRunDetail  *RunDetailPanel  // for inline/split Epics mode sprint section

// Generation counters (ignore stale async load results)
runsLoadSeq     uint64
detailLoadSeq   uint64

// Inline expansion state
sprintExpanded  bool
```

**Two separate RunDetailPanel instances** — never share a single panel across modes. `runsRunDetail` is used by `runsModeDocument()`. `epicsRunDetail` is used by inline expansion (Task 3) and split pane (Task 4). This prevents cross-mode data corruption when `SetData` is called with partial data in one mode, then the user switches to the other mode.

Both are initialized in `NewColdwineView()`:
```go
v.runsRunDetail = NewRunDetailPanel()
v.epicsRunDetail = NewRunDetailPanel()
```

### 2.2 Create `coldwine_mode.go`

Separate file for mode-specific logic to keep coldwine.go manageable:

```go
// switchMode toggles between Epics and Runs modes.
func (v *ColdwineView) switchMode()

// SetRunsMode sets mode to ModeRuns (used by unified_app.go via modeSettable interface).
// Zero-argument method — no ColdwineMode type exported. Focus() handles the data load.
func (v *ColdwineView) SetRunsMode()

// loadRunsForMode fetches runs for Runs mode sidebar.
// Increments v.runsLoadSeq and passes seq to the tea.Cmd.
func (v *ColdwineView) loadRunsForMode() tea.Cmd

// handleRunsKey handles keys in Runs mode.
func (v *ColdwineView) handleRunsKey(msg tea.KeyMsg) tea.Cmd
  // a → advancePhase (uses v.runs[v.selectedRun].ID, NOT v.activeRun)
  // c → cancelRun (uses v.runs[v.selectedRun].ID, NOT v.activeRun)
  // up/down → navigate runs

// advancePhase derives run ID from v.runs[v.selectedRun].ID — TOCTOU-safe.
func (v *ColdwineView) advancePhase() tea.Cmd

// cancelRun derives run ID from v.runs[v.selectedRun].ID — TOCTOU-safe.
func (v *ColdwineView) cancelRun() tea.Cmd

// tryAutoAdvance ported from RunDashboardView. Uses run ID from v.runs[v.selectedRun].
// Uses actual AdvanceResult.FromPhase from server, not captured closure phase.
func (v *ColdwineView) tryAutoAdvance() tea.Cmd

// runsModeSidebarItems returns sidebar items for Runs mode.
func (v *ColdwineView) runsModeSidebarItems() []pkgtui.SidebarItem

// runsModeDocument returns document content for Runs mode.
func (v *ColdwineView) runsModeDocument() string
  // delegates to v.runsRunDetail.Render()
```

**Drop `SetMode(mode ColdwineMode)`** — only `SetRunsMode()` is needed from outside the package. Keep `ColdwineMode` unexported. If more modes are needed from outside `views` later, export at that point.

### 2.3 Modify SidebarItems()

```go
func (v *ColdwineView) SidebarItems() []pkgtui.SidebarItem {
    // Prepend mode toggle items (__ prefix = system-reserved, not entity IDs)
    toggle := []pkgtui.SidebarItem{
        {ID: "__mode_epics", Label: "Epics", Icon: modeIcon(v.mode, ModeEpics)},
        {ID: "__mode_runs", Label: "Runs", Icon: modeIcon(v.mode, ModeRuns)},
    }
    switch v.mode {
    case ModeRuns:
        return append(toggle, v.runsModeSidebarItems()...)
    default:
        return append(toggle, v.epicsSidebarItems()...)
    }
}
```

The existing epic sidebar logic moves to `epicsSidebarItems()`.

### 2.4 Modify View() and renderDocument()

```go
func (v *ColdwineView) View() string {
    sidebarItems := v.SidebarItems()
    var document string
    switch v.mode {
    case ModeRuns:
        document = v.runsModeDocument()
    default:
        document = v.renderDocument()
    }
    chat := v.chatPanel.View()
    return v.shell.Render(sidebarItems, document, chat)
}
```

### 2.5 Modify Update() — message routing

Add to the `tea.KeyMsg` handler in document focus:

```go
case msg.String() == "m":
    if v.shell.Focus() == pkgtui.FocusSidebar || v.shell.Focus() == pkgtui.FocusDocument {
        v.switchMode()
        if v.mode == ModeRuns {
            return v, v.loadRunsForMode()
        }
        return v, nil
    }
```

Add Run-mode keybindings (`a`, `c`) gated on `v.mode == ModeRuns`.

Handle `SidebarSelectMsg` for mode toggle items (sentinel cases BEFORE default):

```go
case pkgtui.SidebarSelectMsg:
    switch msg.ItemID {
    case "__mode_epics":
        v.mode = ModeEpics
        return v, nil
    case "__mode_runs":
        v.mode = ModeRuns
        return v, v.loadRunsForMode()
    default:
        // existing epic selection logic
    }
```

Handle `RunsLoadedMsg` and `RunDetailLoadedMsg` with generation counter checks:
```go
case RunsLoadedMsg:
    if msg.Seq < v.runsLoadSeq { return v, nil } // stale — ignore
    v.runs = msg.Runs
    // clamp selectedRun
    if v.selectedRun >= len(v.runs) { v.selectedRun = max(0, len(v.runs)-1) }
    // ...
case RunDetailLoadedMsg:
    if msg.Seq < v.detailLoadSeq { return v, nil } // stale — ignore
    v.runsRunDetail.SetData(msg.Run, msg.Dispatches, msg.Budget, msg.Events, msg.Gate)
    // ...
```

**DispatchCompletedMsg auto-advance — mode guard:**

Move `DispatchCompletedMsg` handling from RunDashboardView into ColdwineView. The auto-advance call itself is always safe (server enforces gate conditions), but **statusMsg must be rendered in both modes**. When mode is Epics and auto-advance fires, persist the status to the chat panel (not the document area which won't display it).

```go
case DispatchCompletedMsg:
    // Auto-advance is safe to call regardless of mode (server-enforced gates)
    if v.shouldAutoAdvance(msg.Dispatch) {
        cmd := v.tryAutoAdvance()
        // Status feedback: in Runs mode → statusMsg in doc area
        // In Epics mode → chat panel message so the user sees it
        if v.mode == ModeEpics {
            v.chatPanel.AddSystemMessage("Sprint auto-advanced after dispatch completed")
        }
        return v, cmd
    }
```

### 2.6 Modify Focus() — mode-aware loading

```go
func (v *ColdwineView) Focus() tea.Cmd {
    cmds := []tea.Cmd{v.loadData()} // always load epics
    if v.mode == ModeRuns {
        cmds = append(cmds, v.loadRunsForMode())
    }
    return tea.Batch(cmds...)
}
```

This ensures `/sprint` → `SetRunsMode()` → `switchToTab(2)` → `Focus()` properly loads runs data.

### 2.7 Modify Commands()

Return ALL commands always. Gate execution via runtime mode check inside Action closures (not by filtering at `Commands()` call time). This avoids the stale palette problem — `updateCommands()` is only called in `enterDashboard()`, not on mode switch.

**Critical: use message pattern for state mutations**, not direct field writes in Action closures. Action closures run on the goroutine pool, NOT the Update goroutine.

```go
type coldwineModeChangeMsg struct{ mode ColdwineMode }

func (v *ColdwineView) Commands() []tui.Command {
    return []tui.Command{
        {Name: "Switch Mode", Action: func() tea.Cmd {
            return func() tea.Msg {
                if v.mode == ModeEpics { return coldwineModeChangeMsg{ModeRuns} }
                return coldwineModeChangeMsg{ModeEpics}
            }
        }},
        // Epic commands — runtime-gated
        {Name: "New Epic", Action: func() tea.Cmd {
            if v.mode != ModeEpics { return nil }
            // ...existing logic...
        }},
        // Run commands — runtime-gated
        {Name: "Advance Phase", Action: func() tea.Cmd {
            if v.mode != ModeRuns { return nil }
            return func() tea.Msg { /* ... */ }
        }},
        // ...
    }
}
```

Handle `coldwineModeChangeMsg` in `Update()`.

### 2.8 Modify ShortHelp()

```go
func (v *ColdwineView) ShortHelp() string {
    switch v.mode {
    case ModeRuns:
        return "↑/↓ select run  a advance  c cancel  m mode  ctrl+r refresh  tab focus"
    default:
        return "↑/↓ navigate  d dispatch  m mode  ctrl+r refresh  ctrl+g model  tab focus"
    }
}
```

### 2.9 Write mode-switching tests

`internal/tui/views/coldwine_mode_test.go`:
- `TestColdwineView_ModeSwitchKeybinding` — send `tea.KeyMsg("m")` in FocusDocument, assert returned `tea.Cmd` is non-nil (loads runs)
- `TestColdwineView_SidebarSelectMsg_SentinelModeToggle` — verify sentinel IDs (`__mode_epics`, `__mode_runs`) trigger mode switch, not epic selection
- `TestColdwineView_DispatchCompletedMsg_EpicsMode` — auto-advance fires but status goes to chat panel, not document
- `TestColdwineView_DispatchCompletedMsg_RunsMode` — auto-advance fires, statusMsg shown in document
- `TestColdwineView_FocusModeAware` — Focus() with ModeRuns loads both epics and runs
- `TestColdwineView_StaleRunsLoadedMsg` — verify generation counter rejects stale messages

### 2.10 Verify

```bash
go test ./internal/tui/views/... -race -count=1  # existing coldwine_dispatch_test.go + new mode tests
go build ./cmd/autarch/
```

---

## Task 3: Inline Expansion Layout (F3)
**Bead:** iv-ek1z8
**Phase:** executing (as of 2026-02-26T00:54:04Z)
**Files:** `internal/tui/views/coldwine.go` (modify)
**Estimated effort:** Small-medium (1-2 hours)
**Depends on:** Task 2 (which defines `LayoutMode`, `sprintExpanded`, and `epicsRunDetail`)

### 3.1 Modify renderDocument() for inline mode

After the Tasks section (around line 631), add. **No allocation in View()** — uses the cached `v.epicsRunDetail` panel initialized in `NewColdwineView()`. Data is set in `Update()` handlers, not here.

```go
// Inline sprint expansion (only in LayoutInline mode, Epics mode)
if v.layoutMode == LayoutInline && v.mode == ModeEpics {
    if v.selected < 0 || v.selected >= len(v.epics) {
        // bounds guard — no epic selected
    } else {
        epic := v.epics[v.selected]
        if run, ok := v.epicRuns[epic.ID]; ok && run != nil {
            if v.sprintExpanded {
                lines = append(lines, "")
                lines = append(lines, pkgtui.SubtitleStyle.Render("Sprint "+run.ID))
                // v.epicsRunDetail already has data from Update() handler
                v.epicsRunDetail.SetMaxEvents(3)
                v.epicsRunDetail.SetSize(v.width-4, 12)
                lines = append(lines, v.epicsRunDetail.CompactRender())
            } else {
                lines = append(lines, "")
                lines = append(lines, pkgtui.LabelStyle.Render("  s expand sprint details"))
            }
        }
    }
}
```

### 3.2 Update `epicsRunDetail` data in Update() handlers

In the `epicRunsLoadedMsg` handler (and when `RunDetailLoadedMsg` arrives for the epic's run), update `v.epicsRunDetail.SetData(...)`. This keeps the data fresh without mutating state in `View()`.

### 3.3 Add `s` key handler

In the `FocusDocument` key handler — with bounds guard:

```go
case msg.String() == "s":
    if v.layoutMode == LayoutInline && v.mode == ModeEpics {
        v.sprintExpanded = !v.sprintExpanded
        // If expanding, load full detail for the epic's run
        if v.sprintExpanded && v.selected >= 0 && v.selected < len(v.epics) {
            epicID := v.epics[v.selected].ID
            if runID := v.getEpicRunID(epicID); runID != "" {
                v.detailLoadSeq++
                return v, LoadRunDetail(v.iclient, runID, v.detailLoadSeq)
            }
        }
    }
```

### 3.4 Verify

```bash
go build ./cmd/autarch/
```

---

## Task 4: Split Pane Layout (F4)
**Bead:** iv-6a82r
**Phase:** executing (as of 2026-02-26T00:54:04Z)
**Files:** `internal/tui/views/coldwine.go` (modify)
**Estimated effort:** Medium (1-2 hours)
**Depends on:** Task 2 (which defines `LayoutMode`, `epicsRunDetail`)

### 4.1 Modify View() for split pane — use existing SplitLayout

Use `pkg/tui/splitlayout.go` which already provides `SplitLayout` with `LeftWidth()`, `RightWidth()`, `IsStacked()`. Do NOT roll bespoke width arithmetic with `lipgloss.JoinHorizontal`.

```go
func (v *ColdwineView) View() string {
    // ...existing sidebar logic...

    if v.layoutMode == LayoutSplit && v.mode == ModeEpics {
        docWidth := v.width - v.shell.SidebarWidth()
        split := pkgtui.NewSplitLayout(0.5) // 50/50 epic vs sprint
        split.SetMinWidth(120)
        split.SetSize(docWidth, v.height)

        if !split.IsStacked() {
            // Split pane: left = epic detail, right = sprint detail
            epicDoc := v.renderDocument()
            sprintDoc := v.renderSprintPanelForEpic()
            document := split.Render(epicDoc, sprintDoc)
            return v.shell.Render(sidebarItems, document, chat)
        }
        // Width < 120: degrade to inline expansion with auto-expand
        v.sprintExpanded = true
        // Fall through to standard rendering (which includes inline expansion path)
    }
    // Normal or inline rendering...
}
```

### 4.2 Add renderSprintPanelForEpic() — pure reader, no View() mutation

`epicsRunDetail` data is set in `Update()` handlers (from `epicRunsLoadedMsg` and `RunDetailLoadedMsg`). This function is a pure reader.

```go
func (v *ColdwineView) renderSprintPanelForEpic() string {
    if v.selected < 0 || v.selected >= len(v.epics) {
        return "  No epic selected"
    }
    epic := v.epics[v.selected]
    run, ok := v.epicRuns[epic.ID]
    if !ok || run == nil {
        return "  No sprint for this epic"
    }
    return v.epicsRunDetail.Render()
}
```

### 4.3 Width degradation

When falling from Split to Inline due to narrow terminal, auto-set `v.sprintExpanded = true` so the sprint panel stays visible. This prevents the panel from silently disappearing during terminal resize.

### 4.4 Verify

```bash
go build ./cmd/autarch/
```

---

## Task 5: Orphan Run Pseudo-Epic (F5)
**Bead:** iv-kkc8o
**Phase:** executing (as of 2026-02-26T00:54:04Z)
**Files:** `internal/tui/views/coldwine.go` (modify)
**Estimated effort:** Small (30 min - 1 hour)
**Depends on:** Task 2 (runs mode data fields)

### 5.1 Detect orphan runs — called from BOTH async handlers

`computeOrphanRuns()` reads both `v.runs` and `v.epicRuns`, which are populated by separate async loads. **Call it from both handlers** — `epicRunsLoadedMsg` AND `RunsLoadedMsg` — whenever either input changes. Guard at top: only compute when both inputs are available.

```go
func (v *ColdwineView) computeOrphanRuns() {
    // Both inputs must be loaded — nil epicRuns means "not yet loaded", not "no epic runs"
    if v.runs == nil || v.epicRuns == nil {
        v.orphanRuns = nil
        return
    }
    // Runs with epic associations
    associated := make(map[string]bool)
    for _, run := range v.epicRuns {
        if run != nil {
            associated[run.ID] = true
        }
    }
    v.orphanRuns = nil
    for _, r := range v.runs {
        if !associated[r.ID] {
            v.orphanRuns = append(v.orphanRuns, r)
        }
    }
}
```

Wire into both message handlers:
```go
case epicRunsLoadedMsg:
    v.epicRuns = msg.epicRuns
    v.computeOrphanRuns() // re-compute with fresh epicRuns
    // ...

case RunsLoadedMsg:
    if msg.Seq < v.runsLoadSeq { return v, nil }
    v.runs = msg.Runs
    v.computeOrphanRuns() // re-compute with fresh runs
    // ...
```

Also chain `loadEpicRuns()` from `loadRunsForMode()` so `epicRuns` is refreshed when the user explicitly enters Runs mode (prevents stale association data from misclassifying runs as orphans).

### 5.2 Add to Epics sidebar

In `epicsSidebarItems()`, after normal epic items:

```go
if len(v.orphanRuns) > 0 {
    items = append(items, pkgtui.SidebarItem{
        ID:    "__unscoped_sprints",
        Label: fmt.Sprintf("Unscoped (%d)", len(v.orphanRuns)),
        Icon:  "◇",
    })
}
```

### 5.3 Handle selection

In `SidebarSelectMsg`:

```go
case "__unscoped_sprints":
    // Switch to Runs mode, filtered to orphan runs
    v.mode = ModeRuns
    return v, v.loadRunsForMode()
```

### 5.4 Verify

```bash
go build ./cmd/autarch/
```

---

## Task 6: Remove Sprint Tab, Rewire Navigation (F6)
**Bead:** iv-aj7hj
**Phase:** executing (as of 2026-02-26T00:54:04Z)
**Files:** `internal/tui/unified_app.go` (modify), `cmd/autarch/main.go` (modify), `internal/tui/views/run_dashboard.go` (delete)
**Estimated effort:** Medium (1-2 hours)

### 6.1 Update main.go view factory

```go
app.SetDashboardViewFactory(func(c *autarch.Client) []tui.View {
    bigend := views.NewBigendView(c)
    bigend.SetIntercore(iclient)
    coldwine := views.NewColdwineView(c)
    coldwine.SetIntercore(iclient)
    // Sprint tab removed — merged into Coldwine
    return []tui.View{
        bigend,
        views.NewGurgehView(c, gurgehCfg),
        coldwine,
        views.NewPollardView(c, researchCoord),
    }
})
```

### 6.2 Update tab names in unified_app.go

```go
tabNames := []string{"Bigend", "Gurgeh", "Coldwine", "Pollard"}
```

### 6.3 Rewire slash command shortcuts

In the `SlashCommandMsg` handler. **Use name-based tab lookup** (more resilient than hardcoded index 2):

```go
case "coldwine", "cold":
    return a, a.switchToTab(2) // stays same index
case "sprint", "spr":
    // Switch to Coldwine AND activate Runs mode via sprintModeActivator interface
    for i, dv := range a.dashViews {
        if strings.ToLower(dv.Name()) == "coldwine" {
            if sm, ok := dv.(sprintModeActivator); ok {
                sm.SetRunsMode()
            }
            return a, a.switchToTab(i)
        }
    }
    return a, a.switchToTab(2) // fallback
case "pollard", "pol":
    return a, a.switchToTab(3) // was 4, now 3
```

**Note:** Only the `sprintModeActivator` interface approach works. Do NOT use `interface{ SetMode(ColdwineMode) }` — that would require importing `views.ColdwineMode`, causing a circular import.

### 6.4 Update --tool flag handling

In main.go, add alias:

```go
// In the initial tab resolution:
if toolFlag == "sprint" {
    toolFlag = "coldwine"
    // Set runs mode after view creation (deferred via message or SetMode)
}
```

### 6.5 Update SpecHandoffMsg handler

Index stays at 2 (Coldwine) — no change needed. But verify the type assertion still works since ColdwineView hasn't changed its interface.

### 6.6 Delete run_dashboard.go

After verifying everything builds and tests pass:

```bash
rm internal/tui/views/run_dashboard.go
```

**Deletion checklist — verify ALL of these are ported before deleting:**
- Rendering logic → `internal/tui/views/run_detail_panel.go`
- `loadRuns`, `loadDetail` → `internal/tui/views/run_actions.go`
- `shouldAutoAdvance` → `ShouldAutoAdvance` in `run_actions.go`
- `tryAutoAdvance` → `ColdwineView.tryAutoAdvance()` in `coldwine_mode.go` (produces Coldwine-local advance message type, NOT the old `runDashAdvancedMsg`)
- `advancePhase`, `cancelRun` → `ColdwineView` methods in `coldwine_mode.go`
- Sidebar rendering → `renderRunSidebarItems` in `run_detail_panel.go`
- Sprint keybindings (`a`, `c`, auto-advance toggle) → `handleRunsKey` in `coldwine_mode.go`
- `DispatchCompletedMsg` handler → ColdwineView `Update()` with mode guard

### 6.7 Add sprintModeActivator interface

ColdwineMode stays unexported in `internal/tui/views/`. Use the same interface pattern as `specHandoffReceiver`:

```go
// In internal/tui/unified_app.go:
type sprintModeActivator interface {
    SetRunsMode()
}
```

Then `ColdwineView.SetRunsMode()` sets `v.mode = ModeRuns`. Focus() handles the data load when the tab becomes active.

### 6.8 Verify

```bash
go test ./... -race -count=1
go build ./cmd/autarch/
# Manually verify: autarch tui --tool=sprint opens Coldwine in Runs mode
# Manually verify: /sprint in chat switches to Coldwine + Runs mode
```

---

## Task 7: Layout Mode Setting (F7)
**Bead:** iv-5zqjq
**Phase:** executing (as of 2026-02-26T00:54:04Z)
**Files:** `internal/coldwine/config/config.go` (modify), `internal/tui/views/coldwine.go` (modify)
**Estimated effort:** Small (30 min)

Note: `LayoutMode` type, constants, and the `layoutMode` field are already defined in Task 2. This task adds persistence and the config wiring.

### 7.1 Add LayoutMode to config

In `internal/coldwine/config/config.go`:

```go
type TUIConfig struct {
    ConfirmApprove bool   `toml:"confirm_approve"`
    LayoutMode     string `toml:"layout_mode"` // "toggle" (default), "inline", "split"
}
```

### 7.2 Wire config to view

In `NewColdwineView`, accept layout mode via functional option:

```go
func WithLayoutMode(mode LayoutMode) ColdwineOpt {
    return func(v *ColdwineView) { v.layoutMode = mode }
}
```

In `main.go` view factory, parse config and pass:
```go
layoutMode := views.LayoutToggle // default
switch cfg.TUI.LayoutMode {
case "inline": layoutMode = views.LayoutInline
case "split":  layoutMode = views.LayoutSplit
}
coldwine := views.NewColdwineView(c, views.WithLayoutMode(layoutMode))
```

### 7.3 Add command palette entries — use message pattern

Layout mode changes must go through `Update()`, NOT direct field writes in closures. Use the `layoutModeChangedMsg` pattern from Task 2.7:

```go
type layoutModeChangedMsg struct{ mode LayoutMode }

// In Commands():
tui.Command{
    Name: "Layout: Mode Toggle",
    Action: func() tea.Cmd {
        return func() tea.Msg { return layoutModeChangedMsg{LayoutToggle} }
    },
},
tui.Command{
    Name: "Layout: Inline Expansion",
    Action: func() tea.Cmd {
        return func() tea.Msg { return layoutModeChangedMsg{LayoutInline} }
    },
},
tui.Command{
    Name: "Layout: Split Pane",
    Action: func() tea.Cmd {
        return func() tea.Msg { return layoutModeChangedMsg{LayoutSplit} }
    },
},
```

Handle in `Update()`:
```go
case layoutModeChangedMsg:
    v.layoutMode = msg.mode
    // Persist to config (best-effort, don't block)
    go v.saveLayoutMode(msg.mode)
```

### 7.4 Verify

```bash
go build ./cmd/autarch/
```

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Circular import (views ↔ tui) | High | Low | `sprintModeActivator` interface (same pattern as `specHandoffReceiver`) |
| RunDetailPanel rendering differs from original | Low | Medium | Side-by-side visual comparison before deleting run_dashboard.go |
| Mode toggle confuses users who expect Sprint tab | Medium | Low | `/sprint` shortcut preserved; opens Coldwine in Runs mode |
| Split pane layout breaks on narrow terminals | Low | Low | Auto-degrades to inline at <120 cols via SplitLayout.IsStacked() |
| Auto-advance in wrong mode | Low | Medium | Always fires (server-safe), statusMsg routed to chat panel when in Epics mode |
| TOCTOU on advance/cancel | Medium | High | Derive run ID from `v.runs[v.selectedRun].ID`, not cached `activeRun` pointer |
| Stale async loads on rapid mode toggle | Medium | Medium | Generation counters (`runsLoadSeq`, `detailLoadSeq`) reject stale messages |
| Cross-mode panel data corruption | Low | High | Two separate `RunDetailPanel` instances (`runsRunDetail`, `epicsRunDetail`) |
| Palette Action goroutine race | Medium | Medium | Emit messages from closures, handle in Update() — never write fields directly |

## Dependency Graph

```
F1 (RunDetailPanel) → F2 (Mode Toggle + LayoutMode stub + F7 types) → F3 (Inline)
F1 → F2 → F4 (Split Pane)
F1 → F2 → F5 (Orphan Pseudo-Epic)
F1 → F2 → F6 (Tab Removal)
F7 (Config Persistence) — independent, can run any time after F2
```

Critical path: F1 → F2 → F6. Tasks 3, 4, 5 can be parallelized after F2. Task 7 is independent after F2.

## Review Findings Addressed

This plan incorporates all findings from the flux-drive review (architecture, correctness, quality):

| Finding | Fix Applied |
|---------|-------------|
| A1: RunDetailPanel in pkg/tui imports pkg/intercore | Moved to internal/tui/views/ — only FormatTokens/RenderRunStatusBadge in pkg/tui |
| A2: SetMode(ColdwineMode) circular import | Removed — use only SetRunsMode() via sprintModeActivator interface |
| A3: Panel allocation in View() | Reuse struct fields (runsRunDetail, epicsRunDetail), init in NewColdwineView |
| A4: Manual split pane duplicates SplitLayout | Use existing pkg/tui/splitlayout.go |
| C1: Auto-advance fires silently in Epics mode | statusMsg routed to chat panel when mode=Epics |
| C2: TOCTOU advance/cancel on wrong run | Use v.runs[v.selectedRun].ID, not v.activeRun |
| C3: Orphan detection races two async loads | computeOrphanRuns called from both handlers, guard on both inputs non-nil |
| C-H1: Shared runDetail corrupted across modes | Two separate RunDetailPanel instances |
| C-H3: /sprint sets mode but no data load | Focus() is mode-aware, loads runs when ModeRuns |
| Q-F1: View() name collision with tui.View interface | Renamed to Render()/CompactRender() |
| Q-F7: Palette Action closures race on goroutine pool | Emit messages from closures, handle in Update() |
| Q-F8: layoutMode used in Tasks 3/4 before defined in Task 7 | Moved LayoutMode type/field to Task 2 |
| Q-F9: No tests for mode switching | coldwine_mode_test.go added to Task 2 |
| Q-F4: Loading state not specified | Added loading placeholder contract to Task 1 |
| C-M2: Rapid mode toggle causes stale overwrites | Generation counters on RunsLoadedMsg/RunDetailLoadedMsg |
