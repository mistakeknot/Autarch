// Package tier implements the T0-T3 autonomy state machine with
// graduation and demotion logic.
package tier

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// FSM manages tier state transitions.
type FSM struct {
	db      *sql.DB
	project string
}

// New creates a tier FSM connected to the decisions database.
func New(db *sql.DB, project string) *FSM {
	if project == "" {
		project = "demarch"
	}
	return &FSM{db: db, project: project}
}

// Current returns the current tier.
func (f *FSM) Current() (mycroft.Tier, error) {
	var val string
	err := f.db.QueryRow(
		`SELECT value FROM tier_state WHERE key = 'current_tier' AND project = ?`,
		f.project,
	).Scan(&val)
	if err == sql.ErrNoRows {
		return mycroft.T0, nil
	}
	if err != nil {
		return mycroft.T0, fmt.Errorf("query current tier: %w", err)
	}

	var tier int
	if _, err := fmt.Sscanf(val, "%d", &tier); err != nil {
		return mycroft.T0, fmt.Errorf("parse tier %q: %w", val, err)
	}
	return mycroft.Tier(tier), nil
}

// SetTier updates the current tier with a transition record.
func (f *FSM) SetTier(from, to mycroft.Tier, trigger string, evidence Evidence) error {
	tx, err := f.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	// Update tier_state.
	_, err = tx.Exec(
		`INSERT OR REPLACE INTO tier_state (key, project, value) VALUES ('current_tier', ?, ?)`,
		f.project, fmt.Sprintf("%d", to),
	)
	if err != nil {
		return fmt.Errorf("update tier_state: %w", err)
	}

	// Record transition.
	evidenceJSON, _ := json.Marshal(evidence)
	_, err = tx.Exec(
		`INSERT INTO tier_transitions (ts, project, from_tier, to_tier, trigger, evidence)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		now, f.project, int(from), int(to), trigger, string(evidenceJSON),
	)
	if err != nil {
		return fmt.Errorf("insert transition: %w", err)
	}

	return tx.Commit()
}

// Promote moves up one tier with the given evidence.
// All promotions are manual — the caller decides when to promote.
func (f *FSM) Promote(evidence Evidence) error {
	current, err := f.Current()
	if err != nil {
		return err
	}
	if current >= mycroft.T3 {
		return fmt.Errorf("already at maximum tier %s", current)
	}
	return f.SetTier(current, current+1, "manual", evidence)
}

// ShouldPromote evaluates whether promotion criteria are met for the current tier.
// Returns (should promote, next tier, evidence). Does NOT perform the promotion.
func ShouldPromote(current mycroft.Tier, history []DispatchRecord, cfg PromotionCriteria) (bool, Evidence) {
	if current >= mycroft.T3 {
		return false, Evidence{Reason: "already at max tier"}
	}

	// Filter to dispatches with outcomes (exclude pause/resume/shadow).
	var accepted, completed, total int
	for _, r := range history {
		if r.Outcome == "" {
			continue
		}
		total++
		// "success" implies acceptance — count it in both.
		if r.Outcome == string(mycroft.OutcomeAccepted) || r.Outcome == string(mycroft.OutcomeSuccess) {
			accepted++
		}
		if r.Outcome == string(mycroft.OutcomeSuccess) {
			completed++
		}
	}

	if total < cfg.MinSampleSize {
		return false, Evidence{
			SampleSize: total,
			Reason:     fmt.Sprintf("insufficient sample: %d < %d", total, cfg.MinSampleSize),
		}
	}

	approvalRate := float64(accepted) / float64(total)
	completionRate := float64(completed) / float64(total)

	evidence := Evidence{
		ApprovalRate:   approvalRate,
		CompletionRate: completionRate,
		SampleSize:     total,
	}

	if approvalRate < cfg.MinApprovalRate {
		evidence.Reason = fmt.Sprintf("approval rate %.0f%% < %.0f%%", approvalRate*100, cfg.MinApprovalRate*100)
		return false, evidence
	}
	if completionRate < cfg.MinCompletionRate {
		evidence.Reason = fmt.Sprintf("completion rate %.0f%% < %.0f%%", completionRate*100, cfg.MinCompletionRate*100)
		return false, evidence
	}

	evidence.Reason = fmt.Sprintf("criteria met: %.0f%% approval, %.0f%% completion, %d samples",
		approvalRate*100, completionRate*100, total)
	return true, evidence
}

// PromotionCriteria defines the thresholds for tier graduation.
type PromotionCriteria struct {
	MinApprovalRate   float64 // e.g., 0.9 for 90%
	MinCompletionRate float64 // e.g., 0.7 for 70%
	MinSampleSize     int     // minimum dispatches to evaluate
}

// DefaultPromotionCriteria returns the brainstorm-specified thresholds:
// >90% approval AND >70% completion.
func DefaultPromotionCriteria() PromotionCriteria {
	return PromotionCriteria{
		MinApprovalRate:   0.9,
		MinCompletionRate: 0.7,
		MinSampleSize:     20,
	}
}

// Demote drops one tier based on the given trigger.
func (f *FSM) Demote(trigger string, evidence Evidence) error {
	current, err := f.Current()
	if err != nil {
		return err
	}
	if current <= mycroft.T0 {
		return fmt.Errorf("already at minimum tier %s", current)
	}
	return f.SetTier(current, current-1, trigger, evidence)
}

// Evidence captures the data supporting a tier transition.
type Evidence struct {
	ApprovalRate   float64 `json:"approval_rate,omitempty"`
	CompletionRate float64 `json:"completion_rate,omitempty"`
	SampleSize     int     `json:"sample_size,omitempty"`
	FailureRate    float64 `json:"failure_rate,omitempty"`
	Reason         string  `json:"reason,omitempty"`
}

// ShouldDemote checks if demotion triggers are met based on dispatch history.
func ShouldDemote(current mycroft.Tier, history []DispatchRecord, cfg mycroft.DemotionTriggers) (bool, string, Evidence) {
	if len(history) < cfg.MinSampleSize {
		return false, "", Evidence{SampleSize: len(history)}
	}

	// Count failures in the window.
	var failures, total int
	var consecutive int
	var maxConsecutive int

	for _, r := range history {
		total++
		if r.Outcome == string(mycroft.OutcomeFailure) {
			failures++
			consecutive++
			if consecutive > maxConsecutive {
				maxConsecutive = consecutive
			}
		} else {
			consecutive = 0
		}
	}

	// Consecutive failure check.
	if maxConsecutive >= cfg.ConsecutiveFailureLimit {
		return true, "consecutive_failures", Evidence{
			FailureRate: float64(failures) / float64(total),
			SampleSize:  total,
			Reason:      fmt.Sprintf("%d consecutive failures", maxConsecutive),
		}
	}

	// Rate-based check (symmetric thresholds).
	failureRate := float64(failures) / float64(total)
	var threshold float64
	switch current {
	case mycroft.T3:
		threshold = cfg.T3FailureRateThreshold
	case mycroft.T2:
		threshold = cfg.T2FailureRateThreshold
	default:
		return false, "", Evidence{FailureRate: failureRate, SampleSize: total}
	}

	if failureRate > threshold {
		return true, "circuit_breaker", Evidence{
			FailureRate: failureRate,
			SampleSize:  total,
			Reason:      fmt.Sprintf("failure rate %.1f%% > threshold %.1f%%", failureRate*100, threshold*100),
		}
	}

	return false, "", Evidence{FailureRate: failureRate, SampleSize: total}
}

// DispatchRecord is a simplified view of a dispatch_log entry for analysis.
type DispatchRecord struct {
	Agent   string
	Bead    string
	Action  string
	Outcome string
}

// Transitions returns the history of tier transitions.
func (f *FSM) Transitions(limit int) ([]TransitionRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := f.db.Query(
		`SELECT ts, from_tier, to_tier, trigger, evidence
		 FROM tier_transitions WHERE project = ?
		 ORDER BY id DESC LIMIT ?`,
		f.project, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query transitions: %w", err)
	}
	defer rows.Close()

	var records []TransitionRecord
	for rows.Next() {
		var r TransitionRecord
		var evidenceJSON string
		if err := rows.Scan(&r.Timestamp, &r.FromTier, &r.ToTier, &r.Trigger, &evidenceJSON); err != nil {
			return nil, fmt.Errorf("scan transition: %w", err)
		}
		json.Unmarshal([]byte(evidenceJSON), &r.Evidence)
		records = append(records, r)
	}
	return records, rows.Err()
}

// TransitionRecord captures a tier change.
type TransitionRecord struct {
	Timestamp int64
	FromTier  int
	ToTier    int
	Trigger   string
	Evidence  Evidence
}
