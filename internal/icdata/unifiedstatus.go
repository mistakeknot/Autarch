package icdata

import "strings"

// UnifiedStatus represents a normalized 5-state status model that collapses
// the many raw status strings from Intercore, Intermute, and tmux.
type UnifiedStatus int

const (
	StatusActive  UnifiedStatus = iota // working, running
	StatusBlocked                      // blocked, stalled, permission-required
	StatusWaiting                      // waiting, idle, queued
	StatusDone                         // completed, done
	StatusErr                          // failed, error, cancelled, timeout
	StatusUnknown                      // "", unknown
)

// String returns a human-readable label for the status.
func (s UnifiedStatus) String() string {
	switch s {
	case StatusActive:
		return "active"
	case StatusBlocked:
		return "blocked"
	case StatusWaiting:
		return "waiting"
	case StatusDone:
		return "done"
	case StatusErr:
		return "error"
	default:
		return "unknown"
	}
}

// UnifyStatus maps a raw status string to a UnifiedStatus.
func UnifyStatus(raw string) UnifiedStatus {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	// Active states
	case "working", "running", "active", "in_progress", "in-progress", "executing":
		return StatusActive

	// Blocked states
	case "blocked", "stalled", "permission-required", "permission_required":
		return StatusBlocked

	// Waiting states
	case "waiting", "idle", "queued", "pending", "paused", "todo", "assigned", "draft", "open":
		return StatusWaiting

	// Done states
	case "completed", "done", "complete", "closed", "shipped":
		return StatusDone

	// Error states
	case "failed", "error", "cancelled", "canceled", "timeout", "stopped", "crashed":
		return StatusErr

	default:
		return StatusUnknown
	}
}
