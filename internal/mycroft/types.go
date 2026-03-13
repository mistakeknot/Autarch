// Package mycroft implements the fleet orchestrator — coordinating agent
// sessions, ranking work, and managing dispatch at escalating autonomy tiers.
package mycroft

import "time"

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

// FleetView is the composed view of all agents and work, rebuilt each patrol cycle.
type FleetView struct {
	Agents    []AgentView
	Work      []BeadView
	Conflicts []ConflictView
	Freshness map[string]time.Time // per-source timestamp (keyed by source name)
}

// AgentView represents a single agent's state, composed from multiple sources.
type AgentView struct {
	Name         string      `json:"name"`          // Culture ship name (canonical)
	Runtime      string      `json:"runtime"`       // "claude-code" | "skaffen"
	Capabilities []string    `json:"capabilities"`  // from fleet-registry.yaml
	Status       string      `json:"status"`        // from intermux: active, idle, stuck, crashed
	CostProfile  CostProfile `json:"cost_profile"`  // from fleet-registry + interstat
	CurrentBead  string      `json:"current_bead"`  // from beads claim state
	Health       HealthReport `json:"health"`
	Reservations []string    `json:"reservations"`  // from interlock
}

// CostProfile holds cost estimation data for an agent.
type CostProfile struct {
	EstimatedPerBead float64 `json:"estimated_per_bead"` // USD estimate
	Model            string  `json:"model"`              // preferred model
}

// HealthReport captures an agent's health signals.
type HealthReport struct {
	LastSeen  time.Time `json:"last_seen"`
	IsHealthy bool      `json:"is_healthy"`
	Details   string    `json:"details,omitempty"`
}

// BeadView represents a bead from the work queue.
type BeadView struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Type         string   `json:"type"`     // task, bug, feature, docs
	Priority     int      `json:"priority"` // 0-4 (0=critical)
	Complexity   string   `json:"complexity"` // simple, medium, complex, unknown
	Labels       []string `json:"labels"`
	Dependencies []string `json:"dependencies"` // bead IDs this depends on
	DepsResolved bool     `json:"deps_resolved"`
	CreatedAt    time.Time `json:"created_at"`
	ClaimedBy    string   `json:"claimed_by,omitempty"`
}

// ConflictView represents a file reservation conflict.
type ConflictView struct {
	File    string   `json:"file"`
	Holders []string `json:"holders"` // agent names holding reservations
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

// DataSource provides fleet state from external systems.
type DataSource interface {
	FleetState() (FleetView, error)
	AgentHealth(name string) (string, error) // returns status string
	BeadQueue() ([]BeadView, error)
}
