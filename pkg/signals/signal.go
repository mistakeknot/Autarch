// Package signals provides typed alerts for cross-tool communication in Autarch.
// Signals represent real-world changes that affect spec quality: competitor moves,
// stale assumptions, hypothesis timeouts, and execution drift.
package signals

import "time"

// SignalType identifies the category of signal.
type SignalType string

const (
	SignalCompetitorShipped    SignalType = "competitor_shipped"
	SignalResearchInvalidation SignalType = "research_invalidation"
	SignalAssumptionDecayed    SignalType = "assumption_decayed"
	SignalHypothesisStale      SignalType = "hypothesis_stale"
	SignalSpecHealthLow        SignalType = "spec_health_low"
	SignalExecutionDrift       SignalType = "execution_drift"
	SignalVisionDrift          SignalType = "vision_drift"
	SignalTaskBlocked          SignalType = "task_blocked"
)

// Severity indicates urgency of a signal.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Signal represents a typed alert about a real-world change affecting a spec.
type Signal struct {
	ID            string     `json:"id" yaml:"id"`
	Type          SignalType `json:"type" yaml:"type"`
	Source        string     `json:"source" yaml:"source"`       // "pollard", "gurgeh", "coldwine"
	SpecID        string     `json:"spec_id" yaml:"spec_id"`
	AffectedField string    `json:"affected_field" yaml:"affected_field"` // spec field this signal targets (dedup key)
	Severity      Severity   `json:"severity" yaml:"severity"`
	Title         string     `json:"title" yaml:"title"`
	Detail        string     `json:"detail" yaml:"detail"`
	CreatedAt     time.Time  `json:"created_at" yaml:"created_at"`
	Dismissed     bool       `json:"dismissed" yaml:"dismissed"`
	DismissedAt   *time.Time `json:"dismissed_at,omitempty" yaml:"dismissed_at,omitempty"`
}

// IsActive returns true if the signal has not been dismissed.
func (s *Signal) IsActive() bool {
	return !s.Dismissed
}
