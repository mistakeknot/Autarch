package events

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Reader provides a high-level API for reading and subscribing to events
type Reader struct {
	store  *Store
	mu     sync.Mutex
	subs   map[string]*Subscription
	nextID int
}

// NewReader creates a new event reader
func NewReader(store *Store) *Reader {
	return &Reader{
		store: store,
		subs:  make(map[string]*Subscription),
	}
}

// Query retrieves events matching the filter
func (r *Reader) Query(filter *EventFilter) ([]*Event, error) {
	return r.store.Query(filter)
}

// GetByID retrieves a single event by ID
func (r *Reader) GetByID(id int64) (*Event, error) {
	return r.store.GetByID(id)
}

// LastID returns the highest event ID
func (r *Reader) LastID() (int64, error) {
	return r.store.LastID()
}

// Count returns the total number of events
func (r *Reader) Count() (int64, error) {
	return r.store.Count()
}

// Replay replays events since a given ID
func (r *Reader) Replay(sinceID int64, filter *EventFilter, handler func(*Event) error) error {
	return r.store.Replay(sinceID, filter, handler)
}

// ReplayAll replays all events matching the filter
func (r *Reader) ReplayAll(filter *EventFilter, handler func(*Event) error) error {
	return r.Replay(0, filter, handler)
}

// Subscribe creates a subscription for real-time events
func (r *Reader) Subscribe(filter *EventFilter) *Subscription {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	id := fmt.Sprintf("sub-%d", r.nextID)
	sub := &Subscription{
		ID:      id,
		Filter:  filter,
		Channel: make(chan *Event, 100),
	}
	r.subs[id] = sub
	return sub
}

// Unsubscribe removes a subscription
func (r *Reader) Unsubscribe(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sub, ok := r.subs[id]; ok {
		sub.Close()
		delete(r.subs, id)
	}
}

// Watch starts a polling loop that delivers events to a channel
// This is useful for tools that don't have direct writer access
func (r *Reader) Watch(ctx context.Context, filter *EventFilter, interval time.Duration) (<-chan *Event, error) {
	lastID, err := r.LastID()
	if err != nil {
		return nil, err
	}

	ch := make(chan *Event, 100)

	go func() {
		defer close(ch)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				events, err := r.store.Query(&EventFilter{
					EventTypes:  filter.EventTypes,
					EntityTypes: filter.EntityTypes,
					EntityIDs:   filter.EntityIDs,
					SourceTools: filter.SourceTools,
				})
				if err != nil {
					continue
				}

				for _, e := range events {
					if e.ID > lastID {
						select {
						case ch <- e:
							lastID = e.ID
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}
	}()

	return ch, nil
}

// GetRecentByEntity retrieves recent events for a specific entity
func (r *Reader) GetRecentByEntity(entityType EntityType, entityID string, limit int) ([]*Event, error) {
	return r.store.Query(NewEventFilter().
		WithEntityTypes(entityType).
		WithEntityIDs(entityID).
		WithLimit(limit))
}

// GetRecentBySource retrieves recent events from a specific source tool
func (r *Reader) GetRecentBySource(sourceTool SourceTool, limit int) ([]*Event, error) {
	return r.store.Query(NewEventFilter().
		WithSourceTools(sourceTool).
		WithLimit(limit))
}

// GetEntityHistory retrieves the full event history for an entity
func (r *Reader) GetEntityHistory(entityType EntityType, entityID string) ([]*Event, error) {
	return r.store.Query(NewEventFilter().
		WithEntityTypes(entityType).
		WithEntityIDs(entityID).
		WithLimit(1000))
}

// GetActivitySince retrieves all events since a given time
func (r *Reader) GetActivitySince(since time.Time, limit int) ([]*Event, error) {
	return r.store.Query(NewEventFilter().
		WithSince(since).
		WithLimit(limit))
}

// ProjectState provides a way to project current state from events
type ProjectState struct {
	Initiatives map[string]*InitiativeState
	Epics       map[string]*EpicState
	Stories     map[string]*StoryState
	Tasks       map[string]*TaskState
	Runs        map[string]*RunState
}

// InitiativeState holds the current state of an initiative
type InitiativeState struct {
	ID          string
	Title       string
	Status      string
	LastEventID int64
	LastUpdated time.Time
}

// EpicState holds the current state of an epic
type EpicState struct {
	ID           string
	InitiativeID string
	FeatureRef   string
	Title        string
	Status       string
	LastEventID  int64
	LastUpdated  time.Time
}

// StoryState holds the current state of a story
type StoryState struct {
	ID          string
	EpicID      string
	Title       string
	Status      string
	Assignee    string
	LastEventID int64
	LastUpdated time.Time
}

// TaskState holds the current state of a task
type TaskState struct {
	ID          string
	StoryID     string
	Title       string
	Status      string
	Assignee    string
	LastEventID int64
	LastUpdated time.Time
}

// RunState holds the current state of a run
type RunState struct {
	ID           string
	TaskID       string
	AgentName    string
	AgentProgram string
	State        string
	LastEventID  int64
	LastUpdated  time.Time
}

// NewProjectState creates an empty project state
func NewProjectState() *ProjectState {
	return &ProjectState{
		Initiatives: make(map[string]*InitiativeState),
		Epics:       make(map[string]*EpicState),
		Stories:     make(map[string]*StoryState),
		Tasks:       make(map[string]*TaskState),
		Runs:        make(map[string]*RunState),
	}
}

// BuildState builds current state by replaying events
func (r *Reader) BuildState(filter *EventFilter) (*ProjectState, error) {
	state := NewProjectState()

	err := r.ReplayAll(filter, func(e *Event) error {
		state.applyEvent(e)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return state, nil
}

// applyEvent applies an event to update state
func (s *ProjectState) applyEvent(e *Event) {
	switch e.EntityType {
	case EntityInitiative:
		s.applyInitiativeEvent(e)
	case EntityEpic:
		s.applyEpicEvent(e)
	case EntityStory:
		s.applyStoryEvent(e)
	case EntityTask:
		s.applyTaskEvent(e)
	case EntityRun:
		s.applyRunEvent(e)
	}
}

func (s *ProjectState) applyInitiativeEvent(e *Event) {
	payload, _ := e.PayloadJSON()
	id := e.EntityID

	switch e.EventType {
	case EventInitiativeCreated, EventInitiativeUpdated:
		state, ok := s.Initiatives[id]
		if !ok {
			state = &InitiativeState{ID: id}
			s.Initiatives[id] = state
		}
		if title, ok := payload["title"].(string); ok {
			state.Title = title
		}
		if status, ok := payload["status"].(string); ok {
			state.Status = status
		}
		state.LastEventID = e.ID
		state.LastUpdated = e.CreatedAt

	case EventInitiativeClosed:
		if state, ok := s.Initiatives[id]; ok {
			state.Status = "closed"
			state.LastEventID = e.ID
			state.LastUpdated = e.CreatedAt
		}
	}
}

func (s *ProjectState) applyEpicEvent(e *Event) {
	payload, _ := e.PayloadJSON()
	id := e.EntityID

	switch e.EventType {
	case EventEpicCreated, EventEpicUpdated:
		state, ok := s.Epics[id]
		if !ok {
			state = &EpicState{ID: id}
			s.Epics[id] = state
		}
		if title, ok := payload["title"].(string); ok {
			state.Title = title
		}
		if status, ok := payload["status"].(string); ok {
			state.Status = status
		}
		if initID, ok := payload["initiative_id"].(string); ok {
			state.InitiativeID = initID
		}
		if ref, ok := payload["feature_ref"].(string); ok {
			state.FeatureRef = ref
		}
		state.LastEventID = e.ID
		state.LastUpdated = e.CreatedAt

	case EventEpicClosed:
		if state, ok := s.Epics[id]; ok {
			state.Status = "closed"
			state.LastEventID = e.ID
			state.LastUpdated = e.CreatedAt
		}
	}
}

func (s *ProjectState) applyStoryEvent(e *Event) {
	payload, _ := e.PayloadJSON()
	id := e.EntityID

	switch e.EventType {
	case EventStoryCreated, EventStoryUpdated:
		state, ok := s.Stories[id]
		if !ok {
			state = &StoryState{ID: id}
			s.Stories[id] = state
		}
		if title, ok := payload["title"].(string); ok {
			state.Title = title
		}
		if status, ok := payload["status"].(string); ok {
			state.Status = status
		}
		if epicID, ok := payload["epic_id"].(string); ok {
			state.EpicID = epicID
		}
		if assignee, ok := payload["assignee"].(string); ok {
			state.Assignee = assignee
		}
		state.LastEventID = e.ID
		state.LastUpdated = e.CreatedAt

	case EventStoryClosed:
		if state, ok := s.Stories[id]; ok {
			state.Status = "closed"
			state.LastEventID = e.ID
			state.LastUpdated = e.CreatedAt
		}
	}
}

func (s *ProjectState) applyTaskEvent(e *Event) {
	payload, _ := e.PayloadJSON()
	id := e.EntityID

	state, ok := s.Tasks[id]
	if !ok {
		state = &TaskState{ID: id}
		s.Tasks[id] = state
	}

	switch e.EventType {
	case EventTaskCreated:
		if title, ok := payload["title"].(string); ok {
			state.Title = title
		}
		if storyID, ok := payload["story_id"].(string); ok {
			state.StoryID = storyID
		}
		state.Status = "todo"

	case EventTaskAssigned:
		if assignee, ok := payload["assignee"].(string); ok {
			state.Assignee = assignee
		}

	case EventTaskStarted:
		state.Status = "in_progress"

	case EventTaskBlocked:
		state.Status = "blocked"

	case EventTaskCompleted:
		state.Status = "done"
	}

	state.LastEventID = e.ID
	state.LastUpdated = e.CreatedAt
}

func (s *ProjectState) applyRunEvent(e *Event) {
	payload, _ := e.PayloadJSON()
	id := e.EntityID

	state, ok := s.Runs[id]
	if !ok {
		state = &RunState{ID: id}
		s.Runs[id] = state
	}

	switch e.EventType {
	case EventRunStarted:
		if taskID, ok := payload["task_id"].(string); ok {
			state.TaskID = taskID
		}
		if name, ok := payload["agent_name"].(string); ok {
			state.AgentName = name
		}
		if prog, ok := payload["agent_program"].(string); ok {
			state.AgentProgram = prog
		}
		state.State = "working"

	case EventRunWaiting:
		state.State = "waiting"

	case EventRunCompleted:
		state.State = "done"

	case EventRunFailed:
		state.State = "failed"
	}

	state.LastEventID = e.ID
	state.LastUpdated = e.CreatedAt
}
