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
