// Package intercore provides a Go client for the ic (Intercore) CLI.
// It shells out to the ic binary with --json and parses results.
// This is the Go equivalent of os/clavain/hooks/lib-intercore.sh.
package intercore

import (
	"errors"
	"fmt"
	"time"
)

// ErrUnavailable is returned when the ic binary is not found or unhealthy.
var ErrUnavailable = errors.New("intercore: ic binary not available")

// Run represents an Intercore sprint run.
type Run struct {
	ID            string   `json:"id"`
	Goal          string   `json:"goal"`
	Phase         string   `json:"phase"`
	Status        string   `json:"status"`
	ProjectDir    string   `json:"project_dir"`
	ScopeID       string   `json:"scope_id,omitempty"`
	Complexity    int      `json:"complexity"`
	AutoAdvance   bool     `json:"auto_advance"`
	ForceFull     bool     `json:"force_full"`
	TokenBudget   int64    `json:"token_budget,omitempty"`
	BudgetWarnPct int      `json:"budget_warn_pct,omitempty"`
	Phases        []string `json:"phases,omitempty"`
	CreatedAt     int64    `json:"created_at"`
	UpdatedAt     int64    `json:"updated_at"`
}

// IsActive returns true if the run status is "active".
func (r Run) IsActive() bool { return r.Status == "active" }

// CreatedTime returns CreatedAt as a time.Time.
func (r Run) CreatedTime() time.Time { return time.Unix(r.CreatedAt, 0) }

// UpdatedTime returns UpdatedAt as a time.Time.
func (r Run) UpdatedTime() time.Time { return time.Unix(r.UpdatedAt, 0) }

// Dispatch represents an agent dispatch.
type Dispatch struct {
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	Type      string `json:"type"`
	Agent     string `json:"agent,omitempty"`
	Status    string `json:"status"`
	ExitCode  *int   `json:"exit_code,omitempty"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`

	// Result fields — populated by ic dispatch status/list after completion.
	Name           *string `json:"name,omitempty"`
	OutputFile     *string `json:"output_file,omitempty"`
	VerdictSummary *string `json:"verdict_summary,omitempty"`
	ErrorMessage   *string `json:"error_message,omitempty"`
	InputTokens    int     `json:"in_tokens,omitempty"`
	OutputTokens   int     `json:"out_tokens,omitempty"`
}

// DisplayName returns the dispatch name if set, falling back to agent type.
func (d Dispatch) DisplayName() string {
	if d.Name != nil && *d.Name != "" {
		return *d.Name
	}
	if d.Agent != "" {
		return d.Agent
	}
	return d.ID
}

// ResultSummary returns a human-readable result summary for chat messages.
func (d Dispatch) ResultSummary() string {
	if d.VerdictSummary != nil && *d.VerdictSummary != "" {
		return *d.VerdictSummary
	}
	if d.ErrorMessage != nil && *d.ErrorMessage != "" {
		return *d.ErrorMessage
	}
	if d.ExitCode != nil {
		if *d.ExitCode == 0 {
			return "completed successfully"
		}
		return fmt.Sprintf("exited with code %d", *d.ExitCode)
	}
	return d.Status
}

// GateResult from ic gate check.
type GateResult struct {
	RunID     string        `json:"run_id"`
	FromPhase string        `json:"from_phase"`
	ToPhase   string        `json:"to_phase,omitempty"`
	Result    string        `json:"result"`
	Tier      string        `json:"tier"`
	Evidence  *GateEvidence `json:"evidence,omitempty"`
}

// Passed returns true if the gate check passed.
func (g GateResult) Passed() bool { return g.Result == "pass" }

// GateEvidence contains the individual condition checks.
type GateEvidence struct {
	Conditions []GateCondition `json:"conditions"`
}

// GateCondition is a single gate condition check result.
type GateCondition struct {
	Check  string `json:"check"`
	Phase  string `json:"phase,omitempty"`
	Result string `json:"result"`
	Count  int    `json:"count,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// GateRule describes a gate transition rule.
type GateRule struct {
	From   string           `json:"from"`
	To     string           `json:"to"`
	Checks []GateRuleCheck  `json:"checks"`
}

// GateRuleCheck is a single check within a gate rule.
type GateRuleCheck struct {
	Check string `json:"check"`
	Phase string `json:"phase,omitempty"`
}

// AdvanceResult from ic run advance.
type AdvanceResult struct {
	Advanced             bool     `json:"advanced"`
	FromPhase            string   `json:"from_phase"`
	ToPhase              string   `json:"to_phase"`
	GateResult           string   `json:"gate_result"`
	GateTier             string   `json:"gate_tier"`
	Reason               string   `json:"reason,omitempty"`
	EventType            string   `json:"event_type"`
	ActiveAgentCount     int      `json:"active_agent_count,omitempty"`
	NextGateRequirements []string `json:"next_gate_requirements,omitempty"`
}

// Succeeded returns true if the advance succeeded.
func (a AdvanceResult) Succeeded() bool { return a.Advanced }

// Artifact from ic run artifact list.
type Artifact struct {
	ID    string `json:"id,omitempty"`
	RunID string `json:"run_id"`
	Phase string `json:"phase"`
	Path  string `json:"path"`
	Type  string `json:"type,omitempty"`
}

// Event from ic events tail.
type Event struct {
	ID        int64  `json:"id"`
	RunID     string `json:"run_id"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	FromState string `json:"from_state,omitempty"`
	ToState   string `json:"to_state,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// EventTime returns Timestamp as a time.Time.
func (e Event) EventTime() time.Time { return time.Unix(e.Timestamp, 0) }

// RunAgent represents an agent registered on a run.
type RunAgent struct {
	ID         string `json:"id"`
	RunID      string `json:"run_id"`
	Type       string `json:"type"`
	Name       string `json:"name,omitempty"`
	DispatchID string `json:"dispatch_id,omitempty"`
	Status     string `json:"status"`
}

// BudgetResult from ic run budget.
type BudgetResult struct {
	RunID       string `json:"run_id"`
	TokenBudget int64  `json:"token_budget"`
	TokensUsed  int64  `json:"tokens_used"`
	Exceeded    bool   `json:"exceeded"`
	WarnPct     int    `json:"warn_pct,omitempty"`
}
