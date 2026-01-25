package events

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/mistakeknot/autarch/pkg/contract"
)

// Writer provides a high-level API for emitting events
type Writer struct {
	store      *Store
	sourceTool SourceTool
	projectPath string
	mu         sync.Mutex
	subs       []*Subscription
}

// NewWriter creates a new event writer
func NewWriter(store *Store, sourceTool SourceTool) *Writer {
	return &Writer{
		store:      store,
		sourceTool: sourceTool,
		subs:       make([]*Subscription, 0),
	}
}

// SetProjectPath sets the default project path for events
func (w *Writer) SetProjectPath(path string) {
	w.projectPath = path
}

// emit writes an event and notifies subscribers
func (w *Writer) emit(eventType EventType, entityType EntityType, entityID string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	event := &Event{
		EventType:   eventType,
		EntityType:  entityType,
		EntityID:    entityID,
		SourceTool:  w.sourceTool,
		Payload:     data,
		ProjectPath: w.projectPath,
		CreatedAt:   time.Now(),
	}

	if err := w.store.Append(event); err != nil {
		return err
	}

	// Notify subscribers
	w.notifySubscribers(event)
	return nil
}

// notifySubscribers sends the event to all matching subscribers
func (w *Writer) notifySubscribers(event *Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, sub := range w.subs {
		if sub.IsClosed() {
			continue
		}
		if matchesFilter(event, sub.Filter) {
			select {
			case sub.Channel <- event:
			default:
				// Channel full, skip
			}
		}
	}
}

// matchesFilter checks if an event matches a subscription filter
func matchesFilter(event *Event, filter *EventFilter) bool {
	if filter == nil {
		return true
	}

	if len(filter.EventTypes) > 0 {
		found := false
		for _, t := range filter.EventTypes {
			if t == event.EventType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.EntityTypes) > 0 {
		found := false
		for _, t := range filter.EntityTypes {
			if t == event.EntityType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.SourceTools) > 0 {
		found := false
		for _, t := range filter.SourceTools {
			if t == event.SourceTool {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// Initiative events

// EmitInitiativeCreated emits an initiative created event
func (w *Writer) EmitInitiativeCreated(initiative *contract.Initiative) error {
	return w.emit(EventInitiativeCreated, EntityInitiative, initiative.ID, initiative)
}

// EmitInitiativeUpdated emits an initiative updated event
func (w *Writer) EmitInitiativeUpdated(initiative *contract.Initiative) error {
	return w.emit(EventInitiativeUpdated, EntityInitiative, initiative.ID, initiative)
}

// EmitInitiativeClosed emits an initiative closed event
func (w *Writer) EmitInitiativeClosed(initiativeID string, reason string) error {
	return w.emit(EventInitiativeClosed, EntityInitiative, initiativeID, map[string]string{
		"id":     initiativeID,
		"reason": reason,
	})
}

// Epic events

// EmitEpicCreated emits an epic created event
func (w *Writer) EmitEpicCreated(epic *contract.Epic) error {
	return w.emit(EventEpicCreated, EntityEpic, epic.ID, epic)
}

// EmitEpicUpdated emits an epic updated event
func (w *Writer) EmitEpicUpdated(epic *contract.Epic) error {
	return w.emit(EventEpicUpdated, EntityEpic, epic.ID, epic)
}

// EmitEpicClosed emits an epic closed event
func (w *Writer) EmitEpicClosed(epicID string, reason string) error {
	return w.emit(EventEpicClosed, EntityEpic, epicID, map[string]string{
		"id":     epicID,
		"reason": reason,
	})
}

// Story events

// EmitStoryCreated emits a story created event
func (w *Writer) EmitStoryCreated(story *contract.Story) error {
	return w.emit(EventStoryCreated, EntityStory, story.ID, story)
}

// EmitStoryUpdated emits a story updated event
func (w *Writer) EmitStoryUpdated(story *contract.Story) error {
	return w.emit(EventStoryUpdated, EntityStory, story.ID, story)
}

// EmitStoryClosed emits a story closed event
func (w *Writer) EmitStoryClosed(storyID string, reason string) error {
	return w.emit(EventStoryClosed, EntityStory, storyID, map[string]string{
		"id":     storyID,
		"reason": reason,
	})
}

// Task events

// EmitTaskCreated emits a task created event
func (w *Writer) EmitTaskCreated(task *contract.Task) error {
	return w.emit(EventTaskCreated, EntityTask, task.ID, task)
}

// EmitTaskAssigned emits a task assigned event
func (w *Writer) EmitTaskAssigned(taskID, assignee string) error {
	return w.emit(EventTaskAssigned, EntityTask, taskID, map[string]string{
		"task_id":  taskID,
		"assignee": assignee,
	})
}

// EmitTaskStarted emits a task started event
func (w *Writer) EmitTaskStarted(taskID string) error {
	return w.emit(EventTaskStarted, EntityTask, taskID, map[string]string{
		"task_id": taskID,
	})
}

// EmitTaskBlocked emits a task blocked event
func (w *Writer) EmitTaskBlocked(taskID, reason string) error {
	return w.emit(EventTaskBlocked, EntityTask, taskID, map[string]string{
		"task_id": taskID,
		"reason":  reason,
	})
}

// EmitTaskCompleted emits a task completed event
func (w *Writer) EmitTaskCompleted(taskID string) error {
	return w.emit(EventTaskCompleted, EntityTask, taskID, map[string]string{
		"task_id": taskID,
	})
}

// Run events

// EmitRunStarted emits a run started event
func (w *Writer) EmitRunStarted(run *contract.Run) error {
	return w.emit(EventRunStarted, EntityRun, run.ID, run)
}

// EmitRunWaiting emits a run waiting event
func (w *Writer) EmitRunWaiting(runID, reason string) error {
	return w.emit(EventRunWaiting, EntityRun, runID, map[string]string{
		"run_id": runID,
		"reason": reason,
	})
}

// EmitRunCompleted emits a run completed event
func (w *Writer) EmitRunCompleted(runID string) error {
	return w.emit(EventRunCompleted, EntityRun, runID, map[string]string{
		"run_id": runID,
	})
}

// EmitRunFailed emits a run failed event
func (w *Writer) EmitRunFailed(runID, reason string) error {
	return w.emit(EventRunFailed, EntityRun, runID, map[string]string{
		"run_id": runID,
		"reason": reason,
	})
}

// Outcome events

// EmitOutcomeRecorded emits an outcome recorded event
func (w *Writer) EmitOutcomeRecorded(outcome *contract.Outcome) error {
	return w.emit(EventOutcomeRecorded, EntityOutcome, outcome.ID, outcome)
}

// Insight events

// InsightLinkedPayload contains data for insight link events
type InsightLinkedPayload struct {
	InsightID    string `json:"insight_id"`
	InitiativeID string `json:"initiative_id,omitempty"`
	FeatureRef   string `json:"feature_ref,omitempty"`
	LinkedBy     string `json:"linked_by,omitempty"`
}

// EmitInsightLinked emits an insight linked event
func (w *Writer) EmitInsightLinked(insightID, initiativeID, featureRef, linkedBy string) error {
	return w.emit(EventInsightLinked, EntityInsight, insightID, InsightLinkedPayload{
		InsightID:    insightID,
		InitiativeID: initiativeID,
		FeatureRef:   featureRef,
		LinkedBy:     linkedBy,
	})
}
