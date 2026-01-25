// Package events provides an event spine for cross-tool communication in Autarch.
// Events are stored in a global SQLite database at ~/.autarch/events.db
package events

import (
	"encoding/json"
	"time"
)

// EventType identifies the type of event
type EventType string

const (
	// Initiative events
	EventInitiativeCreated EventType = "initiative_created"
	EventInitiativeUpdated EventType = "initiative_updated"
	EventInitiativeClosed  EventType = "initiative_closed"

	// Epic events
	EventEpicCreated EventType = "epic_created"
	EventEpicUpdated EventType = "epic_updated"
	EventEpicClosed  EventType = "epic_closed"

	// Story events
	EventStoryCreated EventType = "story_created"
	EventStoryUpdated EventType = "story_updated"
	EventStoryClosed  EventType = "story_closed"

	// Task events
	EventTaskCreated   EventType = "task_created"
	EventTaskAssigned  EventType = "task_assigned"
	EventTaskStarted   EventType = "task_started"
	EventTaskBlocked   EventType = "task_blocked"
	EventTaskCompleted EventType = "task_completed"

	// Run events
	EventRunStarted   EventType = "run_started"
	EventRunWaiting   EventType = "run_waiting"
	EventRunCompleted EventType = "run_completed"
	EventRunFailed    EventType = "run_failed"

	// Outcome events
	EventOutcomeRecorded EventType = "outcome_recorded"

	// Insight events (Pollard -> Gurgeh)
	EventInsightLinked EventType = "insight_linked"
)

// EntityType identifies the type of entity affected
type EntityType string

const (
	EntityInitiative EntityType = "initiative"
	EntityEpic       EntityType = "epic"
	EntityStory      EntityType = "story"
	EntityTask       EntityType = "task"
	EntityRun        EntityType = "run"
	EntityOutcome    EntityType = "outcome"
	EntityInsight    EntityType = "insight"
)

// SourceTool identifies which tool emitted the event
type SourceTool string

const (
	SourceGurgeh  SourceTool = "gurgeh"
	SourceColdwine SourceTool = "coldwine"
	SourcePollard  SourceTool = "pollard"
	SourceBigend   SourceTool = "bigend"
)

// Event represents a single event in the event log
type Event struct {
	ID         int64      `json:"id"`
	EventType  EventType  `json:"event_type"`
	EntityType EntityType `json:"entity_type"`
	EntityID   string     `json:"entity_id"`
	SourceTool SourceTool `json:"source_tool"`
	Payload    []byte     `json:"payload"`     // JSON payload
	ProjectPath string    `json:"project_path,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// PayloadJSON returns the payload as parsed JSON
func (e *Event) PayloadJSON() (map[string]interface{}, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// EventFilter for querying events
type EventFilter struct {
	EventTypes  []EventType
	EntityTypes []EntityType
	EntityIDs   []string
	SourceTools []SourceTool
	Since       *time.Time
	Until       *time.Time
	Limit       int
	Offset      int
}

// NewEventFilter creates a new filter with defaults
func NewEventFilter() *EventFilter {
	return &EventFilter{
		Limit: 100,
	}
}

// WithEventTypes adds event type filters
func (f *EventFilter) WithEventTypes(types ...EventType) *EventFilter {
	f.EventTypes = append(f.EventTypes, types...)
	return f
}

// WithEntityTypes adds entity type filters
func (f *EventFilter) WithEntityTypes(types ...EntityType) *EventFilter {
	f.EntityTypes = append(f.EntityTypes, types...)
	return f
}

// WithEntityIDs adds entity ID filters
func (f *EventFilter) WithEntityIDs(ids ...string) *EventFilter {
	f.EntityIDs = append(f.EntityIDs, ids...)
	return f
}

// WithSourceTools adds source tool filters
func (f *EventFilter) WithSourceTools(tools ...SourceTool) *EventFilter {
	f.SourceTools = append(f.SourceTools, tools...)
	return f
}

// WithSince sets the start time filter
func (f *EventFilter) WithSince(t time.Time) *EventFilter {
	f.Since = &t
	return f
}

// WithUntil sets the end time filter
func (f *EventFilter) WithUntil(t time.Time) *EventFilter {
	f.Until = &t
	return f
}

// WithLimit sets the result limit
func (f *EventFilter) WithLimit(limit int) *EventFilter {
	f.Limit = limit
	return f
}

// WithOffset sets the result offset
func (f *EventFilter) WithOffset(offset int) *EventFilter {
	f.Offset = offset
	return f
}

// Subscription represents an event subscription
type Subscription struct {
	ID      string
	Filter  *EventFilter
	Channel chan *Event
	closed  bool
}

// Close closes the subscription channel
func (s *Subscription) Close() {
	if !s.closed {
		s.closed = true
		close(s.Channel)
	}
}

// IsClosed returns whether the subscription is closed
func (s *Subscription) IsClosed() bool {
	return s.closed
}
