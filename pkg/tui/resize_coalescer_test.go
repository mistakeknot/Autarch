package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func makeSize(w, h int) tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: w, Height: h}
}

func TestResizeCoalescerSteadySingleEvent(t *testing.T) {
	c := NewResizeCoalescer()
	now := time.Now()

	// First event always applies immediately (snappy startup).
	action := c.Receive(makeSize(80, 24), now)
	if action != ActionApply {
		t.Fatalf("first event: got %d, want ActionApply", action)
	}
	if c.Pending() {
		t.Fatal("pending should be false after ActionApply")
	}

	// Second event after steady delay gap → coalesce.
	now = now.Add(50 * time.Millisecond)
	action = c.Receive(makeSize(100, 30), now)
	if action != ActionCoalesce {
		t.Fatalf("second event: got %d, want ActionCoalesce", action)
	}
	if !c.Pending() {
		t.Fatal("pending should be true after ActionCoalesce")
	}

	// Tick applies the pending resize.
	now = now.Add(ResizeSteadyDelay)
	msg := c.Tick(now)
	if msg == nil {
		t.Fatal("Tick should return pending resize")
	}
	if msg.Width != 100 || msg.Height != 30 {
		t.Fatalf("Tick returned %dx%d, want 100x30", msg.Width, msg.Height)
	}
	if c.Pending() {
		t.Fatal("pending should be false after Tick")
	}
}

func TestResizeCoalescerBurstDetection(t *testing.T) {
	c := NewResizeCoalescer()
	now := time.Now()

	// First event applies immediately.
	c.Receive(makeSize(80, 24), now)

	// Send events at 20/sec (well above burst threshold of 10/sec).
	for i := 0; i < resizeRateWindowSize; i++ {
		now = now.Add(50 * time.Millisecond) // 20 events/sec
		c.Receive(makeSize(80+i, 24), now)
	}

	// Should now be in burst mode → longer delay.
	if c.Delay() != ResizeBurstDelay {
		t.Fatalf("Delay() = %v, want %v (burst mode)", c.Delay(), ResizeBurstDelay)
	}
}

func TestResizeCoalescerHardDeadline(t *testing.T) {
	c := NewResizeCoalescer()
	now := time.Now()

	// First event applies.
	c.Receive(makeSize(80, 24), now)

	// After hard deadline, the next event must be forced.
	now = now.Add(ResizeHardDeadline)
	action := c.Receive(makeSize(120, 40), now)
	if action != ActionApply {
		t.Fatalf("after hard deadline: got %d, want ActionApply", action)
	}
}

func TestResizeCoalescerHysteresis(t *testing.T) {
	c := NewResizeCoalescer()
	now := time.Now()

	// Bootstrap.
	c.Receive(makeSize(80, 24), now)

	// Enter burst mode with rapid events.
	for i := 0; i < resizeRateWindowSize; i++ {
		now = now.Add(50 * time.Millisecond) // 20/sec
		c.Receive(makeSize(80+i, 24), now)
	}
	if c.Delay() != ResizeBurstDelay {
		t.Fatal("should be in burst mode")
	}

	// Slow down — send enough slow events to flush the fast events out of the
	// sliding window AND exceed cooldownFrames. With rateWindowSize=8, we need
	// at least 8 slow events to fully flush, plus cooldownFrames for hysteresis.
	// The first few slow events still have fast events in the window, so the
	// rate stays above burstExitRate. Once the window is fully slow, cooldown
	// begins counting.
	slowEvents := resizeRateWindowSize + resizeCooldownFrames
	for i := 0; i < slowEvents; i++ {
		now = now.Add(1 * time.Second) // 1/sec, well below exit rate
		c.Receive(makeSize(90+i, 24), now)
	}

	if c.Delay() != ResizeSteadyDelay {
		t.Fatalf("should have exited burst after slow events, Delay() = %v", c.Delay())
	}
}

func TestResizeCoalescerSupersede(t *testing.T) {
	c := NewResizeCoalescer()
	now := time.Now()

	// Bootstrap.
	c.Receive(makeSize(80, 24), now)

	// Queue multiple events before tick — only latest should be applied.
	now = now.Add(50 * time.Millisecond)
	c.Receive(makeSize(90, 30), now)
	now = now.Add(5 * time.Millisecond)
	c.Receive(makeSize(100, 35), now)
	now = now.Add(5 * time.Millisecond)
	c.Receive(makeSize(110, 40), now)

	now = now.Add(ResizeSteadyDelay)
	msg := c.Tick(now)
	if msg == nil {
		t.Fatal("Tick should return pending resize")
	}
	if msg.Width != 110 || msg.Height != 40 {
		t.Fatalf("Tick returned %dx%d, want 110x40 (latest)", msg.Width, msg.Height)
	}
}

func TestResizeCoalescerTickWithNoPending(t *testing.T) {
	c := NewResizeCoalescer()
	now := time.Now()

	// Tick with nothing pending returns nil.
	msg := c.Tick(now)
	if msg != nil {
		t.Fatal("Tick with no pending should return nil")
	}
}
