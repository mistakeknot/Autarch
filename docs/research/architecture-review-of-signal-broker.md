# Architecture Review: Signal Broker Wiring

**Date:** 2026-02-20
**Diff:** /tmp/qg-diff-signal-broker.txt
**Files changed:** 17 files, 771 additions
**Reviewer:** fd-architecture (Flux-drive Architecture & Design Reviewer)

---

## Project Context

Codebase-aware mode. Project documentation reviewed:
- `/root/projects/Interverse/hub/autarch/CLAUDE.md`
- `/root/projects/Interverse/hub/autarch/AGENTS.md`
- `/root/projects/Interverse/hub/autarch/docs/ARCHITECTURE.md`
- `/root/projects/Interverse/hub/autarch/pkg/AGENTS.md`

Key conventions confirmed:
- `internal/` for tool-specific code, `pkg/` for shared packages
- Bubble Tea for all TUIs (value-capture pattern for closures is documented)
- `pkg/signals/` is the cross-tool signal type layer
- Bigend is read-only aggregation; writes only to its own broker/events infrastructure
- `pkg/events/` is the event spine (SQLite at `~/.autarch/events.db`)

---

## Change Summary

The change introduces push-based signal delivery from the aggregator to TUI consumers, replacing the existing SQLite polling fallback for real-time signal updates. The six logical pieces are:

1. **`pkg/signals/broker.go`** — deadlock fix: blocking `sub.ch <- sig` replaced with a two-stage non-blocking send with eviction
2. **`internal/bigend/aggregator/aggregator.go`** — broker and events store embedded; `New()` signature extended; `handleIntermuteEvent` dual-writes to broker (sync) and events store (async goroutine)
3. **`internal/bigend/aggregator/signal_convert.go`** — pure conversion table: 4 intermute event types → signal types
4. **`internal/tui/signals_overlay.go`** — broker subscription on Toggle(), `brokerDone` channel for clean shutdown
5. **`internal/tui/views/signals.go`** — broker subscription on Init(), cleanup on Blur()
6. **`cmd/autarch/main.go`** — events store opened in the bigend standalone path; `aggregator.New()` call updated

---

## 1. Boundaries & Coupling

### Layer mapping

```
cmd/autarch/main.go            — composition root, wiring
cmd/bigend/main.go             — composition root, bigend standalone
internal/bigend/aggregator/    — aggregation + event processing (touches broker, events store)
internal/tui/                  — TUI shell (UnifiedApp, SignalsOverlay)
internal/tui/views/            — individual views (SignalsView)
pkg/signals/                   — shared broker + signal types (NO imports from internal/)
pkg/events/                    — shared event store (NO imports from internal/)
```

The import direction is correct. `pkg/signals` does not import `internal/`. `internal/bigend/aggregator` imports `pkg/signals` and `pkg/events`. `internal/tui` imports `pkg/signals`. The aggregator does not import any TUI packages. Layer boundaries are intact.

### Data flow traced end-to-end

```
Intermute WebSocket event arrives
  → aggregator.handleIntermuteEvent(evt intermute.Event)
    → dispatchEvent(aggEvt)         [existing handlers]
    → eventToSignal(aggEvt)         [signal_convert.go: pure function]
      → if mapped: broker.Publish(sig)     [non-blocking, holds mutex]
        → async goroutine: eventsStore.Append(storeEvt)   [if store != nil]
      → subscribers receive via sub.Chan()
        → TUI: waitBrokerSignal() cmd returns brokerSignalMsg
          → Update() prepends signal to slice
          → returns new waitBrokerSignal() to keep loop alive
```

The flow is clean. Each boundary crossing uses explicit contracts (the `Signal` type, the `Subscription` type, Bubble Tea message types).

### Wiring gap: broker not connected to UnifiedApp in cmd/autarch/main.go (CRITICAL)

`internal/tui/unified_app.go:123` defines:

```go
func (a *UnifiedApp) SetSignalBroker(b *signals.Broker) {
    a.signalsOverlay.SetBroker(b)
}
```

In `cmd/autarch/main.go` (the unified entry point, lines 100-239), the app is created at line 133:

```go
app := tui.NewUnifiedApp(client)
app.SetIntermuteManager(mgr)
```

`SetSignalBroker` is never called. The aggregator is not constructed in the unified autarch path — it only appears in the `bigend` subcommand at line 311. So:

- **`autarch tui`** (the recommended, non-deprecated path): `signalsOverlay.broker == nil`. No push updates. Falls back to polling.
- **`bigend --tui`** (deprecated): aggregator is constructed with a broker, but `runBigendTUI` (line 372) calls `bigendTui.New(agg, ...)` which is the standalone bigend TUI, not `UnifiedApp`. `SetSignalBroker` is not called here either.
- **`SignalsView`** (the full-panel signals tab in the autarch TUI): `SetBroker` is never called from any production code. The view exists in the dashboard factory at main.go:232-235 but only as `views.NewBigendView(c)`, `views.NewGurgehView(c, ...)`, `views.NewColdwineView(c)`, `views.NewPollardView(c)` — there is no `SignalsView` in the dashboard factory at all. SignalsView is the `/sig` overlay's sibling component. Its `SetBroker` is tested but not invoked in production.

**Effect:** The entire push path is implemented, tested, and dead in production. The overlay and view fall back to SQLite polling, which is the pre-change behavior.

### Dependency direction: aggregator owns broker — correct

The broker is embedded in the aggregator (`aggregator.go:86`), and returned via `Broker() *signals.Broker`. TUI components receive the broker through an explicit setter. The aggregator does not hold references to TUI components. This is the correct direction.

### New dependency: aggregator → pkg/events

The aggregator now imports `pkg/events`. This is a new dependency from `internal/bigend/aggregator` to a shared package. It is appropriate: `pkg/events` is explicitly designated as the cross-tool event spine in the architecture documentation, and the dual-write is the stated design goal.

### Nil store handling

When `events.OpenStore("")` fails (main.go:307-310), `evStore` is nil and `agg` receives nil. The guard `if a.eventsStore != nil` at aggregator.go:112 makes the persistence path safely optional. The `slog.Warn` on failure is appropriate — persistence is non-essential to the real-time path.

### cmd/bigend/main.go passes nil store

`cmd/bigend/main.go:81`: `agg := aggregator.New(scanner, cfg, nil)`. The standalone bigend binary skips persistence. This is intentional per the commit structure — only the autarch unified path opens the store. This is a reasonable tradeoff given bigend standalone is deprecated.

---

## 2. Pattern Analysis

### Bubble Tea value-capture pattern — correctly applied

`waitBrokerSignal()` in both `signals.go` and `signals_overlay.go` captures `sub` and `done` by value at call time:

```go
func (v *SignalsView) waitBrokerSignal() tea.Cmd {
    sub := v.brokerSub   // capture at call time, not execution time
    done := v.brokerDone // capture at call time
    ...
    return func() tea.Msg { ... }
}
```

This is the established Bubble Tea pattern for avoiding stale closure bugs, documented in `docs/solutions/` as a known gotcha. Correct.

### brokerDone channel pattern — correct

The `brokerDone chan struct{}` created on subscription and closed in `closeBrokerSub()` provides a clean cancellation signal for the blocking goroutine inside the tea.Cmd. The close order (close done first, then close sub) is important: closing `done` unblocks the goroutine, which returns `nil` to Bubble Tea and stops re-scheduling. Then `sub.Close()` closes the channel and removes from the broker's subscriber map. Both `SignalsOverlay` and `SignalsView` follow this order consistently.

### Naming consistency

All new types follow established codebase conventions:
- Message types: `brokerSignalMsg`, `brokerOverlaySignalMsg` — consistent with `intermuteEventMsg`, `signalsOverlayLoadedMsg`
- Methods: `SetBroker`, `waitBrokerSignal` — consistent with `SetIntermuteManager`, `waitIntermuteEvent`
- Channel field: `brokerDone` — consistent with `brokerSub` naming proximity

### signal_convert.go — well-scoped pure function

The conversion table is package-private, not exported. This keeps the mapping concern internal to the aggregator package. If the mapping ever needs to be driven by configuration, the seam exists at `eventToSignal`. No leaky abstraction.

### Dual-write ordering: broker before store — correct

`broker.Publish(sig)` is called synchronously before the async store goroutine is spawned. This ensures TUI consumers get real-time delivery even if the store write is slow or fails. The ordering is correct.

### Signal.ID = EntityID: semantic collision

`signal_convert.go:29`: `sig.ID = evt.EntityID`. Two signals of different types for the same entity produce signals with the same `ID`. For example, `run.failed` and `run.waiting` for `RUN-001` both produce `Signal{ID: "RUN-001", ...}`. The TUI prepends signals into a slice without deduplication, so this has no immediate display bug. However:

1. The events store record at `aggregator.go:128` uses `EntityID: s.ID`, which is correct for entity-centric querying.
2. If any downstream code indexes, deduplicates, or looks up signals by `ID`, same-entity collisions will silently lose signals.
3. The `Signal.ID` field's semantic contract ("unique signal identifier") is violated.

The fix is small: generate an ID that is unique per signal occurrence, not per entity. For example: `fmt.Sprintf("%s:%s:%d", evt.Type, evt.EntityID, evt.Timestamp.UnixNano())`. The store's `EntityID` field should remain `evt.EntityID`.

### Test coverage is thorough

The diff adds:
- `TestPublishNeverBlocksUnderContention` — directly tests the deadlock fix
- `TestHandleIntermuteEventPublishesToBroker`, `TestHandleIntermuteEventSkipsUnmapped`, `TestPublishedSignalWrittenToStore` — broker integration
- `TestEndToEnd_IntermuteEventToBrokerSubscriber`, `TestEndToEnd_DualWriteWithBroker` — full pipeline
- `TestSignalsView_BrokerPush`, `TestSignalsView_BlurClosesSubscription`, `TestSignalsView_DoubleInitNoLeak` — view lifecycle
- `TestSignalsOverlay_BrokerPush`, `TestSignalsOverlay_ToggleCloseCleanup`, `TestSignalsOverlay_NilBrokerWorks` — overlay lifecycle

All tests are appropriately scoped: unit tests for pure functions, integration tests for pipeline coverage, lifecycle tests for subscription management. The `time.Sleep(50ms)` in async store tests is acceptable for this test pattern.

---

## 3. Simplicity & YAGNI

### Publish eviction logic — correct but drop counter over-counts

The two-stage send-with-eviction in `broker.go:52-68`:

```go
select {
case sub.ch <- sig:
default:
    // evict oldest
    select {
    case <-sub.ch:
        b.Dropped.Add(1)
    default:
    }
    // retry send
    select {
    case sub.ch <- sig:
    default:
        b.Dropped.Add(1)
    }
}
```

The Broker holds `b.mu` throughout Publish, so concurrent Publish calls are fully serialized — no second Publish can interleave. However, the subscriber's consumer goroutine can drain `sub.ch` concurrently with Publish (the channel is not protected by `b.mu`). The race:

1. Publish tries send: channel is full → falls to default
2. Consumer goroutines drain all 64 items between line 57 and line 63
3. Publish drains one item (now from an empty channel: the inner `default` triggers, no drain)
4. Publish retries send: succeeds

In this scenario: `Dropped.Add(1)` is called once (at the drain step) even though no signal was actually dropped. The counter over-counts by 1. This is a metrics accuracy issue, not a data correctness issue. The fix: only increment `Dropped` when the final retry send also fails:

```go
select { case <-sub.ch: default: }   // drain without counting
select {
case sub.ch <- sig:
default:
    b.Dropped.Add(1)    // only a real drop
}
```

### No over-engineering detected

The diff does not add plugin hooks, generic frameworks, or extra interfaces. `eventToSignal` is a simple function, not an interface. The broker setter is a simple field assignment. The `brokerDone` channel is the minimum needed for clean shutdown. No speculative extensibility.

### Nil broker fallback is complete

Both `SignalsOverlay.Toggle()` and `SignalsView.Init()` check `broker != nil` before subscribing. `TestSignalsOverlay_NilBrokerWorks` and `TestSignalsView_NilBrokerFallback` verify the nil path does not panic. The fallback is complete.

### Async goroutine for store write

`aggregator.go:113` spawns one goroutine per published signal for store persistence. With 4 mapped event types and low signal rate, this is acceptable. At high signal rates (bulk reconnect events, for example), this could create goroutine spikes. A buffered work channel with a single persistent store-writer goroutine would bound the concurrency. Appropriate to defer until volume warrants it.

---

## Must-Fix vs Optional

### Must Fix (before this is considered fully functional)

1. **A1** — Call `SetSignalBroker(agg.Broker())` on `UnifiedApp` in `cmd/autarch/main.go`. Without this, the push path is non-functional in the recommended entry point. If the autarch TUI path intentionally has no aggregator, document that the overlay is polling-only in that mode, and ensure the broker construction and wiring exists for the bigend path that does have an aggregator.

### Should Fix

2. **A3** — Use a unique ID for `Signal.ID` (not raw `EntityID`) to avoid future deduplication bugs.

3. **A4** — Fix the `Dropped` counter to only increment on actual loss (not on successful eviction + drain).

### Optional / Deferred

4. **I2** — Bounded store-write goroutine pool when signal volume grows.
5. **I1** — Map for signalMapping when entry count exceeds ~8.

---

## Final Verdict

Verdict: needs-changes

The core machinery is architecturally sound: correct layer separation, correct Bubble Tea patterns, correct shutdown sequencing, good test coverage. The single must-fix is the missing wiring call that connects the broker to `UnifiedApp` in the unified entry point — without it the feature is implemented but not operational. The secondary findings are low-risk and fixable in the same pass.
