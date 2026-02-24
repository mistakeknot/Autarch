# Signal Broker Wiring Implementation Plan
**Phase:** executing (as of 2026-02-21T06:04:42Z)

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Goal:** Wire the existing signal broker into the Bigend/TUI runtime so views get push-based event delivery instead of polling SQLite.

**Architecture:** The signal broker (`pkg/signals/broker.go`) becomes an embedded goroutine owned by the aggregator. Events from Intermute WebSocket and aggregator activity are converted to `signals.Signal` and published through the broker. TUI views subscribe for push updates via Bubble Tea messages, with SQLite polling as automatic fallback when no broker is available.

**Tech Stack:** Go, Bubble Tea, nhooyr.io/websocket, SQLite (pkg/events)

**Review amendments:** This plan incorporates findings from 3-agent flux-drive review (architecture, correctness, quality). Key changes from the original plan:
- **Task 0 added**: Fix deadlock in existing `broker.go:Publish()` before any wiring
- **Task 2**: Replaced map-based prefix matching with ordered slice; replaced `inferSeverity` with per-entry severity; removed `signal.raised` passthrough (no concrete use case); renamed function to `eventToSignal`
- **Task 4**: `eventsStore` injected via `New()` parameter (not post-construction setter); `json.Marshal` error handled; store write moved to goroutine; log level raised to `Warn`
- **Task 5**: Added `brokerDone` channel for clean goroutine shutdown; closure captures subscription by value; subscription guard on `Init()`; `Blur()` closes subscription cleanly
- **Task 6**: Same closure capture fix; subscription guard on `Toggle()`
- **Task 7**: Removed Option A (phantom broker) — unified TUI passes `nil` broker; only Bigend standalone path gets wired broker
- **Tests**: Added goroutine-exercising tests, moved integration tests to `aggregator_broker_test.go`, fixed timeout values

---

### Task 0: Fix Deadlock in broker.go Publish

**Files:**
- Modify: `pkg/signals/broker.go:44-65`
- Modify: `pkg/signals/broker_test.go`

**Context:** The existing `Publish()` method has an unconditional blocking send `sub.ch <- sig` (line 62) executed while holding `b.mu`. If the channel refills between the drain and the send (race between evict and subscriber reads), the broker deadlocks — mutex held, all subscribers and future Publish calls stalled. This must be fixed before wiring the broker into any hot path.

**Step 1: Write the failing test**

Add to `pkg/signals/broker_test.go`:

```go
func TestPublishNeverBlocksUnderContention(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe(nil)
	defer sub.Close()

	// Fill the channel completely
	for i := 0; i < 64; i++ {
		b.Publish(testSignal(i))
	}

	// Publish 100 more — must never block even though subscriber isn't draining
	done := make(chan struct{})
	go func() {
		for i := 64; i < 164; i++ {
			b.Publish(testSignal(i))
		}
		close(done)
	}()

	select {
	case <-done:
		// Success — all publishes completed without blocking
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked — deadlock detected")
	}

	// Verify drops were counted
	if b.Dropped.Load() == 0 {
		t.Fatal("expected non-zero drop count")
	}
}
```

**Step 2: Run test to verify it fails (or exhibits deadlock)**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./pkg/signals/ -run TestPublishNeverBlocksUnderContention -v -timeout 10s`
Expected: May deadlock or timeout (the current code has a blocking send under mutex)

**Step 3: Fix the unconditional blocking send**

In `pkg/signals/broker.go`, replace the `Publish()` method (lines 44-65):

```go
// Publish broadcasts a signal to subscribers.
func (b *Broker) Publish(sig Signal) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for sub := range b.subs {
		if len(sub.types) > 0 {
			if !sub.types[sig.Type] {
				continue
			}
		}
		select {
		case sub.ch <- sig:
		default:
			// Channel is full: evict oldest queued signal so newest wins.
			select {
			case <-sub.ch:
				b.Dropped.Add(1)
			default:
			}
			// Second non-blocking attempt after eviction.
			select {
			case sub.ch <- sig:
			default:
				// Truly lost; the subscriber cannot keep up.
				b.Dropped.Add(1)
			}
		}
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./pkg/signals/ -run TestPublish -v -timeout 10s`
Expected: PASS (all publish tests, including the new contention test)

**Step 5: Run full broker test suite**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./pkg/signals/ -v -race`
Expected: PASS with race detector enabled

**Step 6: Commit**

```bash
cd /root/projects/Interverse/hub/autarch
git add pkg/signals/broker.go pkg/signals/broker_test.go
git commit -m "fix(signals): prevent deadlock in Publish — replace blocking send with non-blocking fallback"
```

---

### Task 1: Add Broker Field to Aggregator

**Files:**
- Modify: `internal/bigend/aggregator/aggregator.go:108-166`
- Test: `internal/bigend/aggregator/aggregator_test.go`

**Step 1: Write the failing test**

Create or add to `internal/bigend/aggregator/aggregator_test.go`:

```go
package aggregator

import (
	"testing"

	"github.com/mistakeknot/autarch/internal/bigend/config"
)

func TestNewAggregatorHasBroker(t *testing.T) {
	agg := New(nil, &config.Config{}, nil)
	if agg.Broker() == nil {
		t.Fatal("expected non-nil broker after New()")
	}
}
```

Note: `New()` now takes 3 parameters — the third is `*events.Store` (nil = no dual-write). See Task 4 for details.

**Step 2: Run test to verify it fails**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/bigend/aggregator/ -run TestNewAggregatorHasBroker -v`
Expected: FAIL — `Broker()` method doesn't exist

**Step 3: Write minimal implementation**

In `internal/bigend/aggregator/aggregator.go`, add to the imports:

```go
"github.com/mistakeknot/autarch/pkg/signals"
```

Add fields to `Aggregator` struct (after `wsConnected atomic.Bool` at line ~129):

```go
broker      *signals.Broker
eventsStore *events.Store
```

Modify `New()` signature to accept optional events store:

```go
func New(scanner *discovery.Scanner, cfg *config.Config, store *events.Store) *Aggregator {
```

Add to the `return &Aggregator{...}` in `New()`:

```go
broker:      signals.NewBroker(),
eventsStore: store,
```

Add getter method (after `New()` function):

```go
// Broker returns the embedded signal broker for real-time event delivery.
func (a *Aggregator) Broker() *signals.Broker {
	return a.broker
}
```

Update all existing callers of `New(scanner, cfg)` to `New(scanner, cfg, nil)`. There should be a small number of call sites in `cmd/autarch/main.go` and possibly tests.

**Step 4: Run test to verify it passes**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/bigend/aggregator/ -run TestNewAggregatorHasBroker -v`
Expected: PASS

**Step 5: Build full project to catch call site breakage**

Run: `cd /root/projects/Interverse/hub/autarch && go build ./cmd/...`
Expected: Build succeeds (all `New()` callers updated)

**Step 6: Commit**

```bash
cd /root/projects/Interverse/hub/autarch
git add internal/bigend/aggregator/aggregator.go internal/bigend/aggregator/aggregator_test.go cmd/autarch/main.go
git commit -m "feat(aggregator): add embedded signal broker and events store at construction"
```

---

### Task 2: Event-to-Signal Conversion Function

**Files:**
- Create: `internal/bigend/aggregator/signal_convert.go`
- Test: `internal/bigend/aggregator/signal_convert_test.go`

**Key changes from original plan:**
- Function renamed to `eventToSignal` (takes `aggregator.Event`, not `intermute.Event`)
- Replaced `map[string]SignalType` prefix loop with ordered slice of structs
- Each mapping entry includes severity — eliminates `inferSeverity` string heuristic
- Removed `signal.raised` passthrough (no concrete use case, produces undefined SignalType)
- Exact match only — no prefix matching until a real sub-type requirement exists

**Step 1: Write the failing test**

Create `internal/bigend/aggregator/signal_convert_test.go`:

```go
package aggregator

import (
	"testing"
	"time"

	"github.com/mistakeknot/autarch/pkg/signals"
)

func TestEventToSignal_TaskBlocked(t *testing.T) {
	evt := Event{
		Type:      "task.blocked",
		Project:   "/proj",
		EntityID:  "TASK-001",
		Timestamp: time.Now(),
	}
	sig, ok := eventToSignal(evt)
	if !ok {
		t.Fatal("expected conversion to succeed for task.blocked")
	}
	if sig.Type != signals.SignalTaskBlocked {
		t.Fatalf("expected type %q, got %q", signals.SignalTaskBlocked, sig.Type)
	}
	if sig.Source != "intermute" {
		t.Fatalf("expected source 'intermute', got %q", sig.Source)
	}
	if sig.Severity != signals.SeverityWarning {
		t.Fatalf("expected severity Warning, got %q", sig.Severity)
	}
}

func TestEventToSignal_RunFailed(t *testing.T) {
	evt := Event{
		Type:      "run.failed",
		EntityID:  "RUN-001",
		Timestamp: time.Now(),
	}
	sig, ok := eventToSignal(evt)
	if !ok {
		t.Fatal("expected conversion to succeed for run.failed")
	}
	if sig.Type != signals.SignalExecutionDrift {
		t.Fatalf("expected type %q, got %q", signals.SignalExecutionDrift, sig.Type)
	}
	if sig.Severity != signals.SeverityWarning {
		t.Fatalf("expected severity Warning, got %q", sig.Severity)
	}
}

func TestEventToSignal_RunWaiting(t *testing.T) {
	evt := Event{
		Type:      "run.waiting",
		EntityID:  "RUN-002",
		Timestamp: time.Now(),
	}
	sig, ok := eventToSignal(evt)
	if !ok {
		t.Fatal("expected conversion to succeed for run.waiting")
	}
	if sig.Type != signals.SignalExecutionDrift {
		t.Fatalf("expected type %q, got %q", signals.SignalExecutionDrift, sig.Type)
	}
	if sig.Severity != signals.SeverityInfo {
		t.Fatalf("expected severity Info, got %q", sig.Severity)
	}
}

func TestEventToSignal_SpecRevised(t *testing.T) {
	evt := Event{
		Type:      "spec.revised",
		EntityID:  "SPEC-001",
		Timestamp: time.Now(),
	}
	sig, ok := eventToSignal(evt)
	if !ok {
		t.Fatal("expected conversion to succeed for spec.revised")
	}
	if sig.Type != signals.SignalSpecHealthLow {
		t.Fatalf("expected type %q, got %q", signals.SignalSpecHealthLow, sig.Type)
	}
}

func TestEventToSignal_Unmapped(t *testing.T) {
	evt := Event{
		Type:      "message.sent",
		EntityID:  "MSG-001",
		Timestamp: time.Now(),
	}
	_, ok := eventToSignal(evt)
	if ok {
		t.Fatal("expected conversion to fail for unmapped event type")
	}
}

func TestEventToSignal_ZeroTimestampDefaultsToNow(t *testing.T) {
	evt := Event{
		Type:     "task.blocked",
		EntityID: "TASK-002",
	}
	sig, ok := eventToSignal(evt)
	if !ok {
		t.Fatal("expected conversion to succeed")
	}
	if sig.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt when input timestamp is zero")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/bigend/aggregator/ -run TestEventToSignal -v`
Expected: FAIL — `eventToSignal` undefined

**Step 3: Write minimal implementation**

Create `internal/bigend/aggregator/signal_convert.go`:

```go
package aggregator

import (
	"fmt"
	"time"

	"github.com/mistakeknot/autarch/pkg/signals"
)

// signalMapping defines the conversion table from aggregator event types to signal types.
// Each entry is checked by exact match. Order does not matter (no prefix matching).
var signalMapping = []struct {
	eventType string
	sigType   signals.SignalType
	severity  signals.Severity
}{
	{"task.blocked", signals.SignalTaskBlocked, signals.SeverityWarning},
	{"run.failed", signals.SignalExecutionDrift, signals.SeverityWarning},
	{"run.waiting", signals.SignalExecutionDrift, signals.SeverityInfo},
	{"spec.revised", signals.SignalSpecHealthLow, signals.SeverityInfo},
}

// eventToSignal converts an aggregator Event to a signals.Signal.
// Returns false if the event type has no signal mapping (unmapped events are silently skipped).
func eventToSignal(evt Event) (signals.Signal, bool) {
	for _, m := range signalMapping {
		if evt.Type == m.eventType {
			sig := signals.Signal{
				ID:        evt.EntityID,
				Type:      m.sigType,
				Source:    "intermute",
				Severity:  m.severity,
				Title:     fmt.Sprintf("[%s] %s", evt.Type, evt.EntityID),
				CreatedAt: evt.Timestamp,
			}
			if sig.CreatedAt.IsZero() {
				sig.CreatedAt = time.Now()
			}
			return sig, true
		}
	}
	return signals.Signal{}, false
}
```

**Step 4: Run test to verify it passes**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/bigend/aggregator/ -run TestEventToSignal -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /root/projects/Interverse/hub/autarch
git add internal/bigend/aggregator/signal_convert.go internal/bigend/aggregator/signal_convert_test.go
git commit -m "feat(aggregator): add event-to-signal conversion with explicit severity per mapping"
```

---

### Task 3: Wire Publish into handleIntermuteEvent

**Files:**
- Modify: `internal/bigend/aggregator/aggregator.go:247-265`
- Test: `internal/bigend/aggregator/aggregator_broker_test.go` (new file — integration tests separate from conversion unit tests)

**Step 1: Write the failing test**

Create `internal/bigend/aggregator/aggregator_broker_test.go`:

```go
package aggregator

import (
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/pkg/intermute"
	"github.com/mistakeknot/autarch/pkg/signals"
)

func TestHandleIntermuteEventPublishesToBroker(t *testing.T) {
	agg := New(nil, &config.Config{}, nil)
	sub := agg.Broker().Subscribe(nil)
	defer sub.Close()

	agg.handleIntermuteEvent(intermute.Event{
		Type:      "task.blocked",
		EntityID:  "TASK-002",
		Timestamp: time.Now(),
	})

	select {
	case sig := <-sub.Chan():
		if sig.Type != signals.SignalTaskBlocked {
			t.Fatalf("expected type %q, got %q", signals.SignalTaskBlocked, sig.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for signal from broker")
	}
}

func TestHandleIntermuteEventSkipsUnmapped(t *testing.T) {
	agg := New(nil, &config.Config{}, nil)
	sub := agg.Broker().Subscribe(nil)
	defer sub.Close()

	agg.handleIntermuteEvent(intermute.Event{
		Type:      "message.sent",
		EntityID:  "MSG-001",
		Timestamp: time.Now(),
	})

	// handleIntermuteEvent is synchronous — channel state is final after return
	select {
	case sig := <-sub.Chan():
		t.Fatalf("expected no signal for unmapped event, got %+v", sig)
	default:
		// Correct — no signal in channel immediately after synchronous call
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/bigend/aggregator/ -run TestHandleIntermuteEvent -v`
Expected: FAIL — no signal received (broker.Publish not called yet)

**Step 3: Write minimal implementation**

In `internal/bigend/aggregator/aggregator.go`, modify `handleIntermuteEvent`. Add after the existing `a.dispatchEvent(aggEvt)` line:

```go
	// Publish to signal broker if this event maps to a signal type
	if sig, ok := eventToSignal(aggEvt); ok {
		a.broker.Publish(sig)
	}
```

**Step 4: Run test to verify it passes**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/bigend/aggregator/ -run TestHandleIntermuteEvent -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /root/projects/Interverse/hub/autarch
git add internal/bigend/aggregator/aggregator.go internal/bigend/aggregator/aggregator_broker_test.go
git commit -m "feat(aggregator): publish mapped events to signal broker"
```

---

### Task 4: Dual-Write Signal Events to Events Store

**Files:**
- Modify: `internal/bigend/aggregator/aggregator.go`
- Test: `internal/bigend/aggregator/aggregator_broker_test.go`

**Key changes from original plan:**
- `eventsStore` injected via `New()` parameter (already done in Task 1) — no `SetEventsStore` setter
- `json.Marshal` error checked and logged at `Warn`
- Store write moved to a goroutine to avoid stalling the WebSocket read loop
- Insert failure logged at `Warn` (not `Debug`)

**Step 1: Write the failing test**

Add to `aggregator_broker_test.go`:

```go
import (
	"github.com/mistakeknot/autarch/pkg/events"
)

func TestPublishedSignalWrittenToStore(t *testing.T) {
	store, err := events.OpenStore(t.TempDir() + "/events.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	agg := New(nil, &config.Config{}, store)

	agg.handleIntermuteEvent(intermute.Event{
		Type:      "task.blocked",
		EntityID:  "TASK-003",
		Timestamp: time.Now(),
	})

	// Give the goroutine time to write (store write is async)
	time.Sleep(50 * time.Millisecond)

	// Query the store for signal events
	evs, err := store.Query(events.NewEventFilter().WithEventTypes(events.EventSignalRaised))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("expected 1 signal event in store, got %d", len(evs))
	}
	if evs[0].EntityID != "TASK-003" {
		t.Fatalf("expected entity ID 'TASK-003', got %q", evs[0].EntityID)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/bigend/aggregator/ -run TestPublishedSignalWrittenToStore -v`
Expected: FAIL — no events written to store yet

**Step 3: Write minimal implementation**

In `aggregator.go`, extend the broker publish block in `handleIntermuteEvent`:

```go
	// Publish to signal broker if this event maps to a signal type
	if sig, ok := eventToSignal(aggEvt); ok {
		a.broker.Publish(sig)

		// Dual-write to events store for persistence (async to avoid stalling WS read loop)
		if a.eventsStore != nil {
			go func(s signals.Signal) {
				payload, err := json.Marshal(s)
				if err != nil {
					slog.Warn("failed to marshal signal for persistence",
						"signal_id", s.ID, "signal_type", s.Type, "error", err)
					return
				}
				storeEvt := &events.Event{
					EventType:  events.EventSignalRaised,
					EntityType: events.EntityType("signal"),
					EntityID:   s.ID,
					SourceTool: events.SourceTool(s.Source),
					Payload:    payload,
					CreatedAt:  s.CreatedAt,
				}
				if err := a.eventsStore.Insert(storeEvt); err != nil {
					slog.Warn("failed to persist signal to events store",
						"signal_id", s.ID, "error", err)
				}
			}(sig)
		}
	}
```

Add `"encoding/json"` and `"log/slog"` to imports if not already present.

Check that `events.EntityType("signal")` works. If `events.EntitySignal` exists as a constant, use that instead.

**Step 4: Run test to verify it passes**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/bigend/aggregator/ -run TestPublishedSignalWrittenToStore -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /root/projects/Interverse/hub/autarch
git add internal/bigend/aggregator/aggregator.go internal/bigend/aggregator/aggregator_broker_test.go
git commit -m "feat(aggregator): async dual-write signals to events store"
```

---

### Task 5: TUI SignalsView Broker Subscription

**Files:**
- Modify: `internal/tui/views/signals.go:30-98`
- Test: `internal/tui/views/signals_broker_test.go`

**Key changes from original plan:**
- Added `brokerDone` channel for clean goroutine shutdown on `Blur()`
- Closure captures `sub` and `done` by value at call time (not field access at execution time)
- `Init()` guards against pre-existing subscription before creating new one
- Added goroutine-exercising test

**Step 1: Write the failing test**

Create `internal/tui/views/signals_broker_test.go`:

```go
package views

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/signals"
)

func TestSignalsView_BrokerPush(t *testing.T) {
	broker := signals.NewBroker()
	v := NewSignalsView(nil)
	v.SetBroker(broker)

	cmd := v.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a command")
	}

	want := signals.Signal{
		ID:        "TEST-001",
		Type:      signals.SignalTaskBlocked,
		Source:    "test",
		Severity:  signals.SeverityWarning,
		Title:     "test signal",
		CreatedAt: time.Now(),
	}

	// Test that Update() correctly handles a broker message
	v2, _ := v.Update(brokerSignalMsg{signal: want})
	sv := v2.(*SignalsView)
	if len(sv.signals) != 1 {
		t.Fatalf("expected 1 signal after broker push, got %d", len(sv.signals))
	}
	if sv.signals[0].ID != "TEST-001" {
		t.Fatalf("expected signal ID 'TEST-001', got %q", sv.signals[0].ID)
	}
}

func TestSignalsView_WaitBrokerSignal_Delivers(t *testing.T) {
	broker := signals.NewBroker()
	v := NewSignalsView(nil)
	v.SetBroker(broker)

	// Create subscription as Init() would
	v.brokerDone = make(chan struct{})
	v.brokerSub = broker.Subscribe(nil)

	// Get the wait command
	cmd := v.waitBrokerSignal()

	// Run the command in background (simulates Bubble Tea runtime)
	msgCh := make(chan tea.Msg, 1)
	go func() { msgCh <- cmd() }()

	// Publish a signal
	want := signals.Signal{ID: "DELIVER-001", Type: signals.SignalTaskBlocked}
	broker.Publish(want)

	select {
	case msg := <-msgCh:
		got, ok := msg.(brokerSignalMsg)
		if !ok {
			t.Fatalf("expected brokerSignalMsg, got %T", msg)
		}
		if got.signal.ID != "DELIVER-001" {
			t.Fatalf("unexpected signal ID: %q", got.signal.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: signal not delivered")
	}
}

func TestSignalsView_BlurClosesSubscription(t *testing.T) {
	broker := signals.NewBroker()
	v := NewSignalsView(nil)
	v.SetBroker(broker)

	v.Init()
	if v.brokerSub == nil {
		t.Fatal("expected brokerSub to be set after Init()")
	}

	v.Blur()
	if v.brokerSub != nil {
		t.Fatal("expected brokerSub to be nil after Blur()")
	}
}

func TestSignalsView_DoubleInitNoLeak(t *testing.T) {
	broker := signals.NewBroker()
	v := NewSignalsView(nil)
	v.SetBroker(broker)

	// First Init
	v.Init()
	sub1 := v.brokerSub

	// Blur to clean up
	v.Blur()

	// Second Init — should create new subscription, not leak
	v.Init()
	sub2 := v.brokerSub

	if sub1 == sub2 {
		t.Fatal("expected different subscription after Blur+Init cycle")
	}
}

func TestSignalsView_NilBrokerFallback(t *testing.T) {
	v := NewSignalsView(nil)
	// Don't set broker — should work without panic
	cmd := v.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a command even without broker")
	}
}

// Ensure brokerSignalMsg satisfies tea.Msg (compile-time check)
var _ tea.Msg = brokerSignalMsg{}
```

**Step 2: Run test to verify it fails**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/tui/views/ -run TestSignalsView_Broker -v`
Expected: FAIL — `SetBroker`, `brokerSignalMsg` don't exist

**Step 3: Write minimal implementation**

In `internal/tui/views/signals.go`:

Add fields to `SignalsView` struct (after `intermuteStatus string`):

```go
broker     *signals.Broker
brokerSub  *signals.Subscription
brokerDone chan struct{}
```

Add setter:

```go
// SetBroker configures the signal broker for push-based updates.
// If nil, the view falls back to SQLite polling (existing behavior).
func (v *SignalsView) SetBroker(b *signals.Broker) {
	v.broker = b
}
```

Add message type (near `intermuteEventMsg`):

```go
type brokerSignalMsg struct {
	signal signals.Signal
}
```

Modify `Init()` to include broker subscription with guard:

```go
func (v *SignalsView) Init() tea.Cmd {
	cmds := []tea.Cmd{
		v.loadData(),
		v.connectIntermute(),
	}
	if v.broker != nil && v.brokerSub == nil {
		v.brokerDone = make(chan struct{})
		v.brokerSub = v.broker.Subscribe(nil)
		cmds = append(cmds, v.waitBrokerSignal())
	}
	return tea.Batch(cmds...)
}
```

Add broker wait function — captures subscription by value at call time:

```go
func (v *SignalsView) waitBrokerSignal() tea.Cmd {
	sub := v.brokerSub   // capture at call time, not execution time
	done := v.brokerDone // capture at call time
	if sub == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case sig, ok := <-sub.Chan():
			if !ok {
				return nil
			}
			return brokerSignalMsg{signal: sig}
		case <-done:
			return nil
		}
	}
}
```

Handle the message in `Update()` — add a case in the `switch msg := msg.(type)` block (after `intermuteEventMsg`):

```go
	case brokerSignalMsg:
		// Prepend new signal from broker (most recent first)
		v.signals = append([]signals.Signal{msg.signal}, v.signals...)
		v.selected = clamp(v.selected, 0, v.currentListLen()-1)
		if v.brokerSub != nil {
			return v, v.waitBrokerSignal()
		}
		return v, nil
```

Add/modify `Blur()` for clean subscription shutdown:

```go
func (v *SignalsView) Blur() {
	if v.brokerDone != nil {
		close(v.brokerDone)
		v.brokerDone = nil
	}
	if v.brokerSub != nil {
		v.brokerSub.Close()
		v.brokerSub = nil
	}
}
```

Note: If `Blur()` already exists with other logic, add the broker cleanup at the top. Keep existing logic.

**Step 4: Run test to verify it passes**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/tui/views/ -run TestSignalsView -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /root/projects/Interverse/hub/autarch
git add internal/tui/views/signals.go internal/tui/views/signals_broker_test.go
git commit -m "feat(tui): wire SignalsView to broker with safe subscription lifecycle"
```

---

### Task 6: TUI SignalsOverlay Broker Subscription

**Files:**
- Modify: `internal/tui/signals_overlay.go:17-54`
- Test: `internal/tui/signals_overlay_test.go`

**Key changes from original plan:**
- Same closure capture fix as Task 5
- Added `brokerDone` channel for clean shutdown

**Step 1: Write the failing test**

Create `internal/tui/signals_overlay_test.go`:

```go
package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/signals"
)

func TestSignalsOverlay_BrokerPush(t *testing.T) {
	broker := signals.NewBroker()
	o := NewSignalsOverlay()
	o.SetBroker(broker)

	// Simulate toggle (opens overlay)
	o.Toggle()

	want := signals.Signal{
		ID:        "OV-001",
		Type:      signals.SignalTaskBlocked,
		Source:    "test",
		Title:     "overlay test",
		CreatedAt: time.Now(),
	}

	consumed, _ := o.Update(brokerOverlaySignalMsg{signal: want})
	if !consumed {
		t.Fatal("expected overlay to consume brokerOverlaySignalMsg")
	}
	if len(o.signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(o.signals))
	}
}

func TestSignalsOverlay_WaitBrokerSignal_Delivers(t *testing.T) {
	broker := signals.NewBroker()
	o := NewSignalsOverlay()
	o.SetBroker(broker)
	o.brokerDone = make(chan struct{})
	o.brokerSub = broker.Subscribe(nil)

	cmd := o.waitBrokerOverlaySignal()
	msgCh := make(chan tea.Msg, 1)
	go func() { msgCh <- cmd() }()

	want := signals.Signal{ID: "OV-DELIVER-001", Type: signals.SignalTaskBlocked}
	broker.Publish(want)

	select {
	case msg := <-msgCh:
		got, ok := msg.(brokerOverlaySignalMsg)
		if !ok {
			t.Fatalf("expected brokerOverlaySignalMsg, got %T", msg)
		}
		if got.signal.ID != "OV-DELIVER-001" {
			t.Fatalf("unexpected signal ID: %q", got.signal.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: signal not delivered")
	}
}

func TestSignalsOverlay_ToggleCloseCleanup(t *testing.T) {
	broker := signals.NewBroker()
	o := NewSignalsOverlay()
	o.SetBroker(broker)

	o.Toggle() // open
	if o.brokerSub == nil {
		t.Fatal("expected subscription after opening overlay")
	}

	o.Toggle() // close
	if o.brokerSub != nil {
		t.Fatal("expected subscription to be nil after closing overlay")
	}
}

func TestSignalsOverlay_NilBrokerWorks(t *testing.T) {
	o := NewSignalsOverlay()
	cmd := o.Toggle()
	_ = cmd // should not panic
}
```

**Step 2: Run test to verify it fails**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/tui/ -run TestSignalsOverlay_Broker -v`
Expected: FAIL — `SetBroker`, `brokerOverlaySignalMsg` don't exist

**Step 3: Write minimal implementation**

In `internal/tui/signals_overlay.go`:

Add fields to `SignalsOverlay` struct:

```go
broker     *signals.Broker
brokerSub  *signals.Subscription
brokerDone chan struct{}
```

Add setter:

```go
// SetBroker configures the signal broker for push-based overlay updates.
func (o *SignalsOverlay) SetBroker(b *signals.Broker) {
	o.broker = b
}
```

Add message type:

```go
type brokerOverlaySignalMsg struct {
	signal signals.Signal
}
```

Modify `Toggle()` to subscribe when opening with guard:

```go
func (o *SignalsOverlay) Toggle() tea.Cmd {
	o.visible = !o.visible
	o.selected = 0
	if o.visible {
		o.loaded = false
		cmds := []tea.Cmd{o.loadData()}
		if o.broker != nil && o.brokerSub == nil {
			o.brokerDone = make(chan struct{})
			o.brokerSub = o.broker.Subscribe(nil)
			cmds = append(cmds, o.waitBrokerOverlaySignal())
		}
		return tea.Batch(cmds...)
	}
	// Closing — clean up subscription
	if o.brokerDone != nil {
		close(o.brokerDone)
		o.brokerDone = nil
	}
	if o.brokerSub != nil {
		o.brokerSub.Close()
		o.brokerSub = nil
	}
	return nil
}
```

Add broker wait function — captures by value:

```go
func (o *SignalsOverlay) waitBrokerOverlaySignal() tea.Cmd {
	sub := o.brokerSub   // capture at call time
	done := o.brokerDone // capture at call time
	if sub == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case sig, ok := <-sub.Chan():
			if !ok {
				return nil
			}
			return brokerOverlaySignalMsg{signal: sig}
		case <-done:
			return nil
		}
	}
}
```

Handle in `Update()` — add a case before the `tea.KeyMsg` case:

```go
	case brokerOverlaySignalMsg:
		if o.visible && o.category == 0 {
			o.signals = append([]signals.Signal{msg.signal}, o.signals...)
			o.selected = clampOverlay(o.selected, 0, o.currentListLen()-1)
		}
		if o.brokerSub != nil {
			return true, o.waitBrokerOverlaySignal()
		}
		return true, nil
```

**Step 4: Run test to verify it passes**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/tui/ -run TestSignalsOverlay -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /root/projects/Interverse/hub/autarch
git add internal/tui/signals_overlay.go internal/tui/signals_overlay_test.go
git commit -m "feat(tui): wire SignalsOverlay to broker with safe subscription lifecycle"
```

---

### Task 7: Wire Broker Through App Startup

**Files:**
- Modify: `cmd/autarch/main.go`
- Modify: `internal/tui/unified_app.go`

**Key decision (from review):** The unified TUI path (`tuiCmd`) does NOT own an `Aggregator` — it connects via `autarch.Client` (HTTP). There is no publisher in this path. Creating a standalone broker here would produce a phantom broker with zero signals. **The unified TUI path passes `nil` — existing SQLite polling continues unchanged.** Only the Bigend standalone path (`runBigendTUI`), which owns a live `Aggregator`, gets a wired broker.

**Step 1: Add SetSignalBroker to UnifiedApp**

In `internal/tui/unified_app.go`, add:

```go
// SetSignalBroker configures the embedded signal broker for push-based overlay updates.
// Pass nil to use SQLite polling fallback (default).
func (a *UnifiedApp) SetSignalBroker(b *signals.Broker) {
	a.signalsOverlay.SetBroker(b)
}
```

Add import for `"github.com/mistakeknot/autarch/pkg/signals"` if not present.

**Step 2: Wire broker in Bigend standalone path**

In `cmd/autarch/main.go`, find `runBigendTUI` (around line 358). The aggregator is available. Check if `bigendTui.New()` creates any signals components. If it does, pass `agg.Broker()` to them. If it creates a `UnifiedApp` or similar, call `SetSignalBroker(agg.Broker())`.

For the `bigendCmd` path (around line 304 where `aggregator.New(scanner, cfg)` is called), update to:

```go
// Open events store for signal persistence
store, err := events.OpenStore(eventsDBPath)
if err != nil {
	slog.Warn("failed to open events store for signal persistence", "error", err)
}
agg := aggregator.New(scanner, cfg, store)
```

Check: what is `eventsDBPath`? Look for existing events store usage in `main.go` to find the path pattern. If none exists, use the same pattern as other SQLite stores in the project (likely `~/.autarch/events.db` or similar). If the events store is already opened elsewhere, reuse that instance.

**Step 3: Build and verify**

Run: `cd /root/projects/Interverse/hub/autarch && go build ./cmd/autarch/`
Expected: Build succeeds

Run: `cd /root/projects/Interverse/hub/autarch && go test ./... 2>&1 | tail -20`
Expected: All tests pass

**Step 4: Commit**

```bash
cd /root/projects/Interverse/hub/autarch
git add cmd/autarch/main.go internal/tui/unified_app.go
git commit -m "feat: wire signal broker into Bigend standalone startup path"
```

---

### Task 8: Integration Test — End-to-End Signal Flow

**Files:**
- Create: `internal/bigend/aggregator/signal_integration_test.go`

**Step 1: Write the integration test**

```go
package aggregator

import (
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/pkg/events"
	"github.com/mistakeknot/autarch/pkg/intermute"
	"github.com/mistakeknot/autarch/pkg/signals"
)

func TestEndToEnd_IntermuteEventToBrokerSubscriber(t *testing.T) {
	agg := New(nil, &config.Config{}, nil)

	// Subscribe with type filter
	sub := agg.Broker().Subscribe([]signals.SignalType{signals.SignalTaskBlocked})
	defer sub.Close()

	// Simulate an Intermute event arriving
	agg.handleIntermuteEvent(intermute.Event{
		Type:      "task.blocked",
		EntityID:  "E2E-001",
		Project:   "/test-project",
		Timestamp: time.Now(),
	})

	// Verify signal delivered through broker
	select {
	case sig := <-sub.Chan():
		if sig.Type != signals.SignalTaskBlocked {
			t.Fatalf("wrong type: %q", sig.Type)
		}
		if sig.ID != "E2E-001" {
			t.Fatalf("wrong ID: %q", sig.ID)
		}
		if sig.Source != "intermute" {
			t.Fatalf("wrong source: %q", sig.Source)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for signal")
	}

	// Verify unmapped events don't leak through
	agg.handleIntermuteEvent(intermute.Event{
		Type:      "commit.pushed",
		EntityID:  "COMMIT-001",
		Timestamp: time.Now(),
	})

	select {
	case sig := <-sub.Chan():
		t.Fatalf("unexpected signal for unmapped event: %+v", sig)
	default:
		// Correct — no signal in channel after synchronous call
	}
}

func TestEndToEnd_DualWriteWithBroker(t *testing.T) {
	store, err := events.OpenStore(t.TempDir() + "/events.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	agg := New(nil, &config.Config{}, store)
	sub := agg.Broker().Subscribe(nil)
	defer sub.Close()

	agg.handleIntermuteEvent(intermute.Event{
		Type:      "run.failed",
		EntityID:  "RUN-E2E-001",
		Timestamp: time.Now(),
	})

	// Verify broker delivery
	select {
	case sig := <-sub.Chan():
		if sig.Type != signals.SignalExecutionDrift {
			t.Fatalf("wrong type: %q", sig.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for broker signal")
	}

	// Give the async store write time to complete
	time.Sleep(50 * time.Millisecond)

	// Verify store persistence
	evs, err := store.Query(events.NewEventFilter().WithEventTypes(events.EventSignalRaised))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("expected 1 event in store, got %d", len(evs))
	}
	if evs[0].EntityID != "RUN-E2E-001" {
		t.Fatalf("wrong entity ID in store: %q", evs[0].EntityID)
	}
}
```

**Step 2: Run the integration test**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./internal/bigend/aggregator/ -run TestEndToEnd -v`
Expected: PASS

**Step 3: Run full test suite with race detector**

Run: `cd /root/projects/Interverse/hub/autarch && go test ./... -race 2>&1 | tail -30`
Expected: All tests pass with race detector enabled

**Step 4: Commit**

```bash
cd /root/projects/Interverse/hub/autarch
git add internal/bigend/aggregator/signal_integration_test.go
git commit -m "test: add end-to-end signal broker integration tests"
```
