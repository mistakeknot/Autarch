package aggregator

import (
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/pkg/events"
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

	// Give the async store write time to complete
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
