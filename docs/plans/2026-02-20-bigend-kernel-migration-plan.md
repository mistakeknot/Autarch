# Plan: Bigend Kernel Migration (E7)

> **PRD:** [docs/prds/2026-02-20-bigend-kernel-migration.md](../../docs/prds/2026-02-20-bigend-kernel-migration.md)
> **Epic bead:** iv-ishl (P1)
> **Feature beads:** iv-lemf (F1), iv-9au2 (F2), iv-gv7i (F3), iv-1d9u (F4), iv-4c16 (F5), iv-4zle (F6), iv-jaxw (F7), iv-xu31 (F8)

## Review Findings (flux-drive, 4 agents)

**Reviews:** [architecture](../../docs/research/architecture-review-of-e7-plan.md) · [correctness](../../docs/research/correctness-review-of-e7-plan.md) · [ux](../../docs/research/ux-review-of-e7-plan.md) · [performance](../../docs/research/performance-review-of-e7-plan.md)

**P0 fixes (pre-implementation):**
1. Move `seenEvents`/`seenOrder` update inside `a.mu.Lock()` block — data race with WebSocket addActivity()
2. Reorder `UnifiedStatus` iota so `StatusUnknown = 0` (zero value must not be Active per PRD)
3. F8.1: `UnifiedStatusSymbol` already exists in `pkg/tui/components.go:24` — only add `UnifiedStatusStyle` there (not styles.go)

**P1 fixes (during implementation):**
- F2.3: Move `mergeAgentDispatches` to `aggregator.go` (kernel.go is Intercore-only)
- F5.1: LRU dedup already exists (seenEvents/seenOrder, cap 500) — don't re-add; clarify dedup ownership vs per-call seen-map
- F6: Extract `items.go`, `render_dashboard.go`, `RunListPane` before adding to model.go (prevent god-object)
- F1: Parallelize 3 ic calls within each project goroutine (sequential blocks at N≥15)
- F3: Normalize projPath via EvalSymlinks at SyntheticID construction (prevent symlink collision)
- F4.2: Add explicit Enter-to-navigate keybinding for Active Runs (Enter is already bound to Toggle)
- F1: Add empty-state rendering when no kernel projects exist
- F4: Sidebar badge: show `[1 blocked]` in warning color when blocked>0, fall back to `[2 runs]`

## Current State

**Done (pre-committed):**
- Gates 1-4 all shipped in `22b3be2`
- F7 complete: `internal/icdata/kernelevents.go` — 12 event types, ParseKernelEvent(), String()
- F8 partial: `internal/icdata/unifiedstatus.go` — UnifiedStatus enum, UnifyStatus() mapper
- F8 partial: `TmuxSession.UnifiedState` field wired, `detectSessionState()` calls `icdata.UnifyStatus()`
- F1 partial: `discovery.Scanner` detects `.clavain/intercore.db` via `HasIntercore`, EvalSymlinks dedup
- F1 partial: `aggregator.State.Kernel *KernelState` field, `enrichWithKernelState()` with bounded concurrency (sem=5, 3s timeout), `enrichRuns()` live, `enrichDispatches()` live, `enrichEvents()` live
- F1 partial: `refreshing atomic.Bool` pileup guard
- F3 partial: `Activity.SyntheticID` and `Activity.Source` fields exist
- F3 partial: `kernelEventsToActivities()` converts events to Activities, `mergeActivities()` deduplicates
- F5 partial: `seenEvents`/`seenOrder` LRU already exists on Aggregator (cap 500, lines 121-122, 449-462)
- F8 partial: `UnifiedStatusSymbol()` and `UnifyStatusForRender()` already exist in `pkg/tui/components.go`

**No kernel rendering exists in TUI or web templates.** All rendering work is ahead.

## Delivery Order

```
F1 remaining ─┬─ F2 ──┐
              ├─ F3 ──┼─ F5
              └─ F4 ──┘
                             F6
F8 remaining (parallel with any)
```

F1 remaining delivers TUI sidebar badge + web; F2-F4 parallelize after F1; F5 depends on F3 (event stream); F6 is standalone TUI layout work. F8 remaining can slot in anywhere.

## Tasks

### F1 Remaining: TUI Sidebar Badge + Web Runs Section

**Bead:** iv-lemf

#### F1.1: Sidebar run count badge (`internal/bigend/tui/model.go`)

In project list item rendering, show active run count when `state.Kernel` is non-nil:

```
Interverse         [2 runs]
  .gurgeh .coldwine .clavain/
```

- Read `state.Kernel.Runs[project.Path]` and count items where `UnifyStatus(r.Status) == StatusActive`
- Badge style: dim when 0, normal when >0
- If `state.Kernel.Metrics.KernelErrors[project.Path]` is set, show `!` warning indicator after badge

Files: `internal/bigend/tui/model.go` (project list item rendering, ~line 380-420)

#### F1.2: Web dashboard runs section (`web/templates/dashboard.html`)

Add a "Kernel Runs" section to the dashboard template showing active runs per project.

- Template range over `.Kernel.Runs` when `.Kernel` is non-nil
- Show project name, run count, active/blocked/done breakdown
- Error projects show warning icon

Files: `internal/bigend/web/templates/dashboard.html`, `internal/bigend/web/handler.go` (ensure State is passed to template)

---

### F2: Dispatch Integration — TUI + Web

**Bead:** iv-9au2
**Depends on:** F1 remaining (sidebar badge establishes kernel rendering pattern)

#### F2.1: TUI dispatch view in project detail

When a project is selected and has kernel dispatches, show a dispatch section:

```
 Dispatches (3 active, 1 blocked)
 ─────────────────────────────────
 ● d-7a2  claude-opus-4  reviewing tests     active   2m14s
 ▲ d-7a3  claude-opus-4  blocked on gate     blocked  8m02s
 ○ d-7a1  claude-haiku   generating docs     done     1m30s
```

- Status icons from unified model: `●` active, `▲` blocked, `○` done, `✕` error, `~` waiting, `?` unknown
- Columns: ID (8), agent/model (16), task (fill), status (8), duration (6)
- Sort: active first, then blocked, then by start time desc

Files: `internal/bigend/tui/model.go` (new `renderDispatches()` method)

#### F2.2: Web dispatch list

Add dispatch list to project detail page template.

Files: `internal/bigend/web/templates/projects.html`

#### F2.3: Intermute + kernel merge logic

When an Intermute agent name matches a kernel dispatch agent name, merge into a single display row showing both inbox state (from Intermute) and lifecycle data (from kernel). Names that don't match show in separate sections.

Files: `internal/bigend/aggregator/aggregator.go` (new `mergeDispatchAgents()` in Refresh(), after both agents and kernelState are populated — kernel.go stays Intercore-only)

---

### F3: Event Stream — TUI + Web

**Bead:** iv-gv7i
**Depends on:** F1 remaining

#### F3.1: Source prefix tags in Activity rendering

All activities display with source prefix and color:
- `[K]` kernel — blue (#7aa2f7)
- `[M]` intermute — green (#9ece6a)
- `[T]` tmux — gray (#565f89)

Always shown, not conditional on filter state.

Files: `internal/bigend/tui/model.go` (activity rendering), `pkg/tui/styles.go` (source colors)

#### F3.2: TUI event section in dashboard tab

Add an "Activity Feed" section to the dashboard tab showing the merged stream from `state.Activities`. Use `pkg/tui.LogPane` if available, else a simple viewport.

- Timestamp format: `HH:MM:SS`
- Source tag + summary text
- Auto-scroll to newest

Files: `internal/bigend/tui/model.go` (dashboard tab rendering)

#### F3.3: Web event stream

Add unified activity stream to web dashboard and project detail pages.

Files: `internal/bigend/web/templates/dashboard.html`, `internal/bigend/web/templates/projects.html`

---

### F4: Dashboard Metrics

**Bead:** iv-1d9u
**Depends on:** F1 remaining

#### F4.1: TUI metrics row on dashboard tab

Display a stats bar at the top of the dashboard:

```
 4 projects · 3 active runs · 1 blocked · 5 dispatches · 12,450 in / 3,200 out tokens
```

- Read from `state.Kernel.Metrics`
- Blocked count in warning color (yellow) when >0
- Token totals formatted with comma separators
- `KernelErrors` count shown as "3/4 projects" when partial failure
- Zero-value graceful: show `0` with no errors

Files: `internal/bigend/tui/model.go` (dashboard tab, new `renderKernelMetrics()`)

#### F4.2: Cross-project active runs on dashboard

Flat list of active runs across all projects:

```
 Active Runs
 ──────────────────────────────────────────────
 Interverse  run-42  implement auth flow    plan     14m  ●
 Clavain     run-18  refactor dispatch      build    2h3m ●
```

Navigate: Enter opens project detail view focused on that run.

Columns: project (fill), run ID (8), goal (fill), phase (12), duration (6), status (1)

Files: `internal/bigend/tui/model.go` (dashboard tab)

#### F4.3: Web metrics

Template renders the same metrics on the web dashboard.

Files: `internal/bigend/web/templates/dashboard.html`

---

### F5: Event Viewport — Bootstrap-then-Stream

**Bead:** iv-4c16
**Depends on:** F3 (event stream must be flowing into Activities)

#### F5.1: Consolidate dedup ownership

LRU dedup already exists on Aggregator (`seenEvents`/`seenOrder`, cap 500, lines 121-122/449-462). Two fixes needed:
1. **Move seenEvents update inside `a.mu.Lock()` block** (P0 data race — currently outside any lock)
2. **Clarify dedup contract**: use Aggregator's LRU as the canonical seen-set; eliminate the per-call `seen` map in `mergeActivities()`. New signature: `appendNewActivities(existing, incoming, seenLRU, max)`. Skip sort when `len(incoming)==0`.

Files: `internal/bigend/aggregator/aggregator.go`, `internal/bigend/aggregator/kernel.go`

#### F5.2: Bootstrap batch

On first `enrichWithKernelState()` call (or when Aggregator starts), bootstrap with historical events. Pre-populate seen-set before emitting to Activities so historical events appear as history, not "new".

Files: `internal/bigend/aggregator/kernel.go`

#### F5.3: Event viewport TUI

Dedicated viewport using `pkg/tui.LogPane`:
- Auto-scrolls to newest
- Filter by event type, project, source
- Scroll up pauses auto-scroll, "G" jumps to bottom

Files: `internal/bigend/tui/model.go` (new viewport integration)

#### F5.4: Web event stream

Equivalent web template with auto-refresh.

Files: `internal/bigend/web/templates/dashboard.html`

---

### F6: Two-Pane Layout — Run List + Detail

**Bead:** iv-4zle
**Depends on:** F2 (dispatches must render), F4 (metrics visible)

This is the largest TUI task. It refactors project detail into a run-focused two-pane split.

#### F6.1: Run list pane

Left side of project detail: list of runs.

```
 run-42  implement auth flow        plan    14m  ██░░ ●
 run-41  add error handling         build   2h3m ████ ●
 run-40  setup CI pipeline          done    --   ████ ○
```

Columns (fixed-width): ID (8), status icon (1), phase (12), duration (6), progress bar (8), complexity badge (2) — goal fills remaining.

- Default filter: active + done/error from last 24h. `a` toggles full history.
- Phase duration derived from most recent `phase.advance` event timestamp
- Duration color: green (normal), yellow (>1h), red (>4h)

Files: `internal/bigend/tui/model.go` (new run list rendering)

#### F6.2: Run detail pane

Right side: dispatches, events, and token summary for selected run.

- Dispatches section (from F2 rendering)
- Events section filtered to selected run's events
- Token summary: `in: 12,450 / out: 3,200 / total: 15,650`

Files: `internal/bigend/tui/model.go`

#### F6.3: Focus ring

Tab/h/l cycles focus: sidebar → run list → detail pane. Arrow keys navigate within focused pane only. Focused pane shows distinct border color.

Extend existing `Pane` enum and `activePane` switching logic.

Files: `internal/bigend/tui/model.go` (key handling, pane rendering)

#### F6.4: Responsive narrow fallback

Below 100 columns: show run list only. Enter opens full-screen detail overlay, Esc returns. Selected run preserved in model state.

Files: `internal/bigend/tui/model.go` (layout calculation in `paneWidths()`)

---

### F8 Remaining: TUI Integration of UnifiedStatus

**Bead:** iv-xu31
**Can run in parallel with any feature**

#### F8.1: UnifiedStatus display components (`pkg/tui/`)

`UnifiedStatusSymbol()` and `UnifyStatusForRender()` already exist in `pkg/tui/components.go`. Only add `UnifiedStatusStyle(status icdata.UnifiedStatus) lipgloss.Style` to the same file:

| Status | Symbol (exists) | Color for UnifiedStatusStyle |
|--------|--------|-------|
| Active | `●` | green (#9ece6a) |
| Blocked | `▲` | yellow (#e0af68) |
| Waiting | `~` | blue (#7aa2f7) |
| Done | `○` | gray (#565f89) |
| Error | `✕` | red (#f7768e) |
| Unknown | `?` | dark gray (#414868) |

Also: reorder `internal/icdata/unifiedstatus.go` iota so `StatusUnknown = 0` (zero value = unknown, not active).

Files: `pkg/tui/components.go`, `internal/icdata/unifiedstatus.go`

#### F8.2: Wire TUI session rendering to UnifiedState

Replace `statusForSession()` (which re-runs tmux.Client.DetectStatus()) with a read from `TmuxSession.UnifiedState` set by the aggregator.

Files: `internal/bigend/tui/model.go` (session list item rendering)

#### F8.3: Eliminate redundant capture-pane

Remove capture-pane calls from the TUI render path. The aggregator already runs statedetect and stores the result.

Files: `internal/bigend/tui/model.go`

#### F8.4: All status display points use unified model

Sessions, agents, and dispatches all use `UnifiedStatusSymbol()` + `UnifiedStatusStyle()`.

Files: `internal/bigend/tui/model.go`, `internal/bigend/web/templates/sessions.html`, `internal/bigend/web/templates/agents.html`

---

## Implementation Sequence (Recommended)

| Phase | Features | Commit scope |
|-------|----------|-------------|
| 0 | P0 fixes: seenEvents lock, iota reorder, commit scaffolding | `fix(bigend): P0 review fixes — seenEvents race, iota order` |
| 1 | F8 remaining (add UnifiedStatusStyle only) | `feat(bigend): unified status style in TUI` |
| 2 | F1 remaining (sidebar badge + web + empty state) | `feat(bigend): kernel run count in sidebar + web` |
| 3 | F3 (source tags + activity feed + EvalSymlinks in SyntheticID) | `feat(bigend): event stream with source tags` |
| 4 | F2 (dispatch view) + F4 (metrics + blocked badge) | `feat(bigend): dispatch view + dashboard metrics` |
| 5 | F5 (consolidate dedup + bootstrap + event viewport) | `feat(bigend): bootstrap-then-stream event viewport` |
| 5.5 | Extract items.go, render_dashboard.go from model.go | `refactor(bigend): extract TUI types and dashboard rendering` |
| 6 | F6 (RunListPane + two-pane layout) | `feat(bigend): run list + detail two-pane layout` |

Rationale: Phase 0 fixes P0 issues found in review. F8 first because every other feature needs status styles. Phase 5.5 decomposes model.go before F6 adds ~300 lines. F6 implements RunListPane as a separate struct (like TerminalPane) to avoid bloating Model.Update().

## Testing Strategy

- Unit tests for each new rendering function (mock aggregator state)
- `kernel_test.go` already covers: enrichWithKernelState nil check, computeKernelMetrics, kernelEventsToActivities, mergeActivities
- New tests needed: `UnifiedStatusSymbol()` mapping, `mergeAgentDispatches()`, run list filtering, dedup seen-set LRU
- Race detector: `go test -race ./internal/bigend/...`
- TUI layout tests: `renderTwoPane()` at various widths (extend existing `model_layout_test.go`)
- Pre-existing test failure `TestSprintView_ChatSubmitProducesResponse` is unrelated (mock agent stream issue)

## Signal Broker Preparation

The PRD's remaining open question asks whether E7 should prepare hooks for iv-0v7j (signal broker). Answer: **no explicit hooks**, but the architecture is signal-broker-ready:

- `KernelState` is already a separate struct — a future signal broker can populate it via WebSocket instead of polling
- `mergeActivities()` dedup by SyntheticID works regardless of event source
- The polling interval (2s) is the only thing that changes — swap `Refresh()` timer for signal-driven updates

No code changes needed for signal broker preparation.
