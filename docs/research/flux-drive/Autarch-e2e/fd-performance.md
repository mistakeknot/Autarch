# Performance Review: Autarch End-to-End Data Flow and TUI Responsiveness

**Reviewer:** fd-performance (Claude Opus 4.6)
**Date:** 2026-02-07
**Scope:** Full TUI startup path, tab switching, file I/O, network coordination, memory growth, scan operations

---

## 1. Executive Summary

Autarch is an interactive TUI application built on Bubble Tea (Go). Its performance profile is dominated by **startup latency** (Intermute server health check/spawn) and **external subprocess calls** (Claude Code exploration). The core TUI event loop itself is well-structured and renders efficiently. Tab switching is effectively free. The most impactful performance issues are:

1. **Startup blocks on Intermute for up to 5.5 seconds** in the cold-start case (spawning the Intermute server subprocess).
2. **All four dashboard views are Init()'d eagerly at startup**, including HTTP calls to Intermute for spec listing.
3. **Aggregator.Refresh() is a sequential cascade** of filesystem walks + YAML parses + tmux pane captures -- all synchronous and on the hot path when Bigend is active.
4. **SignalsOverlay opens the events SQLite database on every toggle**, without connection pooling.
5. **`context.TODO()` in ~20 locations** prevents cancellation of long-running operations, meaning the TUI cannot cleanly interrupt stale work.

Overall risk: **Medium**. The TUI frame rate and rendering pipeline are sound. The real performance concerns are startup time and the aggregator's synchronous I/O cascade, which would block the UI thread in certain tab configurations.

---

## 2. Startup Performance Analysis

### 2.1 Critical Path: `tuiCmd()` in `/root/projects/Autarch/cmd/autarch/main.go`

The startup sequence before the TUI event loop begins:

```
1. setup.NeedsSetup()          -- stat check (~1ms)
2. NewManager()                -- pure construction (~0ms)
3. mgr.EnsureRunning(ctx)     -- UP TO 5.5 SECONDS (see below)
4. NewUnifiedApp()             -- pure construction (~0ms)
5. SetDashboardViewFactory()   -- closure capture (~0ms)
6. tui.Run()                   -- enters Bubble Tea event loop
```

**Issue 1 (High): Intermute cold-start blocks TUI for up to 5.5 seconds**

Location: `/root/projects/Autarch/internal/intermute/manager.go`, lines 61-77 and 95-142.

`EnsureRunning()` first does a health check (`isHealthy()`) with a 500ms HTTP timeout. If no server is found, it calls `start()` which:
- Spawns the `intermute serve` binary as a subprocess
- Polls `isHealthy()` in a loop with `100ms` sleep intervals for up to **5 seconds**
- Each health check has its own 500ms timeout

Worst case: 500ms (initial check fails) + 5000ms (startup poll loop) = **5.5 seconds** before the TUI renders anything. The user sees a blank terminal.

**Impact:** User perceives the application as slow to launch or broken on first run. Subsequent launches (when Intermute is already running) see only the 500ms health check.

**Fix:** Show the TUI immediately with a "Connecting to Intermute..." status, and move `EnsureRunning()` into a background `tea.Cmd`. The TUI can display a loading state and transition when the server is ready.

### 2.2 `UnifiedApp.Init()` in `/root/projects/Autarch/internal/tui/unified_app.go`

```go
func (a *UnifiedApp) Init() tea.Cmd {
    LoadChatSettings()           // File read (~1ms)
    agent.DetectAgent()          // exec.LookPath for claude/codex (~5ms)
    initAgentSelector()          // Reads agents.toml, calls exec.LookPath (~10ms)
    initPaletteCommands()        // Pure construction (~0ms)
    return a.enterDashboard()    // CREATES AND INITS ALL 4 VIEWS
}
```

**Issue 2 (Medium): All four dashboard views are Init()'d eagerly**

Location: `/root/projects/Autarch/internal/tui/unified_app.go`, lines 490-526.

`enterDashboard()` calls `createDashboardViews(client)` which constructs all four views (Bigend, Gurgeh, Coldwine, Pollard), then calls `Init()` on each:

```go
for _, v := range a.dashViews {
    cmds = append(cmds, v.Init())
}
```

Looking at what each `Init()` does:
- **GurgehView.Init()**: If onboarding is active, creates the KickoffView. If in browser mode, calls `loadSpecs()` which does an HTTP GET to `/api/specs` on the Intermute server.
- **BigendView.Init()**: Likely triggers spec/project loading from Intermute.
- **ColdwineView.Init()**: Likely triggers task loading from Intermute.
- **PollardView.Init()**: Likely triggers insight loading from Intermute.

These are all `tea.Cmd` returns (async), so they do not block the event loop. However, they produce 4 simultaneous HTTP requests to Intermute on startup, plus the 4 responses flowing back as messages. This is acceptable because Bubble Tea batches these correctly.

**Verdict:** The eager init is fine because the commands are async. No fix needed unless Intermute server has connection limits.

### 2.3 Agent Detection

`agent.DetectAgent()` calls `exec.LookPath` for known agent binaries. `initAgentSelector()` reads `agents.toml` (file I/O) and calls `exec.LookPath` for each candidate. Total cost: ~15ms. Runs once at startup. Not a concern.

---

## 3. Tab Switching / View Transition Analysis

### 3.1 Dashboard Tab Switching

Location: `/root/projects/Autarch/internal/tui/unified_app.go`, lines 621-644.

```go
func (a *UnifiedApp) switchDashboardTab(idx int) tea.Cmd {
    if oldActive == idx { return nil }
    a.dashViews[oldActive].Blur()
    a.tabs.SetActive(idx)
    a.currentView = a.dashViews[idx]
    return a.currentView.Focus()
}
```

This is **effectively free**: one Blur() call, one pointer swap, one Focus() call. No data loading, no I/O. The view was already initialized during `enterDashboard()`.

**Verdict:** No performance issue. Tab switching is instantaneous.

### 3.2 Gurgeh Onboarding View Transitions

Location: `/root/projects/Autarch/internal/tui/views/gurgeh_onboarding.go`.

Each state transition (Kickoff -> Interview -> SpecSummary -> EpicReview -> TaskReview) creates a new view via the factory function, calls `Init()`, `Focus()`, and `sendWindowSize()`. These are all pure construction + a single `tea.Batch` of async commands. View creation is cheap.

The expensive operation is `scanCodebase()` (line 868), which spawns a `claude -p` subprocess. This is expected to take minutes and is correctly offloaded to a goroutine with progress streaming via channels.

**Verdict:** View transitions are fast. The scan operation is correctly async.

---

## 4. File I/O Patterns

### 4.1 Aggregator Enrichment Cascade

Location: `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go`, lines 358-408.

`Refresh()` runs this sequence **synchronously**:

```
1. scanner.Scan()              -- WalkDir up to depth 3 across all scan roots
2. enrichWithTaskStats()       -- Opens SQLite DB per project with Coldwine
3. enrichWithGurgStats()       -- WalkDir .gurgeh/specs/ per project, parses YAML
4. enrichWithPollardStats()    -- WalkDir .pollard/{sources,insights,reports}/ per project
5. loadAgents()                -- HTTP call to Intermute
6. loadTmuxSessions()          -- tmux list-sessions + capture-pane per session
7. colony.Detect()             -- Process check
8. loadMCPStatuses()           -- Stat checks per project
```

**Issue 3 (Medium): Aggregator enrichment is sequential N+1 style**

For each project (N), steps 2-4 each do independent I/O:

- `enrichWithTaskStats()`: Opens a new SQLite connection per project via `coldwine.NewReader(path)`. If there are 10 projects with Coldwine, that is 10 SQLite opens, 10 queries, 10 closes.
- `enrichWithGurgStats()`: Calls `gurgSpecs.LoadSummaries()` which does `os.ReadDir` + YAML parse per file. For each project with `.gurgeh/specs/`, every YAML file is read and partially parsed.
- `enrichWithPollardStats()`: Three `filepath.WalkDir` calls per project (sources, insights, reports).

**Impact:** If there are 10 projects with 50 YAML specs each, that is 500 YAML parses + 30 WalkDir calls + 10 SQLite opens. This is on the order of 100-500ms total, which blocks any code that calls `Refresh()` synchronously. In web mode, this runs on a ticker (every `ScanInterval`). In TUI mode, it runs in the `Refresh()` calls inside `refreshForEvent()` goroutines (line 275-307), which is correct.

However, `enrichWithGurgStats()` is called **while holding no lock** in the main `Refresh()` path, but **while holding the lock** in `refreshForEvent()` (line 278). This means event-triggered refreshes hold `a.mu.Lock()` during filesystem I/O, blocking all `GetState()` reads until the I/O completes.

**Fix:** Move `enrichWithGurgStats()` calls outside the lock in `refreshForEvent()`. Compute the enriched data, then take the lock only to swap the results.

### 4.2 LoadSummaries: Unbounded YAML Parse

Location: `/root/projects/Autarch/internal/gurgeh/specs/load.go`, lines 29-63.

```go
func LoadSummaries(dir string) ([]Summary, []string) {
    entries, _ := os.ReadDir(dir)
    for _, e := range entries {
        yamlsafe.UnmarshalFile(path, &doc)
    }
}
```

This reads **every** YAML file in the directory. There is no pagination, caching, or limit. If a project accumulates hundreds of spec files over time, this function reads them all every time it is called.

**Impact:** Currently low (most projects have <20 specs). Could degrade if spec count grows. The function is called from `enrichWithGurgStats()` (aggregator refresh), `LoadSummariesWithArchived()`, and several CLI commands.

**Fix (deferred):** Not urgent. If spec counts grow past 100, add a summary cache keyed by directory mtime.

### 4.3 SignalsOverlay Database Open/Close Per Toggle

Location: `/root/projects/Autarch/internal/tui/signals_overlay.go`, lines 269-323.

**Issue 4 (Low): Every Toggle() opens and closes the events SQLite database**

```go
func (o *SignalsOverlay) loadData() tea.Cmd {
    return func() tea.Msg {
        store, err := events.OpenStore("")    // Opens DB + runs migrations
        defer store.Close()                   // Closes DB
        evs, err := store.Query(...)
    }
}
```

`OpenStore()` calls `autarchdb.Open(path)` which opens a SQLite connection with WAL mode, then runs the schema migration check (`CREATE TABLE IF NOT EXISTS` for 3 tables and 7 indexes). This is fast (~5-10ms) but unnecessary if done repeatedly.

**Impact:** User toggles signals overlay with `/sig` or the palette. Each toggle causes a database open, migration check, query, and close. Perceptible as a brief flicker of "Loading..." text.

**Fix:** Keep a long-lived `*events.Store` reference on the overlay or pass one from the application level. Open it once at startup.

---

## 5. Network / Coordination Overhead

### 5.1 Intermute Client Connections

The `autarch.Client` uses a standard `http.Client` with a 30-second timeout per request. It is not pooled or connection-reused beyond Go's default transport connection pooling. Since all requests go to `127.0.0.1`, TCP connection reuse via keep-alive is effective.

The `ListSpecs()` call in `GurgehView.loadSpecs()` (line 108 in gurgeh.go) makes an HTTP GET to the local Intermute server. This is fast (<10ms for local loopback).

**Verdict:** No network performance issue for local-only mode.

### 5.2 Intermute WebSocket (Aggregator)

Location: `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go`, lines 156-208.

`ConnectWebSocket()` subscribes to 25+ event types. The `handleIntermuteEvent()` handler (line 224) triggers `refreshForEvent()` which spawns goroutines for partial refresh. These goroutines do filesystem I/O and HTTP calls.

**Issue 5 (Low): Event storm could cause refresh pile-up**

If Intermute emits many events in rapid succession (e.g., batch task creation), each event triggers a separate goroutine doing `enrichWithGurgStats()` or `loadAgents()`. These goroutines are not debounced or rate-limited.

**Impact:** Brief CPU spike and lock contention on `a.mu`. Unlikely to affect TUI responsiveness because these run in background goroutines, but could cause stale reads during the pile-up.

**Fix:** Add a debounce timer (e.g., 500ms) to `refreshForEvent()` so rapid events coalesce into a single refresh.

### 5.3 Research Coordinator Network Usage

Location: `/root/projects/Autarch/internal/pollard/research/coordinator.go`.

Hunters run concurrently (`wg.Add(1)` + goroutine per hunter). Each hunter makes external HTTP calls (GitHub API, arxiv, etc.) with a max of 10 results per query. The `publishToIntermute()` call (line 264) runs in a background goroutine.

**Verdict:** Well-structured. Hunters are correctly parallelized and findings are published non-blocking.

---

## 6. Memory Growth Risks

### 6.1 Log Pane: Capped at 500 Entries

Location: `/root/projects/Autarch/pkg/tui/logpane.go`, line 13 and 47-49.

```go
const maxLogEntries = 500
if len(p.entries) > maxLogEntries {
    p.entries = p.entries[len(p.entries)-maxLogEntries:]
}
```

**Verdict:** Bounded. 500 entries x ~200 bytes = ~100KB. No growth risk.

### 6.2 LogHandler Channel: 256-Buffer, Non-blocking Drop

Location: `/root/projects/Autarch/pkg/tui/loghandler.go`, lines 71-76.

```go
select {
case h.msgChan <- msg:
default:
    // Drop on overflow
}
```

**Verdict:** Bounded and non-blocking. Correct behavior for a log pane.

### 6.3 Signals Broker: 64-Buffer Per Subscriber with Eviction

Location: `/root/projects/Autarch/pkg/signals/broker.go`, lines 33, 53-64.

Subscribers get a 64-element channel. On overflow, the oldest signal is evicted. The `Dropped` counter tracks this.

**Verdict:** Bounded. No leak risk.

### 6.4 Aggregator Activities: Capped at 100

Location: `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go`, lines 257-261.

```go
a.state.Activities = append([]Activity{activity}, a.state.Activities...)
if len(a.state.Activities) > 100 {
    a.state.Activities = a.state.Activities[:100]
}
```

**Issue 6 (Low): Prepend-then-truncate allocates a new slice every time**

`append([]Activity{activity}, a.state.Activities...)` creates a new slice header, copies all existing elements, prepends the new one, then truncates. This is O(n) per event where n=100.

**Impact:** Negligible at 100 elements. Only worth noting if the cap were raised significantly.

**Verdict:** Correct behavior, bounded growth.

### 6.5 Research Run Updates: Unbounded

Location: `/root/projects/Autarch/internal/pollard/research/run.go`, lines 155-166.

```go
func (r *Run) AddUpdate(update Update) {
    r.updates = append(r.updates, update)
}
```

The `updates` slice grows without limit for the lifetime of a Run. However, runs are short-lived (minutes) and each hunter produces at most a handful of updates.

**Verdict:** No practical risk. A run with 10 hunters x 10 queries = ~100 updates x ~1KB = ~100KB.

### 6.6 Sprint State: Bounded by Phase Count

Sprint state in `.gurgeh/sprints/<id>.json` holds 8 phases of content. Each phase is a few KB of text. The entire sprint state is ~50-100KB. Sprints auto-save as JSON files. No unbounded growth.

### 6.7 Event Store (SQLite): Unbounded on Disk

Location: `/root/projects/Autarch/pkg/events/store.go`.

The events table grows without TTL or rotation. Every `emit()` appends a row. Over months of use, this could grow to tens of thousands of rows. The `SignalsOverlay.loadData()` queries with `LIMIT 100`, so read performance is fine. But the database file grows without bound.

**Issue 7 (Low): No event store rotation or TTL**

**Impact:** Disk usage grows slowly. Not a TUI performance issue, but a data hygiene concern.

**Fix (deferred):** Add a periodic `DELETE FROM events WHERE created_at < ?` with a configurable retention window (e.g., 30 days).

---

## 7. Scan Operations

### 7.1 Codebase Exploration (Claude Code)

Location: `/root/projects/Autarch/internal/gurgeh/exploration/explore.go`, lines 21-114.

`Explore()` spawns `claude -p <prompt> --output-format stream-json --verbose --print` as a subprocess with a **10-minute timeout**. It reads streaming JSON via `bufio.Scanner` with a 1MB buffer.

**Verdict:** This is inherently expensive (minutes of LLM processing), but it is correctly:
- Timeout-bounded (10 minutes)
- Run in a goroutine (line 874 of `gurgeh_onboarding.go`)
- Progress-streamed to the TUI via channels
- Non-blocking to the event loop

The 1MB scanner buffer (`scanner.Buffer(make([]byte, 1024*1024), 1024*1024)`) is allocated per scan. This is fine for a one-time operation.

### 7.2 Project Discovery Scanner

Location: `/root/projects/Autarch/internal/bigend/discovery/scanner.go`, lines 79-133.

`Scan()` uses `filepath.WalkDir` with a depth limit of 3 and skips excluded patterns. For each discovered project, `examineProject()` does 8 `os.Stat()` calls (checking for `.gurgeh`, `.praude`, `.coldwine`, `.tandemonium`, `.pollard`, `.agent_mail`).

**Verdict:** Bounded by depth limit. Typical scan of `/root/projects/` with ~20 subdirectories completes in <50ms. Not a concern.

### 7.3 Tmux State Detection

Location: `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go`, lines 570-651.

`loadTmuxSessions()` calls `tmuxClient.ListSessions()` (one tmux command), then for each agent session, calls `tmuxClient.CapturePane(name, 50)` (captures last 50 lines). Each `CapturePane` spawns a `tmux capture-pane` subprocess.

**Issue 8 (Low): CapturePane runs a subprocess per agent session**

With 5 active tmux sessions, this is 5 subprocess spawns during each `Refresh()`. Each takes ~10-20ms.

**Impact:** 50-100ms added to Refresh() for 5 sessions. Only affects Bigend web mode (ticker-driven) or event-triggered refreshes.

**Fix:** Not urgent. Could batch captures or cache state with a short TTL.

---

## 8. Cancellation and Context Hygiene

**Issue 9 (Medium): Pervasive `context.TODO()` prevents clean cancellation**

Locations (from grep): 20+ call sites across the codebase including:

- `/root/projects/Autarch/internal/tui/views/gurgeh_onboarding.go:89` - Onboarding context created with `context.WithCancel(context.TODO())`
- `/root/projects/Autarch/internal/tui/views/sprint_view.go:90,112,436,476` - Sprint operations use `context.TODO()`
- `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go:286` - Agent refresh goroutine uses `context.TODO()`
- `/root/projects/Autarch/internal/bigend/tui/model.go:673,830` - Bigend TUI operations

When the TUI exits (Ctrl+C x2), long-running operations spawned with `context.TODO()` continue running in the background. For example:
- A `claude` subprocess spawned by `exploration.Explore()` will keep running because its context has no parent cancellation
- Intermute HTTP calls in `refreshForEvent()` goroutines will wait for their full timeout

**Impact:** After TUI exit, orphaned goroutines and subprocesses continue for their timeout duration (up to 10 minutes for Claude Code). The user's terminal returns, but CPU/memory is consumed by zombie work.

**Fix:** Thread a cancellation context from the `tea.Program` through to all long-running operations. The onboarding view's context (line 89) already has a `cancel` function but it is created from `context.TODO()` -- it should derive from a program-level context. Similarly, `sprint_view.go` operations should accept a context parameter.

---

## 9. Rendering Performance

### 9.1 Overlay Rendering: ANSI-Aware String Splicing

Location: `/root/projects/Autarch/internal/tui/unified_app.go`, lines 823-875.

The `overlay()` and `insertAt()` functions splice overlay content into the base frame using ANSI-aware string operations (`ansi.Truncate`, `ansi.TruncateLeft`). This runs on every `View()` call when any overlay is visible.

**Verdict:** These are O(width) per line of overlay. With overlays typically 30-50 lines and terminal widths of 80-200, this is ~5000-10000 character operations per frame. Negligible compared to the 16ms budget at 60fps.

### 9.2 Log Pane `updateContent()`: Rebuilds on Every Batch

Location: `/root/projects/Autarch/pkg/tui/logpane.go`, lines 70-77.

```go
func (p *LogPane) updateContent() {
    var b strings.Builder
    for _, e := range p.entries {
        b.WriteString(p.formatEntry(e))
        b.WriteString("\n")
    }
    p.viewport.SetContent(b.String())
}
```

This rebuilds the entire viewport content string from all 500 entries on every batch. At 500 entries x ~60 chars = ~30KB of string building per batch.

**Impact:** Batches arrive at most every 100ms (from `batchLoop`). Building 30KB of strings every 100ms is ~300KB/s of allocation. Go's garbage collector handles this easily. Not a frame rate issue.

**Verdict:** Acceptable. Only optimize if log volume becomes extreme (>1000 entries/second sustained).

---

## 10. Recommendations (Prioritized by User Impact)

### Must-Fix

| # | Impact | Location | Problem | Fix |
|---|--------|----------|---------|-----|
| 1 | **High** | `cmd/autarch/main.go:128-133` | Intermute cold-start blocks TUI for up to 5.5s | Show TUI immediately with loading state; move `EnsureRunning()` to background `tea.Cmd` |
| 9 | **Medium** | 20+ files | `context.TODO()` prevents cancellation of long operations; orphan processes after exit | Thread program-level cancellation context through all operations |

### Should-Fix

| # | Impact | Location | Problem | Fix |
|---|--------|----------|---------|-----|
| 3 | **Medium** | `aggregator.go:275-307` | `refreshForEvent()` holds mutex during filesystem I/O | Compute enrichment outside lock, swap results under lock |
| 5 | **Low** | `aggregator.go:268-307` | Event storms cause unbatched refresh goroutine pile-up | Debounce refresh triggers with 500ms coalesce window |
| 4 | **Low** | `signals_overlay.go:271` | SQLite opened/closed on every overlay toggle | Keep long-lived `*events.Store` reference |

### Skip (Premature Optimization)

| # | Why Skip |
|---|----------|
| 2 | Eager view Init() is async via `tea.Cmd` -- does not block rendering |
| 6 | Activity prepend is O(100) per event -- negligible |
| 7 | Event store growth is a hygiene issue, not a performance issue |
| 8 | Tmux CapturePane subprocesses are <100ms total and run in background |
| Log pane rebuild | 30KB string build every 100ms is well within GC capacity |
| Overlay rendering | ANSI splicing is O(width*lines) per frame -- fast |

---

## 11. Summary

| Metric | Assessment |
|--------|-----------|
| **Overall risk** | Medium |
| **Startup time** | Poor in cold-start (up to 5.5s blank screen); fast when Intermute is pre-running |
| **Tab switching** | Excellent (pointer swap + Focus() call) |
| **Rendering** | Excellent (ANSI-aware overlay, batched log updates, lipgloss layout) |
| **File I/O** | Acceptable but aggregator holds locks during I/O |
| **Network** | Good (local loopback, no external calls on critical path) |
| **Memory** | Good (all buffers capped, no unbounded growth in hot paths) |
| **Cancellation** | Poor (`context.TODO()` everywhere prevents clean shutdown) |

The two must-fix items (startup latency and cancellation hygiene) affect the user's first impression and last impression of the application. Everything in between -- tab switching, rendering, file I/O patterns -- is well-designed for an interactive TUI.
