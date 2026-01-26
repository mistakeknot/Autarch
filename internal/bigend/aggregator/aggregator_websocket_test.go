package aggregator

import (
	"strings"
	"testing"
	"time"
)

func TestSummarizeEvent(t *testing.T) {
	tests := []struct {
		name     string
		evt      Event
		contains string
	}{
		{
			name: "spec created",
			evt: Event{
				Type:     "spec.created",
				EntityID: "SPEC-001",
			},
			contains: "New spec created",
		},
		{
			name: "task updated",
			evt: Event{
				Type:     "task.updated",
				EntityID: "TASK-001",
			},
			contains: "updated",
		},
		{
			name: "message sent",
			evt: Event{
				Type:     "message.sent",
				EntityID: "MSG-001",
			},
			contains: "sent",
		},
		{
			name: "agent registered",
			evt: Event{
				Type:     "agent.registered",
				EntityID: "claude-main",
			},
			contains: "registered",
		},
		{
			name: "reservation released",
			evt: Event{
				Type:     "reservation.released",
				EntityID: "RES-001",
			},
			contains: "released",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := summarizeEvent(tt.evt)
			if !strings.Contains(strings.ToLower(summary), strings.ToLower(tt.contains)) {
				t.Errorf("summarizeEvent() = %q, want to contain %q", summary, tt.contains)
			}
		})
	}
}

func TestAggregator_EventHandlers(t *testing.T) {
	// Create aggregator without Intermute (will be nil)
	agg := New(nil, nil)

	// Track received events
	var receivedEvents []Event

	// Register a handler
	agg.On("spec.created", func(evt Event) {
		receivedEvents = append(receivedEvents, evt)
	})

	// Dispatch an event
	testEvent := Event{
		Type:      "spec.created",
		EntityID:  "SPEC-TEST",
		Project:   "test-project",
		Timestamp: time.Now(),
	}
	agg.dispatchEvent(testEvent)

	if len(receivedEvents) != 1 {
		t.Errorf("expected 1 event, got %d", len(receivedEvents))
	}
	if receivedEvents[0].EntityID != "SPEC-TEST" {
		t.Errorf("expected entity ID SPEC-TEST, got %s", receivedEvents[0].EntityID)
	}
}

func TestAggregator_WildcardHandler(t *testing.T) {
	agg := New(nil, nil)

	var receivedCount int

	// Register a wildcard handler
	agg.On("*", func(evt Event) {
		receivedCount++
	})

	// Dispatch multiple event types
	events := []string{"spec.created", "task.updated", "agent.registered"}
	for _, evtType := range events {
		agg.dispatchEvent(Event{Type: evtType, Timestamp: time.Now()})
	}

	if receivedCount != 3 {
		t.Errorf("expected 3 events, got %d", receivedCount)
	}
}

func TestAggregator_AddActivity(t *testing.T) {
	agg := New(nil, nil)

	// Add some activities
	for i := 0; i < 5; i++ {
		evt := Event{
			Type:      "spec.created",
			EntityID:  "SPEC-" + string(rune('A'+i)),
			Timestamp: time.Now(),
		}
		agg.addActivity(evt)
	}

	state := agg.GetState()
	if len(state.Activities) != 5 {
		t.Errorf("expected 5 activities, got %d", len(state.Activities))
	}

	// Most recent should be first
	if !strings.Contains(state.Activities[0].Summary, "SPEC-E") {
		t.Errorf("expected most recent activity first, got %s", state.Activities[0].Summary)
	}
}

func TestAggregator_ActivityLimit(t *testing.T) {
	agg := New(nil, nil)

	// Add more than 100 activities
	for i := 0; i < 150; i++ {
		evt := Event{
			Type:      "spec.created",
			EntityID:  "SPEC-" + string(rune(i%26+'A')),
			Timestamp: time.Now(),
		}
		agg.addActivity(evt)
	}

	state := agg.GetState()
	if len(state.Activities) != 100 {
		t.Errorf("expected max 100 activities, got %d", len(state.Activities))
	}
}

func TestAggregator_IsWebSocketConnected(t *testing.T) {
	agg := New(nil, nil)

	// Initially not connected
	if agg.IsWebSocketConnected() {
		t.Error("expected not connected initially")
	}
}
