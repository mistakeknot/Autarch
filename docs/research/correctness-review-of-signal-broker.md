# Correctness Review: Signal Broker Wiring

**Reviewer:** Julik (Flux-drive Correctness Reviewer)
**Date:** 2026-02-20
**Diff reviewed:** `/tmp/qg-diff-signal-broker.txt`
**Files analyzed:**
- `/root/projects/Interverse/hub/autarch/pkg/signals/broker.go`
- `/root/projects/Interverse/hub/autarch/internal/bigend/aggregator/aggregator.go`
- `/root/projects/Interverse/hub/autarch/internal/bigend/aggregator/signal_convert.go`
- `/root/projects/Interverse/hub/autarch/internal/tui/views/signals.go`
- `/root/projects/Interverse/hub/autarch/internal/tui/signals_overlay.go`
- Test files in the same packages

**Verdict: needs-changes**

---

## Invariants This Change Must Preserve

Before reviewing each area, I established the invariants that must remain true:

1. **Broker.Publish() must never block** — it is called from the WebSocket read goroutine; blocking stalls the entire event pipeline.
2. **Subscriber channels must not receive sends after `Close()` is called** — sending to a closed channel panics.
3. **Bubble Tea's `Update()` is single-threaded** — all state mutations of `SignalsView` and `SignalsOverlay` happen on one goroutine; goroutines spawned by tea.Cmd closures communicate only via returned `tea.Msg` values.
4. **A subscription must not be created twice without an intervening close** — double-subscribe leaks the first subscription's goroutine.
5. **Events written to the SQLite store must have non-empty EntityID** — the schema declares `entity_id TEXT NOT NULL`; empty-string writes are silently accepted but corrupt downstream queries.
6. **`brokerDone` must be closed exactly once** — closing a closed channel panics.
7. **`Dropped` counter should represent incoming-signal loss** — the metric is the observable for data loss alerting.

---

## Area-by-Area Analysis

### 1. broker.go Publish: Non-blocking select with eviction

**What changed:**
The blocking `sub.ch <- sig` was replaced with a two-step non-blocking pattern:

```go
select {
case sub.ch <- sig:
default:
    // channel full — evict oldest
    select {
    case <-sub.ch:
        b.Dropped.Add(1)
    default:
    }
    // retry after eviction
    select {
    case sub.ch <- sig:
    default:
        b.Dropped.Add(1)
    }
}
```

**Correctness of non-blocking behavior:** Correct. `Publish()` holds `b.mu` during the entire fan-out. No path blocks. The `TestPublishNeverBlocksUnderContention` test validates this under a full channel.

**Issue C-01: `Dropped` semantic change.** The `Dropped` counter now increments during successful evictions. The incoming signal IS delivered; only the old signal was dropped. Before this change, `Dropped` meant "an incoming signal was lost." After this change, it means "an old signal was evicted to make room." These are fundamentally different operational events. The `TestPublishDropCounter` test asserts `Dropped == 3` for 3 overflow publishes, and the test still passes — but only because 3 evictions happen, not because 3 incoming signals are lost. Any monitoring built against `Dropped` will fire on routine backpressure rather than actual data loss. This is a correctness issue for the observability invariant.

**Issue C-02: Panic window — concurrent `sub.Close()` during second send attempt.**

`Subscription.Close()` is:

```go
func (s *Subscription) Close() {
    s.broker.mu.Lock()
    delete(s.broker.subs, s.sub)
    s.broker.mu.Unlock()
    close(s.sub.ch)   // <-- outside the lock
}
```

The `close(s.sub.ch)` happens after the lock is released. This means there is a window between "sub is removed from the live set" and "sub's channel is closed."

Concrete race interleaving:

```
Time  Goroutine A (WebSocket / Publish)   Goroutine B (Bubble Tea / sub.Close)
 T1   acquires b.mu
 T2   iterates subs, finds sub (still in map because B hasn't locked yet)
 T3   channel full — enters evict branch
 T4   evicts one signal from sub.ch
 T5   releases b.mu (after eviction)
 T6                                         acquires b.mu
 T7                                         delete(b.subs, sub)
 T8                                         releases b.mu
 T9                                         close(sub.ch)  <-- channel is now closed
 T10  second send: `case sub.ch <- sig:`   PANIC: send on closed channel
```

This race is real because:
- `Publish()` is called from the intermute WebSocket read goroutine.
- `sub.Close()` is called from Bubble Tea's main loop via `Blur()` or `Toggle()`.
- These are concurrent by design.
- The window (T5–T9) is a few nanoseconds but real under scheduler preemption.

The fix: move `close(s.sub.ch)` inside the lock:

```go
func (s *Subscription) Close() {
    if s == nil || s.broker == nil || s.sub == nil { return }
    s.broker.mu.Lock()
    delete(s.broker.subs, s.sub)
    close(s.sub.ch)
    s.broker.mu.Unlock()
}
```

Since `Publish()` holds `b.mu` during the entire send loop, the lock is not re-entered recursively. If B holds the lock and closes the channel, A cannot be inside the send path for that subscriber simultaneously.

Note: `ServeWS()` also reads from `sub.sub.ch` without going through `Publish()`:
```go
case sig := <-sub.sub.ch:
```
If the channel is closed while `ServeWS` is in this select, the receive will return zero-value + `ok=false`. The `ServeWS` code does not check `ok`, it just writes the zero-value signal to the WebSocket. This is a separate minor bug but is pre-existing, not introduced by this diff.

---

### 2. Async dual-write goroutine: `sig` value capture

```go
if sig, ok := eventToSignal(aggEvt); ok {
    a.broker.Publish(sig)
    if a.eventsStore != nil {
        go func(s signals.Signal) {
            payload, err := json.Marshal(s)
            ...
        }(sig)
    }
}
```

`sig` is passed by value as the goroutine parameter `s`. `signals.Signal` is a struct containing:
- Value fields: `ID string`, `Type SignalType`, `Source string`, `SpecID string`, `AffectedField string`, `Severity Severity`, `Title string`, `Detail string`, `CreatedAt time.Time`, `Dismissed bool`
- One pointer field: `DismissedAt *time.Time`

`eventToSignal()` never sets `DismissedAt` — it remains nil in all four mappings in `signal_convert.go`. The value copy is therefore a complete, independent copy of all signal data. No aliasing. This is safe.

`a.eventsStore` is read without locking. This is safe because `eventsStore` is set exactly once in `New()` and never modified after construction. The Go memory model guarantees that a write before goroutine creation is visible to that goroutine, and here the write happens at construction well before any events are processed.

---

### 3. waitBrokerSignal / waitBrokerOverlaySignal: value capture prevents double-consumer

```go
func (v *SignalsView) waitBrokerSignal() tea.Cmd {
    sub := v.brokerSub   // captured at call time
    done := v.brokerDone // captured at call time
    if sub == nil {
        return nil
    }
    return func() tea.Msg {
        select {
        case sig, ok := <-sub.Chan():
            if !ok { return nil }
            return brokerSignalMsg{signal: sig}
        case <-done:
            return nil
        }
    }
}
```

The local `sub` and `done` variables capture the channel references at the moment `waitBrokerSignal()` is called, not when the returned closure executes. This is the correct pattern for Bubble Tea: the closure runs on a separate goroutine managed by the runtime, potentially after `v.brokerSub` has been replaced or nilled.

The closed-channel case (`ok == false`) returns `nil` tea.Msg. Bubble Tea receives a `nil` msg and the Update switch falls through to the default return. This is benign but produces one spurious no-op update tick (Issue C-03, severity INFO).

The done-channel close correctly signals the goroutine before `sub.ch` is closed (see closeBrokerSub analysis below), so the goroutine always exits via `<-done` rather than receiving a zero-value from a closed `sub.ch`. This prevents a spurious `brokerSignalMsg{signal: Signal{}}` from reaching `Update()`.

There is no double-consumer issue: only one goroutine at a time is blocked in the closure (the Bubble Tea runtime only runs one cmd at a time per registered command, and the chain `Update() -> return v, v.waitBrokerSignal()` re-issues the command only after the previous one has resolved and delivered a message).

---

### 4. SignalsView.Init() and SignalsOverlay.Toggle(): subscription guard

```go
if v.broker != nil && v.brokerSub == nil {
    v.brokerDone = make(chan struct{})
    v.brokerSub = v.broker.Subscribe(nil)
    cmds = append(cmds, v.waitBrokerSignal())
}
```

**Issue C-04: Guard works only if `Blur()` is called between Inits.**

The guard `v.brokerSub == nil` correctly prevents double-subscription when `Blur()` has been called (which sets `brokerSub = nil`). However, if `Init()` is called a second time without a prior `Blur()` (e.g., a parent re-initializes its children on every `WindowSizeMsg`), the second call silently skips subscription setup. The first subscription is retained (correct for leak prevention), but no new `waitBrokerSignal` command is returned. If the first `waitBrokerSignal` command has already delivered its message and the view is in a state where it expects to receive the next command, no new goroutine is started to drain the subscription channel.

In practice this creates a silent signal backlog: signals arrive on `brokerSub.ch`, the channel fills to capacity, and `Publish()` starts evicting signals for this subscriber. No error, no log — just silently stale signal display.

The invariant needed from the caller: `Init()` must always be preceded by `Blur()` if `Init()` was called before. This should be documented.

---

### 5. brokerDone close ordering in closeBrokerSub()

```go
func (o *SignalsOverlay) closeBrokerSub() {
    if o.brokerDone != nil {
        close(o.brokerDone)
        o.brokerDone = nil
    }
    if o.brokerSub != nil {
        o.brokerSub.Close()
        o.brokerSub = nil
    }
}
```

Ordering: done-first, then sub-close. The goroutine in `waitBrokerSignal()` is blocked on:

```go
select {
case sig, ok := <-sub.Chan():
case <-done:
    return nil
}
```

When `done` is closed, the goroutine exits via `return nil` before `sub.ch` is closed. `sub.Close()` is then called with no goroutine reading `sub.ch`. This is correct.

The nil guard (`if o.brokerDone != nil`) prevents double-close of `brokerDone`. Since `closeBrokerSub()` is called only from Bubble Tea's single-threaded Update path (`Blur()`, `Toggle()`, `Close()`), there is no concurrent call risk. A second call to `closeBrokerSub()` (e.g., `Toggle()` then `Close()`) will safely no-op on both guarded blocks.

**This area is implemented correctly.** Confirmed safe.

---

### 6. SignalsView.Init() subscription guard: prevents leak on double-Init

**Clarification on the positive case.** When `Init()` is called the first time with a broker:
1. `brokerSub == nil` → guard passes, subscription created, `waitBrokerSignal` command issued.
2. Bubble Tea runs the command's goroutine; it blocks on the subscription channel.

When `Blur()` is called:
1. `close(brokerDone)` → goroutine unblocks, returns `nil`.
2. `sub.Close()` → removes subscription, closes channel.
3. `brokerSub = nil`.

When `Init()` is called again:
1. `brokerSub == nil` → guard passes again, new subscription created.

This is the correct lifecycle for a view that is focused, blurred, then focused again. The guard correctly prevents the double-subscribe leak in this path. The issue (C-04) is only for callers that invoke Init() without a prior Blur().

---

### 7. Integration test: time.Sleep(50ms) for async store write

```go
// TestPublishedSignalWrittenToStore and TestEndToEnd_DualWriteWithBroker
agg.handleIntermuteEvent(intermute.Event{...})

// Give the async store write time to complete
time.Sleep(50 * time.Millisecond)

evs, err := store.Query(...)
```

**Issue C-07:** The goroutine spawned by the dual-write path runs `json.Marshal()` and then `store.Append()` (a SQLite write). On a loaded CI host with SQLite WAL checkpoint activity, 50ms is not a conservative bound. A single disk-flush stall can exceed this. The test will then fail with `expected 1 signal event in store, got 0` — a false negative that is hard to reproduce locally.

A polling loop with a 500ms deadline and 5ms sleep is straightforward and eliminates the flakiness without meaningfully slowing the test suite.

---

### 8. eventToSignal: empty EntityID data quality

```go
func eventToSignal(evt Event) (signals.Signal, bool) {
    for _, m := range signalMapping {
        if evt.Type == m.eventType {
            sig := signals.Signal{
                ID: evt.EntityID,  // could be empty
                ...
            }
```

**Issue C-08:** Some intermute event types (`spec.revised`, `run.waiting`) may arrive with an empty `EntityID`. The signal gets `ID = ""`, which is then written to the events store as `entity_id = ""`. SQLite's NOT NULL constraint permits empty string. Downstream code that queries for specific entity IDs or uses `EntityID` as a dedup key will silently mishandle these records.

`eventToSignal()` is the right place to reject or handle this: if `evt.EntityID == ""`, either generate a UUID for the signal, or skip persistence (log a warning). The broker delivery can still proceed with an empty ID — only the store write needs the guard.

---

## Summary of Findings by Priority

| Priority | Finding | Action |
|----------|---------|--------|
| Fix before merge | C-02: panic window in Publish when sub.Close() races the second send | Move `close(sub.ch)` inside `b.mu` in `Subscription.Close()` |
| Fix before merge | C-01: Dropped counter semantics broken — measures evictions not incoming loss | Add separate `Evicted` counter; keep `Dropped` for actual incoming loss |
| Fix before merge | C-08: empty EntityID written to store | Add guard in goroutine before `store.Append()` |
| Fix in follow-up | C-07: time.Sleep in test is fragile | Replace with polling loop |
| Document | C-04: double-Init without Blur leaks subscription goroutine capacity | Add invariant comment or assertion |
| No action | C-03: stale goroutine returns nil msg | Benign |
| No action | C-05: ordering confirmed correct | Safe |
| No action | C-06: value capture confirmed safe | Safe |

---

## Recommended Fixes

### Fix C-02 (critical — panic)

In `/root/projects/Interverse/hub/autarch/pkg/signals/broker.go`, `Subscription.Close()`:

```go
// Before (current — has race):
func (s *Subscription) Close() {
    if s == nil || s.broker == nil || s.sub == nil { return }
    s.broker.mu.Lock()
    delete(s.broker.subs, s.sub)
    s.broker.mu.Unlock()
    close(s.sub.ch)  // outside lock — race window
}

// After (correct):
func (s *Subscription) Close() {
    if s == nil || s.broker == nil || s.sub == nil { return }
    s.broker.mu.Lock()
    delete(s.broker.subs, s.sub)
    close(s.sub.ch)  // inside lock — atomic with delete
    s.broker.mu.Unlock()
}
```

Also update `ServeWS()` to handle the closed-channel case:
```go
case sig, ok := <-sub.sub.ch:
    if !ok { return }
```

### Fix C-01 (metric integrity)

In `/root/projects/Interverse/hub/autarch/pkg/signals/broker.go`:

```go
type Broker struct {
    mu      sync.Mutex
    subs    map[*subscriber]struct{}
    Dropped atomic.Int64  // incoming signals lost (channel full after eviction attempt)
    Evicted atomic.Int64  // old signals removed to make room for newer ones
}

// In Publish():
select {
case <-sub.ch:
    b.Evicted.Add(1)  // old signal evicted
default:
}
select {
case sub.ch <- sig:
default:
    b.Dropped.Add(1)  // incoming signal also lost (truly full)
}
```

### Fix C-08 (data quality)

In the goroutine in `aggregator.go` `handleIntermuteEvent`:

```go
go func(s signals.Signal) {
    if s.ID == "" {
        slog.Warn("skipping store write for signal with empty ID",
            "signal_type", s.Type, "source", s.Source)
        return
    }
    payload, err := json.Marshal(s)
    ...
}(sig)
```

### Fix C-07 (test reliability)

Replace `time.Sleep(50 * time.Millisecond)` in both test functions with:

```go
deadline := time.Now().Add(500 * time.Millisecond)
var evs []*events.Event
for time.Now().Before(deadline) {
    evs, err = store.Query(events.NewEventFilter().WithEventTypes(events.EventSignalRaised))
    if err != nil { t.Fatal(err) }
    if len(evs) >= 1 { break }
    time.Sleep(5 * time.Millisecond)
}
```

---

## What Is Done Well

- The value-capture pattern in `waitBrokerSignal()` and `waitBrokerOverlaySignal()` is correct and prevents the closure-over-loop-variable bug that is common in Go.
- The done-before-sub close ordering in `closeBrokerSub()` is correct.
- The `brokerSub == nil` guard in `Init()` and `Toggle()` correctly prevents double-subscribe in the normal focus/blur lifecycle.
- The goroutine's `sig` value parameter correctly prevents aliasing.
- The async goroutine correctly suppresses its error from the caller path — no blocking of the WebSocket read loop.
- The Bubble Tea message types are distinct (`brokerSignalMsg` vs `brokerOverlaySignalMsg`) — no cross-component message confusion.
