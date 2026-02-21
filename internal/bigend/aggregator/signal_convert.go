package aggregator

import (
	"fmt"
	"time"

	"github.com/mistakeknot/autarch/pkg/signals"
)

// signalMapping defines the conversion table from aggregator event types to signal types.
// Each entry is checked by exact match. Order does not matter.
var signalMapping = []struct {
	eventType string
	sigType   signals.SignalType
	severity  signals.Severity
}{
	{"task.blocked", signals.SignalTaskBlocked, signals.SeverityWarning},
	{"run.failed", signals.SignalExecutionDrift, signals.SeverityWarning},
	{"run.waiting", signals.SignalExecutionDrift, signals.SeverityInfo},
	{"spec.revised", signals.SignalSpecHealthLow, signals.SeverityInfo},
}

// eventToSignal converts an aggregator Event to a signals.Signal.
// Returns false if the event type has no signal mapping (unmapped events are silently skipped).
func eventToSignal(evt Event) (signals.Signal, bool) {
	for _, m := range signalMapping {
		if evt.Type == m.eventType {
			sig := signals.Signal{
				ID:        evt.EntityID,
				Type:      m.sigType,
				Source:    "intermute",
				Severity:  m.severity,
				Title:     fmt.Sprintf("[%s] %s", evt.Type, evt.EntityID),
				CreatedAt: evt.Timestamp,
			}
			if sig.CreatedAt.IsZero() {
				sig.CreatedAt = time.Now()
			}
			return sig, true
		}
	}
	return signals.Signal{}, false
}
