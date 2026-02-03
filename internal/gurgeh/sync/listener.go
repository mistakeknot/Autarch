// Package sync — listener.go subscribes to Coldwine task.blocked events
// and emits SignalTaskBlocked signals so Gurgeh can flag specs as needing revision.
package sync

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mistakeknot/autarch/pkg/intermute"
	"github.com/mistakeknot/autarch/pkg/signals"
)

// SignalEmitter abstracts signal persistence so the listener doesn't depend on concrete stores.
type SignalEmitter interface {
	Emit(sig signals.Signal) error
}

// Listener subscribes to Intermute task events and emits Gurgeh signals.
type Listener struct {
	client  *intermute.Client
	project string
	signals SignalEmitter
}

// NewListener creates a listener that converts task.blocked events into signals.
func NewListener(client *intermute.Client, project string, emitter SignalEmitter) *Listener {
	return &Listener{
		client:  client,
		project: project,
		signals: emitter,
	}
}

// blockedPayload mirrors the Coldwine broadcaster's BlockedPayload for JSON decoding.
type blockedPayload struct {
	EventType string `json:"event_type"`
	TaskID    string `json:"task_id"`
	SpecID    string `json:"spec_id,omitempty"`
	Title     string `json:"title"`
	Reason    string `json:"reason"`
}

// Register hooks the listener into the Intermute client's event dispatch.
// Call this before client.Subscribe(ctx, "task.blocked").
func (l *Listener) Register() {
	l.client.On("task.blocked", func(evt intermute.Event) {
		l.handleBlocked(evt)
	})
}

func (l *Listener) handleBlocked(evt intermute.Event) {
	// Decode Data — it may arrive as raw JSON string or pre-parsed map.
	var bp blockedPayload
	switch v := evt.Data.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &bp); err != nil {
			return
		}
	case []byte:
		if err := json.Unmarshal(v, &bp); err != nil {
			return
		}
	case map[string]interface{}:
		raw, _ := json.Marshal(v)
		if err := json.Unmarshal(raw, &bp); err != nil {
			return
		}
	default:
		return
	}

	if bp.SpecID == "" {
		return // No spec linkage — nothing for Gurgeh to act on.
	}

	sig := signals.Signal{
		ID:        fmt.Sprintf("task-blocked-%s-%d", bp.TaskID, time.Now().UnixMilli()),
		Type:      signals.SignalTaskBlocked,
		Source:    "coldwine",
		SpecID:    bp.SpecID,
		Severity:  signals.SeverityWarning,
		Title:     fmt.Sprintf("Task blocked: %s", bp.Title),
		Detail:    bp.Reason,
		CreatedAt: time.Now(),
	}

	// Best-effort emit — logging is the emitter's responsibility.
	_ = l.signals.Emit(sig)
}
