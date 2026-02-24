# Wire Signal Broker into Bigend/TUI Runtime Path

**Bead:** iv-0v7j
**Date:** 2026-02-20
**Status:** Brainstorm complete

## What We're Building

Connect the existing signal broker (`pkg/signals/broker.go`) into the Bigend/TUI runtime so that latency-sensitive views get push-based event delivery instead of polling SQLite. The broker is already fully built — pub/sub fan-out, WebSocket streaming, backpressure with evict-oldest. It just needs to be instantiated and wired into the data flow.

### The Gap (Five Disconnected Wiring Points)

1. **Broker never instantiated** — no code calls `signals.NewBroker()` during TUI or Bigend startup
2. **No publish source** — aggregator processes Intermute events into Activities but never calls `broker.Publish()`
3. **TUI never subscribes** — views read from SQLite events store (polling), not from broker channels
4. **Events never written to store from broker path** — the events store has signal event types defined but only manual writes exist
5. **Intermute events never converted to signals** — `handleIntermuteEvent()` creates `Activity` structs, not `signals.Signal` structs

### What Already Works

- **Broker** (`pkg/signals/broker.go`): typed pub/sub, 64-item buffered channels, evict-oldest backpressure, WebSocket streaming via `ServeWS()`
- **Signal types** (`pkg/signals/signal.go`): CompetitorShipped, ResearchInvalidation, AssumptionDecayed, SpecHealthLow, ExecutionDrift, VisionDrift, etc.
- **Events store** (`pkg/events/`): SQLite with EventSignalRaised/EventSignalDismissed types
- **TUI overlay** (`internal/tui/signals_overlay.go`): reads from events store, renders signals
- **SignalsView** (`internal/tui/views/signals.go`): full view with filtering, Intermute connection, categories
- **Aggregator** (`internal/bigend/aggregator/`): processes Intermute WebSocket events into Activity feed
- **Signals CLI** (`internal/signals/cli/serve.go`): standalone WebSocket server (calls `signals.NewServer(nil)` — but passes nil broker!)

## Why This Approach

The vision doc (autarch-vision.md:204-215) is explicit: the broker is an **embedded goroutine** within the app process, not a daemon. It's a **rendering optimization** — if removed, the system works identically but TUI updates are slower (polling). This means:

- **No new architecture** — just wiring existing pieces together
- **Graceful degradation** — if broker fails, TUI falls back to SQLite polling (current behavior)
- **No write-path changes** — broker is read-only projection, events store remains source of truth

### Chosen Approach: Full Wiring

Wire all five points in a single sprint:
1. Instantiate broker at TUI/Bigend startup
2. Convert aggregator events → broker.Publish()
3. Subscribe TUI views to broker for push updates
4. Write signal events to store (for persistence/replay)
5. Convert Intermute events → Signal types where applicable

**Alternatives considered:**
- *Aggregator-only*: Proves broker works but no user-visible improvement. More risk from split delivery.
- *WebSocket-only*: Use the existing `signals serve` command externally. Adds a process to manage and contradicts "embedded goroutine" vision.

## Key Decisions

1. **Broker lives in aggregator** — the aggregator already owns the event flow from all sources (tmux, Intermute, kernel). Adding `*signals.Broker` as a field on `Aggregator` is natural.
2. **TUI subscribes via Bubble Tea messages** — a goroutine reads from `subscription.Chan()` and sends `tea.Msg` into the Bubble Tea program. This follows the existing pattern in `waitIntermuteEvent()`.
3. **Dual delivery** — broker.Publish() AND events store write happen in parallel. Broker for real-time, store for persistence. If either fails, the other still works.
4. **Signals CLI passes the broker** — `signals.NewServer(nil)` currently gets nil. When launched embedded, it should receive the aggregator's broker instance.
5. **Fallback is free** — if no broker subscription exists, TUI views already work via SQLite polling. No explicit fallback code needed.

## Open Questions

1. **Event-to-Signal mapping**: Which Intermute event types should become which Signal types? The mapping isn't 1:1 — need to define the conversion table.
2. **Startup ordering**: Should the broker start before or after the Intermute WebSocket connection? (Likely before — broker is a local goroutine, Intermute is network.)
3. **Testing**: The broker has tests (`broker_test.go`). Do we need integration tests for the wiring, or are unit tests on the conversion functions sufficient?

## Scope Boundaries

**In scope:**
- Broker instantiation in aggregator
- Publish calls from aggregator event handlers
- TUI subscription plumbing
- Events store dual-write
- Intermute → Signal type conversion

**Out of scope:**
- New signal types (use existing ones)
- WebSocket server changes (already works, just needs non-nil broker)
- Events store schema changes (signal event types already exist)
- Intercore event bus integration (separate bead: iv-6abk)
