# Quality Review: Signal Broker Wiring

**Files reviewed:** 17 files (771 additions), Go
**Date:** 2026-02-20
**Scope:** `pkg/signals/broker.go`, `internal/bigend/aggregator/aggregator.go`, `internal/bigend/aggregator/signal_convert.go`, `internal/tui/signals_overlay.go`, `internal/tui/views/signals.go`, `cmd/autarch/main.go`, and 7 test files

---

## Context

This change wires a push-based signal delivery path from Intermute WebSocket events through the aggregator to TUI consumers. The architecture is:

```
Intermute WS event
  → aggregator.handleIntermuteEvent
  → eventToSignal (mapping table)
  → broker.Publish
  → [subscriber channels] → SignalsView / SignalsOverlay
  → async dual-write → events.Store (SQLite)
```

The existing path was poll-based (TUI reads SQLite on a timer). This adds a fan-out broker for sub-second push delivery without removing the polling fallback.

---

## Overall Assessment

The design is sound. The broker is simple and correct for its stated purpose (fan-out, drop-newest on overflow, non-blocking). The Bubble Tea subscription lifecycle is handled correctly in both TUI consumers: local variables capture `sub` and `done` at call time before returning the `tea.Cmd` closure, which is the right pattern to prevent stale-pointer reads when the view tears down mid-flight. Test coverage is broad and includes the integration path (event in → store row out). The `closeBrokerSub` helper in `SignalsOverlay` is a clean improvement over inline teardown.

Three issues require attention before merge. The remaining items are improvements, not blockers.

---

## Issue Analysis

### W1 — `Dropped` counter double-increments on single publish failure (`pkg/signals/broker.go`)

**Severity:** WARNING
**Lines:** 53–67

The evict-then-write logic in `Publish` can increment `Dropped` twice for a single call:

```go
select {
case sub.ch <- sig:
default:
    // Channel full: evict oldest
    select {
    case <-sub.ch:
        b.Dropped.Add(1)   // signal #1 lost: the evicted one
    default:
    }
    // Re-attempt write
    select {
    case sub.ch <- sig:
    default:
        b.Dropped.Add(1)   // signal #2 "lost": but this is the current sig failing to land
    }
}
```

The first `Dropped.Add(1)` is correct: a queued signal was discarded. The second fires when the channel is still full after eviction (race with a concurrent reader that drained between the eviction and the write). In this case, `sig` itself is lost — that is a real drop. So two increments can legitimately represent two real discards.

The problem is semantic: operators monitoring `Dropped` will see two increments per publish call in a heavily-contested subscriber, making "how many signals did we lose" ambiguous. The two drops have different root causes (slow subscriber eviction vs. re-insert race) and should ideally be separated. At minimum, add a comment distinguishing them. If a `SlowSubscriberEvictions` counter is desired later, the code structure already has the right shape.

**Recommended fix:** Add a comment at the second increment making the two cases explicit, and document the semantics of `Dropped` in the field's godoc:

```go
// Dropped counts signals discarded due to subscriber backpressure.
// Each overflow event may contribute up to 2 increments: one for eviction
// of the oldest queued signal, and one if the channel is still full after
// eviction (concurrent drain race).
Dropped atomic.Int64
```

---

### W2 — `ServeWS` bypasses `Subscription` abstraction (`pkg/signals/broker.go` line 88)

**Severity:** WARNING

`ServeWS` is defined in `broker.go` (same package) and accesses `sub.sub.ch` directly:

```go
case sig := <-sub.sub.ch:
```

This reaches through two levels of indirection: `Subscription.sub` (the unexported `*subscriber`), then `.ch` (the channel). `Subscription.Chan()` already exists and returns `<-chan Signal`. The direct access creates a hidden invariant: if `Chan()` ever adds filtering, buffering, or instrumentation, `ServeWS` silently bypasses it.

**Fix:**

```go
case sig := <-sub.Chan():
```

One character change, zero behavioral impact today, eliminates the abstraction violation.

---

### W3 — `evStore` nil-used after failed open in `cmd/autarch/main.go` (lines 307–311)

**Severity:** WARNING

```go
evStore, err := events.OpenStore("")
if err != nil {
    slog.Warn("failed to open events store for signal persistence", "error", err)
}
agg := aggregator.New(scanner, cfg, evStore)  // evStore is nil when err != nil
```

The aggregator's internal nil guard (`if a.eventsStore != nil`) prevents a panic, but the intent here is unclear. Is this "best-effort, degrade gracefully"? Or is it "this should not fail in production"? As written, a deploy with a misconfigured store path silently drops all signal persistence with only a `WARN` log and no non-zero exit code.

**If intentional degradation is correct:** add an explicit `slog.Info("signal persistence disabled — running in broker-only mode")` after the nil case so the degraded state is observable.

**If persistence is required:** change to:
```go
evStore, err := events.OpenStore("")
if err != nil {
    return fmt.Errorf("open events store: %w", err)
}
```

---

## Improvement Opportunities

### I1 — Signal ID reuses `EntityID`, no uniqueness guarantee (`internal/bigend/aggregator/signal_convert.go` line 29)

`sig.ID = evt.EntityID`. If a task emits `task.blocked` twice (unblocked and re-blocked), both signals share the ID `TASK-002`. The events store uses `EntityID` as a query field but not as a primary key, so duplicate IDs in the store are not an immediate bug. However, any consumer that deduplicates by signal ID (a future UI, a notification suppressor) will incorrectly discard the second occurrence. A safe approach is to generate the ID as a hash of `(eventType + entityID + timestamp.UnixNano)` or to use `uuid.New()` from `github.com/google/uuid` if already in the dependency graph.

---

### I2 — Broker subscription lifecycle is duplicated (`internal/tui/signals_overlay.go` and `internal/tui/views/signals.go`)

`waitBrokerOverlaySignal` (overlay, lines 674–691) and `waitBrokerSignal` (signals view, lines 444–461) are structurally identical: capture `sub` and `done`, return a `tea.Cmd` that selects on `sub.Chan()` vs `done`, wrap the result in a type-specific message struct. The teardown logic in `closeBrokerSub` (overlay) is equivalent to the inline code in `SignalsView.Blur()`.

The message types differ (`brokerOverlaySignalMsg` vs `brokerSignalMsg`), which prevents direct code reuse via a shared function. Options:

1. A shared `makeBrokerWaitCmd(sub *signals.Subscription, done <-chan struct{}, wrap func(signals.Signal) tea.Msg) tea.Cmd` in `pkg/tui`.
2. A `brokerListener` struct embedding the subscription state, with a method `Wait(wrap func) tea.Cmd` and `Close()`.

Either approach removes the drift risk when the protocol changes (e.g. closed-channel semantics, context integration).

---

### I3 — Async store tests use `time.Sleep` (`internal/bigend/aggregator/aggregator_broker_test.go` line 226, `signal_integration_test.go` line 563)

```go
time.Sleep(50 * time.Millisecond)
```

This is a 50 ms blind wait for the goroutine spawned inside `handleIntermuteEvent` to complete the `store.Append`. On a loaded CI runner or under `-race`, 50 ms may not be sufficient. A polling loop with a 500 ms deadline is more robust:

```go
var evs []*events.Event
deadline := time.Now().Add(500 * time.Millisecond)
for time.Now().Before(deadline) {
    evs, err = store.Query(...)
    if err == nil && len(evs) >= 1 {
        break
    }
    time.Sleep(5 * time.Millisecond)
}
```

Alternatively, the goroutine in `handleIntermuteEvent` could accept an optional `sync.WaitGroup` (nil in production, set in tests) to allow test-driven synchronization without changing the public API.

---

### I4 — `signal_convert_test.go` should be table-driven (lines 373–456)

Five test functions (`TestEventToSignal_TaskBlocked`, `TestEventToSignal_RunFailed`, `TestEventToSignal_RunWaiting`, `TestEventToSignal_SpecRevised`, `TestEventToSignal_Unmapped`) follow the same structure: construct an `Event`, call `eventToSignal`, assert `ok`, assert `.Type`, assert `.Severity`. A table-driven test reduces this from ~100 lines to ~40 and makes adding new mappings to `signalMapping` a one-line table entry in both the production code and the test. This matches the prevailing style in `aggregator_websocket_test.go`.

Example shape:
```go
func TestEventToSignal(t *testing.T) {
    cases := []struct {
        name        string
        eventType   string
        wantOk      bool
        wantType    signals.SignalType
        wantSeverity signals.Severity
    }{
        {"task.blocked", "task.blocked", true, signals.SignalTaskBlocked, signals.SeverityWarning},
        {"run.failed",   "run.failed",   true, signals.SignalExecutionDrift, signals.SeverityWarning},
        // ...
        {"unmapped", "message.sent", false, "", ""},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            ...
        })
    }
}
```

---

## What Is Done Well

- **Deadlock fix in `broker.Publish`** is correct. The non-blocking select after eviction prevents the WS read loop from stalling under slow subscribers. The logic is clear and the new test (`TestPublishNeverBlocksUnderContention`) validates it properly.
- **Bubble Tea lifecycle pattern** is correct in both TUI consumers. Capturing `sub` and `done` as local variables before returning the `tea.Cmd` closure is the right way to handle teardown races in Bubble Tea. The comments in the code explain the intent.
- **`closeBrokerSub` cleanup helper** in `SignalsOverlay` is an improvement over inline teardown. The nil-before-close ordering (close `done` first, then `Close()` the subscription) is correct: it unblocks any in-flight `waitBrokerOverlaySignal` goroutine before the channel is closed.
- **`SetBroker` / optional broker pattern** is clean API design. Both `SignalsView` and `SignalsOverlay` accept a nil broker and fall back to SQLite polling, preserving backward compatibility with `cmd/bigend/main.go` which passes `nil`.
- **`signal_convert.go`** mapping table is idiomatic Go. Using an anonymous struct slice rather than a map preserves insertion order legibility and makes multiple events mapping to the same signal type (e.g. `run.failed` and `run.waiting` both mapping to `SignalExecutionDrift`) visually obvious.
- **Test file organization** follows the project pattern: `aggregator_broker_test.go` for broker integration, `signal_convert_test.go` for the mapping table, `signal_integration_test.go` for end-to-end paths. The separation of concerns is appropriate.
- **`aggregator.New` signature change** (adds `store *events.Store`) is backward compatible via nil. The docstring on `New` explains the nil behavior explicitly.

---

## Verdict

**needs-changes** — W1 (drop counter semantics), W2 (abstraction bypass in `ServeWS`), and W3 (nil store degradation without stated intent) should be resolved. I1–I4 are improvements that can follow in a subsequent commit.
