# Plan: Resize Coalescing

**Bead:** iv-a0zv
**Date:** 2026-02-23

## Overview

Add a resize coalescer to `pkg/tui/` that detects burst vs steady resize events and coalesces during bursts. Drop-in integration for any Bubble Tea model that handles `tea.WindowSizeMsg`.

## Algorithm

From FrankenTUI, simplified (no BOCPD):

- **Steady mode** (default): apply resize after 16ms delay (~60fps responsiveness)
- **Burst mode**: apply resize after 40ms delay (aggressive coalescing)
- **Hard deadline**: always apply after 100ms regardless of mode
- **Burst detection**: enter burst when event rate >= 10/sec, exit when < 5/sec for 3 consecutive coalesce decisions (hysteresis)

## Tasks

### Task 1: ResizeCoalescer struct

**File:** `pkg/tui/resize_coalescer.go` (new)

```go
type ResizeCoalescer struct {
    pending     *tea.WindowSizeMsg  // latest queued resize (nil = nothing pending)
    regime      regime              // steady or burst
    lastApply   time.Time           // when we last applied a resize
    lastEvent   time.Time           // when we last received a resize event
    eventTimes  []time.Time         // sliding window for rate calculation (last 8 events)
    cooldown    int                 // frames since rate dropped below exit threshold
}

type regime int
const (
    regimeSteady regime = iota
    regimeBurst
)

// CoalesceAction tells the caller what to do
type CoalesceAction int
const (
    ActionCoalesce CoalesceAction = iota  // swallow this resize, wait for timer
    ActionApply                            // apply the pending resize now
)
```

**Methods:**

```go
func NewResizeCoalescer() *ResizeCoalescer

// Receive records a new WindowSizeMsg. Returns the action to take.
// ActionCoalesce → start/restart a timer (caller returns tea.Tick)
// ActionApply → apply immediately (hard deadline exceeded)
func (c *ResizeCoalescer) Receive(msg tea.WindowSizeMsg, now time.Time) CoalesceAction

// Tick is called when the coalesce timer fires. Returns the pending size
// if it should be applied, or nil if another event superseded it.
func (c *ResizeCoalescer) Tick(now time.Time) *tea.WindowSizeMsg

// Delay returns the current coalesce delay based on regime.
func (c *ResizeCoalescer) Delay() time.Duration
```

**Constants:**

```go
const (
    steadyDelay     = 16 * time.Millisecond
    burstDelay      = 40 * time.Millisecond
    hardDeadline    = 100 * time.Millisecond
    burstEnterRate  = 10.0  // events/sec
    burstExitRate   = 5.0   // events/sec
    cooldownFrames  = 3
    rateWindowSize  = 8
)
```

**`Receive` logic:**
1. Record event time in sliding window (cap at `rateWindowSize`)
2. Store `msg` as `pending`
3. Calculate event rate from window
4. If rate >= `burstEnterRate` → switch to burst, reset cooldown
5. If burst and rate < `burstExitRate` → increment cooldown; if cooldown >= `cooldownFrames` → switch to steady
6. If `now - lastApply >= hardDeadline` → return `ActionApply` (force)
7. Otherwise → return `ActionCoalesce`

**`Tick` logic:**
1. If `pending == nil` → return nil
2. Return pending, set `lastApply = now`, clear pending

**Test file:** `pkg/tui/resize_coalescer_test.go`
- TestResizeCoalescerSteadySingleEvent: one event → ActionCoalesce, tick → applies
- TestResizeCoalescerBurstDetection: rapid events → enters burst, longer delay
- TestResizeCoalescerHardDeadline: force apply after 100ms even in burst
- TestResizeCoalescerHysteresis: burst → slow down → stays burst for cooldown frames → exits
- TestResizeCoalescerSupersede: multiple events before tick → only latest applied

**Depends on:** nothing
**Risk:** low — purely additive, no existing code touched

---

### Task 2: Wire into UnifiedApp

**File:** `internal/tui/unified_app.go` (edit)

The coalescer wraps the existing `WindowSizeMsg` handler.

**Changes to `UnifiedApp` struct:**
```go
resizeCoalescer *pkgtui.ResizeCoalescer
```

**New message type:**
```go
type resizeTickMsg struct{}
```

**`Update` changes for `tea.WindowSizeMsg`:**
```go
case tea.WindowSizeMsg:
    action := a.resizeCoalescer.Receive(msg, time.Now())
    if action == pkgtui.ActionApply {
        return a.applyResize(msg)
    }
    // Coalesce — start timer
    return a, tea.Tick(a.resizeCoalescer.Delay(), func(time.Time) tea.Msg {
        return resizeTickMsg{}
    })
```

**New handler for `resizeTickMsg`:**
```go
case resizeTickMsg:
    if pending := a.resizeCoalescer.Tick(time.Now()); pending != nil {
        return a.applyResize(*pending)
    }
    return a, nil
```

**Extract existing resize logic to `applyResize`:**
```go
func (a *UnifiedApp) applyResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
    // existing WindowSizeMsg body, unchanged
}
```

**Init change:**
```go
resizeCoalescer: pkgtui.NewResizeCoalescer(),
```

**Test:** No new test file needed — existing layout tests cover resize. The coalescer's logic is tested in Task 1.

**Depends on:** Task 1
**Risk:** low — extract method refactor + add one field + add one message type

---

### Task 3: Wire into Bigend Model

**File:** `internal/bigend/tui/model.go` (edit)

Same pattern as Task 2 but for the Bigend-specific model.

**Changes to `Model` struct:**
```go
resizeCoalescer *pkgtui.ResizeCoalescer
```

**New message type (local to package):**
```go
type resizeTickMsg struct{}
```

**`Update` changes:** Same pattern — `WindowSizeMsg` → `Receive()` → coalesce or apply. Extract existing body to `applyResize` method.

**`Init` change:** Add `resizeCoalescer: pkgtui.NewResizeCoalescer()` to `NewModel()`.

**Depends on:** Task 1
**Risk:** low — same pattern as Task 2

## File Summary

| File | Action |
|------|--------|
| `pkg/tui/resize_coalescer.go` | New |
| `pkg/tui/resize_coalescer_test.go` | New |
| `internal/tui/unified_app.go` | Edit (extract method, add coalescer) |
| `internal/bigend/tui/model.go` | Edit (extract method, add coalescer) |

## Verification

```bash
cd apps/autarch && go test -race ./pkg/tui/... ./internal/tui/... ./internal/bigend/tui/...
```
