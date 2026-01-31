package events

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/mistakeknot/autarch/pkg/contract"
)

// Writer provides a high-level API for emitting events
type Writer struct {
	store       *Store
	sourceTool  SourceTool
	projectPath string
	mu          sync.Mutex
	subs        []*Subscription
	bridge      *IntermuteBridge
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

// AttachBridge attaches an IntermuteBridge to forward events to Intermute.
// When attached, events are forwarded after being stored locally.
// Bridge errors are logged but do not fail local event emission (graceful degradation).
func (w *Writer) AttachBridge(bridge *IntermuteBridge) {
	w.bridge = bridge
}

// forwardToBridge sends an event to the attached Intermute bridge if present.
// This is non-blocking and errors are logged but do not propagate.
func (w *Writer) forwardToBridge(event *Event) {
	if w.bridge == nil {
		return
	}

	// Use a background context with timeout for bridge operations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.bridge.Forward(ctx, event); err != nil {
		log.Printf("[events] bridge forward failed for %s/%s: %v",
			event.EventType, event.EntityID, err)
	}
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

	// Forward to Intermute bridge if attached (non-blocking, errors logged)
	w.forwardToBridge(event)

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
	return w.emit(EventInitiativeClosed, EntityInitiative, initiativeID, InitiativeClosedPayload{
		ID:     initiativeID,
		Reason: reason,
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
	return w.emit(EventEpicClosed, EntityEpic, epicID, EpicClosedPayload{
		ID:     epicID,
		Reason: reason,
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
	return w.emit(EventStoryClosed, EntityStory, storyID, StoryClosedPayload{
		ID:     storyID,
		Reason: reason,
	})
}

// Task events

// EmitTaskCreated emits a task created event
func (w *Writer) EmitTaskCreated(task *contract.Task) error {
	return w.emit(EventTaskCreated, EntityTask, task.ID, task)
}

// EmitTaskAssigned emits a task assigned event
func (w *Writer) EmitTaskAssigned(taskID, assignee string) error {
	return w.emit(EventTaskAssigned, EntityTask, taskID, TaskAssignedPayload{
		TaskID:   taskID,
		Assignee: assignee,
	})
}

// EmitTaskStarted emits a task started event
func (w *Writer) EmitTaskStarted(taskID string) error {
	return w.emit(EventTaskStarted, EntityTask, taskID, TaskStartedPayload{
		TaskID: taskID,
	})
}

// EmitTaskBlocked emits a task blocked event
func (w *Writer) EmitTaskBlocked(taskID, reason string) error {
	return w.emit(EventTaskBlocked, EntityTask, taskID, TaskBlockedPayload{
		TaskID: taskID,
		Reason: reason,
	})
}

// EmitTaskCompleted emits a task completed event
func (w *Writer) EmitTaskCompleted(taskID string) error {
	return w.emit(EventTaskCompleted, EntityTask, taskID, TaskCompletedPayload{
		TaskID: taskID,
	})
}

// Run events

// EmitRunStarted emits a run started event
func (w *Writer) EmitRunStarted(run *contract.Run) error {
	return w.emit(EventRunStarted, EntityRun, run.ID, run)
}

// EmitRunWaiting emits a run waiting event
func (w *Writer) EmitRunWaiting(runID, reason string) error {
	return w.emit(EventRunWaiting, EntityRun, runID, RunWaitingPayload{
		RunID:  runID,
		Reason: reason,
	})
}

// EmitRunCompleted emits a run completed event
func (w *Writer) EmitRunCompleted(runID string) error {
	return w.emit(EventRunCompleted, EntityRun, runID, RunCompletedPayload{
		RunID: runID,
	})
}

// EmitRunFailed emits a run failed event
func (w *Writer) EmitRunFailed(runID, reason string) error {
	return w.emit(EventRunFailed, EntityRun, runID, RunFailedPayload{
		RunID:  runID,
		Reason: reason,
	})
}

// Outcome events

// EmitOutcomeRecorded emits an outcome recorded event
func (w *Writer) EmitOutcomeRecorded(outcome *contract.Outcome) error {
	return w.emit(EventOutcomeRecorded, EntityOutcome, outcome.ID, outcome)
}

// Spec events

// EmitSpecRevised emits a spec revised event.
func (w *Writer) EmitSpecRevised(spec SpecSnapshot) error {
	return w.emit(EventSpecRevised, EntitySpec, spec.ID, SpecRevisedPayload{
		Spec: spec,
	})
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
