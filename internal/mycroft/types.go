// Package mycroft implements the fleet orchestrator — coordinating agent
// sessions, ranking work, and managing dispatch at escalating autonomy tiers.
package mycroft

import (
	"github.com/mistakeknot/autarch/pkg/fleet"
)

// Re-export fleet view types so existing consumers keep working.
// New code should import pkg/fleet directly.
type (
	FleetView    = fleet.FleetView
	AgentView    = fleet.AgentView
	BeadView     = fleet.BeadView
	ConflictView = fleet.ConflictView
	CostProfile  = fleet.CostProfile
	HealthReport = fleet.HealthReport
	DataSource   = fleet.DataSource
)

// Tier represents an autonomy level.
type Tier int

const (
	T0 Tier = 0 // Observe/shadow — no actions, log what would be suggested
	T1 Tier = 1 // Suggest/approve — present suggestions, user approves
	T2 Tier = 2 // Auto-dispatch (low-risk) — dispatch within allowlist
	T3 Tier = 3 // Full dispatch — dispatch anything
)

func (t Tier) String() string {
	switch t {
	case T0:
		return "T0:observe"
	case T1:
		return "T1:suggest"
	case T2:
		return "T2:auto-low-risk"
	case T3:
		return "T3:auto-full"
	default:
		return "unknown"
	}
}

// FailureClass categorizes an agent's failure state for recovery routing.
type FailureClass string

const (
	FailureHealthy   FailureClass = "healthy"
	FailureClean     FailureClass = "clean"     // no uncommitted changes
	FailureDirty     FailureClass = "dirty"     // uncommitted changes present
	FailureDegraded  FailureClass = "degraded"  // high token spend, no progress
	FailureCorrupted FailureClass = "corrupted" // known-bad git state
)

// DispatchAction enumerates the actions Mycroft can take.
type DispatchAction string

const (
	ActionShadowSuggest  DispatchAction = "shadow_suggest"
	ActionSuggest        DispatchAction = "suggest"
	ActionAutoDispatch   DispatchAction = "auto_dispatch"
	ActionRestart        DispatchAction = "restart"
	ActionPatchReassign  DispatchAction = "patch_reassign"
	ActionEscalate       DispatchAction = "escalate"
	ActionPause          DispatchAction = "pause"
	ActionResume         DispatchAction = "resume"
	ActionManualOverride DispatchAction = "manual_override"
)

// DispatchOutcome records what happened with a dispatch.
type DispatchOutcome string

const (
	OutcomeAccepted DispatchOutcome = "accepted"
	OutcomeRejected DispatchOutcome = "rejected"
	OutcomeSuccess  DispatchOutcome = "success"
	OutcomeFailure  DispatchOutcome = "failure"
)
