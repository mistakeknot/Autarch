// Package fleet provides shared fleet view types used by the Mycroft
// orchestrator and its TUI dashboard. Types here are the public contract
// between the patrol loop, aggregator sources, and view layer.
package fleet

import "time"

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
	Reservations []string    `json:"reservations"` // from interlock
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
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Type         string    `json:"type"`     // task, bug, feature, docs
	Priority     int       `json:"priority"` // 0-4 (0=critical)
	Complexity   string    `json:"complexity"`
	Labels       []string  `json:"labels"`
	Dependencies []string  `json:"dependencies"`
	DepsResolved bool      `json:"deps_resolved"`
	CreatedAt    time.Time `json:"created_at"`
	ClaimedBy    string    `json:"claimed_by,omitempty"`
}

// ConflictView represents a file reservation conflict.
type ConflictView struct {
	File    string   `json:"file"`
	Holders []string `json:"holders"` // agent names holding reservations
}

// DataSource provides fleet state from external systems.
type DataSource interface {
	FleetState() (FleetView, error)
	AgentHealth(name string) (string, error) // returns status string
	BeadQueue() ([]BeadView, error)
}
