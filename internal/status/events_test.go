package status

import (
	"strings"
	"testing"
)

func TestEventsPaneEmpty(t *testing.T) {
	p := NewEventsPane()
	p.SetSize(80, 20)

	view := p.View()
	if !strings.Contains(view, "No events") {
		t.Fatal("expected 'No events' in empty pane")
	}
	if !strings.Contains(view, "EVENTS") {
		t.Fatal("expected 'EVENTS' header")
	}
}

func TestEventsPaneRenderWithData(t *testing.T) {
	p := NewEventsPane()
	p.SetSize(80, 20)

	p.SetEvents([]Event{
		{
			ID:        1,
			RunID:     "r1",
			Source:    "phase",
			Type:      "advance",
			FromState: "plan",
			ToState:   "executing",
			Timestamp: "2026-02-20T09:15:00Z",
		},
		{
			ID:        2,
			RunID:     "r1",
			Source:    "dispatch",
			Type:      "start",
			ToState:   "running",
			Timestamp: "2026-02-20T09:16:00Z",
		},
		{
			ID:        3,
			RunID:     "r1",
			Source:    "gate",
			Type:      "blocked",
			FromState: "executing",
			ToState:   "blocked",
			Reason:    "Tests failing",
			Timestamp: "2026-02-20T09:17:00Z",
		},
	})

	view := p.View()

	// Header should show count
	if !strings.Contains(view, "EVENTS (last 3)") {
		t.Error("expected 'EVENTS (last 3)' header")
	}

	// Event types should appear
	if !strings.Contains(view, "advance") {
		t.Error("expected event type 'advance'")
	}
	if !strings.Contains(view, "start") {
		t.Error("expected event type 'start'")
	}
	if !strings.Contains(view, "blocked") {
		t.Error("expected event type 'blocked'")
	}

	// State transitions — use short names to avoid 24-byte truncation
	if !strings.Contains(view, "plan") {
		t.Error("expected from_state 'plan'")
	}
	if !strings.Contains(view, "executing") {
		t.Error("expected to_state 'executing'")
	}

	// Do NOT check formatted timestamps — t.Local() makes them machine-dependent
}

func TestEventsPaneRenderTruncation(t *testing.T) {
	p := NewEventsPane()
	p.SetSize(80, 3) // header + 2 rows

	events := []Event{
		{ID: 1, Type: "event_alpha", Timestamp: "2026-02-20T09:01:00Z"},
		{ID: 2, Type: "event_bravo", Timestamp: "2026-02-20T09:02:00Z"},
		{ID: 3, Type: "event_charlie", Timestamp: "2026-02-20T09:03:00Z"},
		{ID: 4, Type: "event_delta", Timestamp: "2026-02-20T09:04:00Z"},
		{ID: 5, Type: "event_echo", Timestamp: "2026-02-20T09:05:00Z"},
	}
	p.SetEvents(events)

	view := p.View()

	// With maxRows=2 (height 3 minus header), only last 2 events should show
	if !strings.Contains(view, "event_delta") {
		t.Error("expected event_delta (4th event) in truncated view")
	}
	if !strings.Contains(view, "event_echo") {
		t.Error("expected event_echo (5th event) in truncated view")
	}
	if strings.Contains(view, "event_alpha") {
		t.Error("expected event_alpha (1st event) to be truncated")
	}
}

func TestEventsPaneRenderMalformedTimestamp(t *testing.T) {
	p := NewEventsPane()
	p.SetSize(80, 20)

	badTimestamp := "not-a-real-timestamp"
	p.SetEvents([]Event{
		{ID: 1, Type: "advance", Timestamp: badTimestamp},
	})

	view := p.View()

	// formatEventTime truncates non-RFC3339 timestamps to 8 chars
	if !strings.Contains(view, badTimestamp[:8]) {
		t.Errorf("expected truncated timestamp %q in view", badTimestamp[:8])
	}
	if !strings.Contains(view, "advance") {
		t.Error("expected event type in view")
	}
}
