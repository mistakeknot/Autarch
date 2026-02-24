# PRD: Wire Signal Broker into Bigend/TUI Runtime Path

**Bead:** iv-log2

## Problem

Bigend/TUI views poll SQLite for signals and events, adding latency to the rendering loop. The signal broker (`pkg/signals/broker.go`) provides sub-second push delivery but is never instantiated — zero callers of `NewBroker()` exist in the runtime.

## Solution

Wire the existing signal broker into the aggregator lifecycle, publish events through it, and subscribe TUI views to receive push-based updates. No new architecture — just connecting five existing but disconnected pieces.

## Features

### F1: Broker Lifecycle in Aggregator

**What:** Instantiate `signals.NewBroker()` in the aggregator and expose it for consumers.

**Acceptance criteria:**
- [ ] `Aggregator` struct has a `*signals.Broker` field, created in `New()`
- [ ] Broker is accessible via `Aggregator.Broker()` getter
- [ ] Broker is created before Intermute WebSocket connection (startup ordering)
- [ ] `signals.NewServer(nil)` in `serve.go` accepts the aggregator's broker when launched embedded
- [ ] Unit test: broker is non-nil after `New()` and accepts subscriptions

### F2: Publish Path (Aggregator → Broker)

**What:** Convert aggregator events to signals and publish them through the broker. Write signal events to the events store for persistence.

**Acceptance criteria:**
- [ ] `handleIntermuteEvent()` calls `broker.Publish()` for mapped event types
- [ ] Conversion function `intermuteEventToSignal()` maps Intermute event types to `signals.Signal` structs
- [ ] Mapping table covers: spec events → SpecHealthLow, task events → ExecutionDrift, and passthrough for signal-typed events
- [ ] Events store dual-write: signal events are persisted as `EventSignalRaised` in the SQLite store
- [ ] Unmapped event types are silently skipped (no error, no publish)
- [ ] Unit test: conversion function produces correct Signal types for each mapped Intermute event type
- [ ] Unit test: unmapped events produce no publish call

### F3: TUI Subscription (Views Consume from Broker)

**What:** TUI views subscribe to the broker and receive push updates via Bubble Tea messages instead of polling SQLite.

**Acceptance criteria:**
- [ ] `SignalsView` subscribes to broker on Init when broker is available
- [ ] New `tea.Msg` type `brokerSignalMsg` delivers signals from the subscription channel
- [ ] `SignalsView.Update()` handles `brokerSignalMsg` by prepending to the signals list (no full reload)
- [ ] `SignalsOverlay` similarly subscribes when broker is available
- [ ] Fallback: if no broker is provided (nil), views continue using SQLite polling (current behavior unchanged)
- [ ] Subscription is cleaned up on view close/blur
- [ ] Unit test: view receives signals from broker subscription
- [ ] Unit test: nil broker falls back to polling without error

## Non-goals

- New signal types (use existing `pkg/signals` types)
- WebSocket server protocol changes (`ServeWS` already works)
- Events store schema changes (signal event types already defined)
- Intercore event bus integration (separate bead: iv-6abk)
- Performance benchmarking (broker is a rendering optimization — correctness first)

## Dependencies

- `pkg/signals/broker.go` — existing, tested
- `pkg/signals/signal.go` — existing signal types
- `pkg/events/` — existing events store with `EventSignalRaised`/`EventSignalDismissed`
- `pkg/intermute/` — existing Intermute client and event types
- `internal/bigend/aggregator/` — existing aggregator, owns event flow
- `internal/tui/views/signals.go` — existing SignalsView
- `internal/tui/signals_overlay.go` — existing SignalsOverlay

## Open Questions

1. **Exact mapping table**: Which Intermute event types map to which Signal types? Need to enumerate during planning (look at `pkg/intermute` event constants and `pkg/signals` type constants).
