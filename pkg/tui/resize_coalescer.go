package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Resize coalescing thresholds, ported from FrankenTUI.
const (
	ResizeSteadyDelay    = 16 * time.Millisecond  // ~60fps responsiveness in steady state
	ResizeBurstDelay     = 40 * time.Millisecond   // aggressive coalescing during bursts
	ResizeHardDeadline   = 100 * time.Millisecond  // max latency guarantee
	resizeBurstEnterRate = 10.0                     // events/sec to enter burst mode
	resizeBurstExitRate  = 5.0                      // events/sec to exit burst mode
	resizeCooldownFrames = 3                        // consecutive low-rate ticks before exiting burst
	resizeRateWindowSize = 8                        // sliding window for rate calculation
)

// CoalesceAction tells the caller what to do with a resize event.
type CoalesceAction int

const (
	// ActionCoalesce means the resize was queued. The caller should start a
	// timer with Delay() and call Tick() when it fires.
	ActionCoalesce CoalesceAction = iota
	// ActionApply means the resize should be applied immediately (hard
	// deadline exceeded).
	ActionApply
)

type resizeRegime int

const (
	regimeSteady resizeRegime = iota
	regimeBurst
)

// ResizeCoalescer detects burst vs steady resize events and coalesces during
// bursts to reduce redundant layout recalculations. It is a pure state machine
// with no goroutines — the caller drives it via Receive() and Tick().
type ResizeCoalescer struct {
	pending    *tea.WindowSizeMsg
	regime     resizeRegime
	lastApply  time.Time
	eventTimes []time.Time
	cooldown   int
}

// NewResizeCoalescer creates a coalescer in steady mode.
func NewResizeCoalescer() *ResizeCoalescer {
	return &ResizeCoalescer{
		regime:     regimeSteady,
		eventTimes: make([]time.Time, 0, resizeRateWindowSize),
	}
}

// Receive records a new WindowSizeMsg. Returns ActionApply if the hard deadline
// has been exceeded (caller should apply immediately), or ActionCoalesce if the
// caller should start a timer and wait.
func (c *ResizeCoalescer) Receive(msg tea.WindowSizeMsg, now time.Time) CoalesceAction {
	c.pending = &msg
	c.recordEvent(now)
	c.updateRegime()

	// Hard deadline: if we haven't applied in too long, force it.
	if !c.lastApply.IsZero() && now.Sub(c.lastApply) >= ResizeHardDeadline {
		c.apply(now)
		return ActionApply
	}
	// First event ever — also apply immediately for snappy startup.
	if c.lastApply.IsZero() {
		c.apply(now)
		return ActionApply
	}

	return ActionCoalesce
}

// Tick is called when the coalesce timer fires. Returns the pending resize if
// it should be applied, or nil if nothing is pending (another Receive superseded
// or the pending was already consumed).
func (c *ResizeCoalescer) Tick(now time.Time) *tea.WindowSizeMsg {
	if c.pending == nil {
		return nil
	}
	msg := *c.pending
	c.apply(now)
	return &msg
}

// Delay returns the current coalesce delay based on the detected regime.
func (c *ResizeCoalescer) Delay() time.Duration {
	if c.regime == regimeBurst {
		return ResizeBurstDelay
	}
	return ResizeSteadyDelay
}

// Pending returns true if there is an unapplied resize queued.
func (c *ResizeCoalescer) Pending() bool {
	return c.pending != nil
}

func (c *ResizeCoalescer) apply(now time.Time) {
	c.pending = nil
	c.lastApply = now
}

func (c *ResizeCoalescer) recordEvent(now time.Time) {
	c.eventTimes = append(c.eventTimes, now)
	if len(c.eventTimes) > resizeRateWindowSize {
		c.eventTimes = c.eventTimes[len(c.eventTimes)-resizeRateWindowSize:]
	}
}

// eventRate returns the event rate in events/sec over the sliding window.
func (c *ResizeCoalescer) eventRate() float64 {
	n := len(c.eventTimes)
	if n < 2 {
		return 0
	}
	span := c.eventTimes[n-1].Sub(c.eventTimes[0])
	if span <= 0 {
		return 0
	}
	return float64(n-1) / span.Seconds()
}

func (c *ResizeCoalescer) updateRegime() {
	rate := c.eventRate()
	switch c.regime {
	case regimeSteady:
		if rate >= resizeBurstEnterRate {
			c.regime = regimeBurst
			c.cooldown = 0
		}
	case regimeBurst:
		if rate < resizeBurstExitRate {
			c.cooldown++
			if c.cooldown >= resizeCooldownFrames {
				c.regime = regimeSteady
				c.cooldown = 0
			}
		} else {
			c.cooldown = 0
		}
	}
}
