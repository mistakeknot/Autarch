# Bigend Roadmap

> Multi-project AI agent mission control TUI

## Vision

Bigend is a read-only aggregator that monitors agent activity, displays run progress, and provides a dashboard view across projects. It is part of the Autarch suite — Layer 3 of the 3-layer Interverse architecture:

```
Layer 3: Apps (Autarch) — Bigend, Gurgeh, Coldwine, Pollard
Layer 2: OS (Clavain) — the autonomous software agency
Layer 1: Kernel (Intercore) — runs, phases, gates, dispatches, events
```

Bigend never writes to the kernel — it only reads. For any action that implies policy (creating runs, advancing phases, overriding gates), Bigend submits intents to the OS layer (Clavain). This makes Bigend the lowest-risk Intercore migration and the first validation that the kernel provides sufficient observability data.

**Key tech decisions:**
- Go + Bubble Tea TUI (primary interface)
- Shared `pkg/tui` component library (ShellLayout, ChatPanel, Tokyo Night theme)
- Read-only consumer of kernel state
- htmx web interface (secondary, preserved for non-terminal access)

---

## Milestones

### M0: Foundation (Done)

**Goal:** Working mission control with filesystem discovery and tmux agent monitoring

| Component | Status | Implementation |
|-----------|--------|----------------|
| Project discovery | Done | `discovery/scanner.go` — scans for Gurgeh/Coldwine/Pollard/AgentMail/Intercore markers |
| tmux integration | Done | `tmux/client.go`, `tmux/detector.go` — lists sessions, detects agents, captures pane output |
| State detection | Done | `statedetect/` — heuristic agent state detection (working/waiting/blocked/stalled/done/error) with confidence scoring |
| Colony detection | Done | `colony/` — detects multi-agent colonies on same project |
| Agent command resolution | Done | `agentcmd/resolver.go` — identifies running agent commands |
| MCP Agent Mail reader | Done | `mcp/manager.go` — reads agent registrations and inbox counts |
| Coldwine task reader | Done | `coldwine/reader.go` — reads task stats from Coldwine |
| Aggregator | Done | `aggregator/aggregator.go` — central state aggregation from all data sources |
| Signal broker | Done | `pkg/signals/broker.go` — in-process pub/sub with WebSocket streaming |
| Bubble Tea TUI | Done | `tui/model.go`, `tui/render_dashboard.go` — dashboard with filtering, pane views, terminal output |
| Web interface | Done | `web/server.go` — htmx + Tailwind web dashboard (secondary) |
| Daemon mode | Done | `daemon/server.go` — background data collection and serving |
| Kernel data reader | Done | `aggregator/kernel.go` — initial Intercore data reading (partial) |

**Deliverable:** `./dev bigend` starts a TUI showing discovered projects, active tmux sessions with agent detection, state confidence, colony grouping, and live terminal output. Web mode available via `./dev bigend --web`.

**Data sources (current):**
- Filesystem scanning for project markers (`.gurgeh/`, `.coldwine/`, `.pollard/`, `.agent_mail/`, `.intercore/`)
- tmux CLI commands for session/pane data
- MCP Agent Mail SQLite databases
- Coldwine task databases

---

### M1: Autarch Status Tool

**Goal:** Minimal TUI that validates the full Intercore stack — the primary wedge

The Autarch Status Tool is the first app that ships because it validates the kernel's query APIs without requiring the complexity of full Bigend. It answers the most common question: "what's running right now?"

| Task | Priority | Description |
|------|----------|-------------|
| Run list view | P0 | Display active runs with phase progress via `ic run list` |
| Event stream tail | P0 | Live-updating event stream via `ic events tail --all --consumer=autarch-status` |
| Dispatch dashboard | P0 | Active dispatches with agent/model/duration via `ic dispatch list --active` |
| Discovery inbox | P1 | Confidence-tiered review of kernel discoveries |
| `pkg/tui` integration | P0 | Build on shared ShellLayout, Tokyo Night theme |
| Kernel API validation | P0 | Confirm `ic` query APIs provide sufficient data for real-time display |

**Mockup:**
```
+-- Autarch Status -------------------------------------------+
|                                                              |
| Runs           Phase        Dispatches   Events              |
| ----------     ---------    ----------   ------              |
| R42 auth       executing    3 active     47 total            |
| R41 refactor   shipping     0 active     23 total            |
|                                                              |
| Active Dispatches                                            |
| -----------------------------------------------             |
| D12 reviewer-arch   running  2m14s  Opus                     |
| D13 reviewer-qual   running  1m48s  Haiku                    |
| D14 reviewer-safe   completed 3m02s Sonnet                   |
|                                                              |
| Event Stream (last 5)                                        |
| -----------------------------------------------             |
| 14:23:01  dispatch.completed  D14  reviewer-safe             |
| 14:22:58  gate.passed         R42  plan-review               |
| 14:22:45  phase.advanced      R42  executing                 |
|                                                              |
+--------------------------------------------------------------+
```

**Why ship this first:**
1. Validates kernel query APIs are sufficient for real-time display
2. Validates `pkg/tui` components work with kernel data
3. Zero agency logic — pure rendering of kernel state
4. Becomes the foundation for Bigend's kernel migration (M2)

**Deliverable:** Standalone TUI binary that reads all data from `ic` commands and displays run/dispatch/event state in real time.

---

### M2: Kernel Data Migration

**Goal:** Swap Bigend's data sources from filesystem/tmux to Intercore kernel queries

Bigend is a pure observer. Migration replaces the current discovery and monitoring data sources with kernel equivalents:

| Current Source | Kernel Replacement | Notes |
|---------------|-------------------|-------|
| `discovery/scanner.go` (filesystem scan) | `ic run list` across project databases | Projects are those with active or recent runs |
| `tmux/detector.go` (tmux scraping) | `ic dispatch list --active` | Dispatches replace tmux sessions as the source of truth |
| `aggregator/aggregator.go` (polling loop) | `ic events tail --all --consumer=bigend` | Event-driven instead of poll-driven |
| Per-project stat collection | Kernel aggregates | Runs per state, dispatches per status, token totals |

| Task | Priority | Description |
|------|----------|-------------|
| `ic run list` integration | P0 | Replace filesystem project discovery with kernel run queries |
| `ic dispatch list` integration | P0 | Replace tmux agent detection with kernel dispatch data |
| `ic events tail` integration | P0 | Replace polling with kernel event consumption |
| Kernel aggregate metrics | P1 | Dashboard totals from kernel (runs per state, dispatch counts, tokens) |
| Dual-read fallback | P1 | Prefer kernel data, fall back to legacy sources during migration |
| Legacy source removal | P2 | Remove filesystem/tmux data paths once kernel sources are validated |

**Migration coexistence:** During migration, Bigend reads from both legacy sources and the kernel. Kernel data takes precedence when available. Legacy fallback ensures no data loss if a project hasn't been registered with Intercore yet.

**Implementation notes:**
- `aggregator/kernel.go` already has initial kernel data reading — extend to cover all data sources
- The aggregator's `State` struct remains the same; only the data sources feeding it change
- tmux integration is preserved as a supplementary data source for projects not yet using Intercore

**Deliverable:** Bigend dashboard shows the same information as M0 but sourced from kernel state. Legacy data sources remain as fallback.

---

### M3: Signal Broker Integration

**Goal:** Connect Bigend's existing signal broker to Intercore's event bus for sub-second TUI updates

The signal broker pattern is already proven in Bigend and Coldwine code (`pkg/signals/broker.go`). This milestone connects it to the kernel's event bus so TUI views receive push-based updates instead of polling.

| Task | Priority | Description |
|------|----------|-------------|
| Kernel event bus consumer | P0 | Connect signal broker to `ic events tail` as a streaming consumer |
| Typed subscriptions | P0 | TUI views subscribe to specific event types (dispatch.completed, phase.advanced, gate.passed) |
| Backpressure handling | P1 | Evict-oldest-on-full for view consumers; blocking for audit consumers |
| Reconnection logic | P1 | Broker rebuilds from kernel cursor position on disconnect |
| WebSocket relay | P2 | Web interface receives events via WebSocket from the broker |

**Architecture (from autarch-vision.md):**
- The signal broker is an embedded goroutine within the Autarch app process — not a standalone daemon
- It starts when Bigend launches and dies with that process
- All broker state is derived from the kernel event log and can be rebuilt from scratch on restart
- The kernel's durable event log remains the source of truth; the broker is a real-time projection

**Implementation notes:**
- `pkg/signals/broker.go` already implements in-process pub/sub with fan-out
- `aggregator/signal_convert.go` already converts between aggregator events and signal types
- The gap is connecting the broker's input to `ic events tail` instead of the aggregator's polling loop

**Deliverable:** TUI dashboard updates within 200ms of kernel events. No polling required for real-time data.

---

### M4: Rendering Performance

**Goal:** Implement FrankenTUI rendering patterns for large-scale dashboard performance

These patterns are drawn from the FrankenTUI research synthesis (see `docs/research/frankentui-research-synthesis.md`). They address specific performance problems that emerge when Bigend monitors many projects and agents simultaneously.

| Pattern | Bead | Priority | Description |
|---------|------|----------|-------------|
| Dirty row tracking | iv-t217 | P2 | Three-tier system (row bitmap, dirty spans, cell bitmap). Skip 90%+ of diff work on stable frames. Current Bigend renders entire frames every tick. |
| Budget degradation | iv-m33r | P2 | PID-controlled quality levels: Full -> SimpleBorders -> PlainText -> MinimalLayout -> SkipFrame. Graceful degradation when aggregator has many items. |
| Virtualized lists | iv-8nly | P2 | Fenwick tree for O(log n) scroll-to-index with variable-height items. Current dispatch/run lists scan linearly. Three strategies: Fixed, Variable (LRU cache), VariableFenwick. |
| Resize coalescing | iv-a0zv | P2 | Burst detection with 40ms coalescing delay, 16ms steady-state response, 100ms hard deadline. Fairness guard prevents input starvation during resize. |
| Inline mode | iv-omzb | P2 | Scrollback-preserving agent output display. TerminalWriter protocol: save cursor, clear UI region, render, restore. Logs write above UI naturally. |
| One-writer rule | iv-mvc6 | P3 | Serialize all terminal writes through a single writer. Prevents concurrent ANSI corruption from async updates — a problem already encountered in Bigend. |

**Implementation notes:**
- These are Go adaptations of Rust patterns. The architectural ideas port directly; the SIMD/cache-alignment details do not.
- Dirty row tracking and budget degradation provide the highest value-to-effort ratio.
- Virtualized lists become critical when monitoring 10+ projects with many dispatches each.
- One-writer rule addresses an existing intermittent rendering corruption bug.

**Deliverable:** Bigend maintains smooth rendering when monitoring 20+ projects with 50+ concurrent dispatches.

---

### M5: Agent Activity Feed

**Goal:** Unified timeline of all agent activity sourced from kernel events

Replaces the current tmux-scraping-based activity detection with a proper event-driven timeline. Kernel events provide richer, more reliable activity data than heuristic tmux pane analysis.

| Task | Priority | Description |
|------|----------|-------------|
| Kernel event timeline | P0 | Unified activity stream from `ic events tail --all --consumer=bigend` |
| Event type rendering | P0 | Distinct rendering for dispatch.started, dispatch.completed, phase.advanced, gate.passed, gate.failed, run.created, run.completed |
| Cross-project aggregation | P1 | Single timeline across all monitored projects |
| Filtering and search | P1 | Filter by project, agent, event type, time range |
| Token usage tracking | P2 | Per-dispatch and per-run token consumption from kernel metrics |
| Historical replay | P2 | Scroll back through kernel event history (cursor-based pagination) |

**Activity types (kernel-sourced):**
```
| Event Type          | Source                    | Display |
|---------------------|---------------------------|---------|
| run.created         | ic events                 | New run started with phase chain |
| run.completed       | ic events                 | Run finished (success/failure) |
| phase.advanced      | ic events                 | Phase transition with gate results |
| gate.passed         | ic events                 | Gate evaluation succeeded |
| gate.failed         | ic events                 | Gate evaluation failed (with evidence) |
| dispatch.started    | ic events                 | Agent dispatched (model, role) |
| dispatch.completed  | ic events                 | Agent finished (duration, tokens) |
| artifact.added      | ic events                 | New artifact registered |
```

**Implementation notes:**
- The aggregator's `Activity` struct already has a `Source` field ("kernel", "intermute", "tmux") — kernel events become the primary source.
- tmux-sourced activities are demoted to supplementary (for projects not yet on Intercore).
- The existing `SyntheticID` dedup key prevents duplicate activities when both kernel and tmux sources report the same event.

**Deliverable:** Activity feed shows all agent activity with sub-second latency, richer metadata than tmux scraping, and full cross-project aggregation.

---

### M6: Future

Deferred capabilities that require additional infrastructure or architectural decisions.

#### Multi-Host Monitoring (Deferred)

| Task | Status | Description |
|------|--------|-------------|
| Remote kernel queries | Deferred | Query `ic` on remote hosts via SSH or API |
| Host configuration | Deferred | Define remote hosts in TOML config |
| Unified cross-host view | Deferred | Merge run/dispatch/event data from multiple hosts |
| Latency handling | Deferred | Graceful degradation for slow links |

**Prerequisite:** Intercore needs a remote query API or SSH-tunnel support. Local-only by default; revisit when a concrete multi-host need appears.

#### Control Actions (Deferred)

| Task | Status | Description |
|------|--------|-------------|
| Start run | Deferred | Submit `start-run` intent to OS via Clavain CLI |
| Advance run | Deferred | Submit `advance-run` intent to OS |
| Override gate | Deferred | Submit `override-gate` intent to OS with reason |
| Agent dispatch | Deferred | Request new dispatch via OS intent |

**Prerequisite:** Requires the OS intent submission mechanism. Bigend submits intents to Clavain (the OS), which validates against policy and performs kernel mutations. In v1, intent submission is via direct CLI invocation (e.g., `clavain sprint start`). Future versions may introduce a structured intent API.

**Write-path contract:** Bigend does not call kernel primitives directly for policy-governing operations. All mutations go through the OS layer. This preserves the "apps are swappable" contract — a replacement dashboard would submit the same intents.

---

## Implementation Order

```
M0 (Done) ──> M1 (Status Tool) ──> M2 (Kernel Migration)
                                        |
                                        v
                                    M3 (Signal Broker) ──> M5 (Activity Feed)
                                        |
                                        v
                                    M4 (Rendering Perf)
                                        |
                                        v
                                    M6 (Future: multi-host, control)
```

**Recommended sequence:**
1. **M1 (Status Tool)** — Primary wedge. Validates full Intercore stack with minimal scope.
2. **M2 (Kernel Migration)** — Swap Bigend data sources. Builds on M1's kernel API validation.
3. **M3 (Signal Broker)** — Connect existing broker to kernel event bus. Enables real-time updates.
4. **M4 (Rendering Perf)** — FrankenTUI patterns. Independent of data source; can parallelize with M3.
5. **M5 (Activity Feed)** — Rich kernel-sourced timeline. Depends on M2+M3 for event data.
6. **M6 (Future)** — Multi-host and control actions. Deferred until infrastructure is ready.

---

## Technical Decisions

### Why Go + Bubble Tea?
- Matches the Autarch monorepo stack (all four tools are Go + Bubble Tea)
- Shared `pkg/tui` component library provides consistent UX (ShellLayout, ChatPanel, Tokyo Night)
- Single binary deployment
- Excellent concurrency for signal broker and background aggregation

### Why read-only?
- Bigend is a pure observer — it renders kernel state, it does not control it
- Read-only makes Bigend the lowest-risk Intercore migration
- Control actions require the OS intent submission mechanism (M6, deferred)
- Read-only prevents accidental state corruption

### Why migrate to Intercore?
- Kernel is the single source of truth for runs, dispatches, and events
- A run created via Clavain's `/sprint` should be visible in Bigend's dashboard
- Filesystem scanning and tmux scraping are heuristic and fragile
- Kernel data is structured, queryable, and consistent across all surfaces (CLI, TUI, web)

### Why keep the web interface?
- Secondary access path for non-terminal contexts
- Receives events via WebSocket from the same signal broker
- Lower priority than TUI but preserved for completeness

---

## File Structure

```
apps/autarch/
├── cmd/bigend/
│   └── main.go
├── internal/bigend/
│   ├── aggregator/        # Central state aggregation
│   │   ├── aggregator.go  # Main aggregator loop
│   │   ├── kernel.go      # Intercore data reader
│   │   └── signal_convert.go  # Signal type conversion
│   ├── agentcmd/          # Agent command resolution
│   ├── claude/            # Claude session detection
│   ├── coldwine/          # Coldwine task reader
│   ├── colony/            # Multi-agent colony detection
│   ├── config/            # Configuration
│   ├── daemon/            # Background daemon mode
│   ├── discovery/         # Project scanner
│   ├── mcp/               # MCP Agent Mail reader
│   ├── statedetect/       # Agent state heuristics
│   ├── tmux/              # tmux client and detector
│   ├── tui/               # Bubble Tea TUI
│   │   ├── model.go       # Main TUI model
│   │   ├── render_dashboard.go  # Dashboard rendering
│   │   ├── pane.go        # Pane views
│   │   ├── run_pane.go    # Run detail pane
│   │   ├── terminal.go    # Terminal output viewer
│   │   ├── items.go       # List item types
│   │   └── signals.go     # Signal handling
│   └── web/               # htmx web interface
├── pkg/signals/           # Signal broker (shared with Coldwine)
│   ├── broker.go          # In-process pub/sub
│   ├── signal.go          # Signal types
│   ├── server.go          # WebSocket server
│   └── client.go          # WebSocket client
└── docs/bigend/
    └── roadmap.md          # This file
```

---

## Success Metrics

### M1 Success
- [ ] Status tool displays all active runs from `ic run list` within 2 seconds of launch
- [ ] Event stream updates within 500ms of kernel event emission
- [ ] Dispatch dashboard shows model, role, duration, and status for all active dispatches

### M2 Success
- [ ] All dashboard data sourced from kernel queries (with legacy fallback)
- [ ] No data loss compared to M0 filesystem/tmux sources
- [ ] Kernel query latency under 100ms for typical project counts (1-20)

### M3 Success
- [ ] TUI updates within 200ms of kernel events (push-based, no polling)
- [ ] Signal broker reconnects automatically after kernel restart
- [ ] Backpressure correctly drops old frames for TUI consumers without affecting event ordering

### Overall Success
- [ ] Single developer can monitor 10+ projects with 20+ concurrent agents effectively
- [ ] Context switching between projects takes under 2 seconds
- [ ] Dashboard remains responsive (60fps) with 50+ items via FrankenTUI patterns (M4)

---

## Changelog

| Date | Change |
|------|--------|
| 2026-01-20 | Initial roadmap created (as Vauxhall) |
| 2026-01-20 | M0 completed (foundation) |
| 2026-02-22 | Complete rewrite — aligned to Autarch/Intercore architecture, renamed to Bigend, replaced htmx web milestones with kernel migration milestones |
