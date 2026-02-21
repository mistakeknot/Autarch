package icdata

// KernelEvent represents typed kernel event categories from Intercore.
type KernelEvent int

const (
	EventUnknown           KernelEvent = iota
	EventPhaseAdvance                  // "phase.advance"
	EventPhaseRollback                 // "phase.rollback"
	EventGateCheck                     // "gate.check"
	EventGatePassed                    // "gate.passed"
	EventGateFailed                    // "gate.failed"
	EventDispatchSpawned               // "dispatch.spawned"
	EventDispatchCompleted             // "dispatch.completed"
	EventDispatchFailed                // "dispatch.failed"
	EventDispatchCancelled             // "dispatch.cancelled"
	EventArtifactAdded                 // "artifact.added"
	EventTokensRecorded                // "tokens.recorded"
	EventBudgetExceeded                // "budget.exceeded"
)

// String returns the ic event string representation.
func (e KernelEvent) String() string {
	switch e {
	case EventPhaseAdvance:
		return "phase.advance"
	case EventPhaseRollback:
		return "phase.rollback"
	case EventGateCheck:
		return "gate.check"
	case EventGatePassed:
		return "gate.passed"
	case EventGateFailed:
		return "gate.failed"
	case EventDispatchSpawned:
		return "dispatch.spawned"
	case EventDispatchCompleted:
		return "dispatch.completed"
	case EventDispatchFailed:
		return "dispatch.failed"
	case EventDispatchCancelled:
		return "dispatch.cancelled"
	case EventArtifactAdded:
		return "artifact.added"
	case EventTokensRecorded:
		return "tokens.recorded"
	case EventBudgetExceeded:
		return "budget.exceeded"
	default:
		return "unknown"
	}
}

var kernelEventMap = map[string]KernelEvent{
	"phase.advance":       EventPhaseAdvance,
	"phase.rollback":      EventPhaseRollback,
	"gate.check":          EventGateCheck,
	"gate.passed":         EventGatePassed,
	"gate.failed":         EventGateFailed,
	"dispatch.spawned":    EventDispatchSpawned,
	"dispatch.completed":  EventDispatchCompleted,
	"dispatch.failed":     EventDispatchFailed,
	"dispatch.cancelled":  EventDispatchCancelled,
	"artifact.added":      EventArtifactAdded,
	"tokens.recorded":     EventTokensRecorded,
	"budget.exceeded":     EventBudgetExceeded,
}

// ParseKernelEvent converts a string to a KernelEvent. Returns EventUnknown for unrecognized strings.
func ParseKernelEvent(s string) KernelEvent {
	if e, ok := kernelEventMap[s]; ok {
		return e
	}
	return EventUnknown
}
