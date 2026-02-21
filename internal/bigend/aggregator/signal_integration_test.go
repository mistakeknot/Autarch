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
