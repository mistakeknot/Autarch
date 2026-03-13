package scheduler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// Dispatcher handles the dispatch flow: claim bead → write state → spawn agent.
type Dispatcher struct {
	db      *sql.DB
	spawner AgentSpawner
	project string
}

// AgentSpawner creates and manages agent sessions.
type AgentSpawner interface {
	Spawn(agentName string, bead mycroft.BeadView, contextFile string) (sessionID string, err error)
	Kill(sessionID string) error
}

// NewDispatcher creates a Dispatcher.
func NewDispatcher(db *sql.DB, spawner AgentSpawner, project string) *Dispatcher {
	if project == "" {
		project = "demarch"
	}
	return &Dispatcher{db: db, spawner: spawner, project: project}
}

// LogShadow records a shadow suggestion (T0 behavior).
func (d *Dispatcher) LogShadow(agent string, bead mycroft.BeadView, reasoning string) error {
	return d.logDispatchFull(agent, bead.ID, mycroft.ActionShadowSuggest, "", "", reasoning, 0)
}

// LogSuggestion records a dispatch suggestion (T1 behavior).
func (d *Dispatcher) LogSuggestion(agent string, bead mycroft.BeadView, reasoning string) error {
	return d.logDispatchFull(agent, bead.ID, mycroft.ActionSuggest, "", "", reasoning, 0)
}

// LogApproval records user approval of a suggestion.
func (d *Dispatcher) LogApproval(agent string, beadID string) error {
	return d.logDispatch(agent, beadID, mycroft.ActionSuggest, string(mycroft.OutcomeAccepted), "", 0)
}

// LogRejection records user rejection of a suggestion with reason.
func (d *Dispatcher) LogRejection(agent string, beadID string, reason string) error {
	return d.logDispatch(agent, beadID, mycroft.ActionSuggest, string(mycroft.OutcomeRejected), reason, 0)
}

// LogPause records a pause action.
func (d *Dispatcher) LogPause() error {
	return d.logDispatch("mycroft", "", mycroft.ActionPause, "", "", 0)
}

// LogResume records a resume action.
func (d *Dispatcher) LogResume() error {
	return d.logDispatch("mycroft", "", mycroft.ActionResume, "", "", 0)
}

// LogOverride records a manual override.
func (d *Dispatcher) LogOverride(agent string, beadID string, reason string) error {
	return d.logDispatch(agent, beadID, mycroft.ActionManualOverride, string(mycroft.OutcomeAccepted), reason, 0)
}

func (d *Dispatcher) logDispatch(agent string, beadID string, action mycroft.DispatchAction, outcome string, reason string, costActual float64) error {
	return d.logDispatchFull(agent, beadID, action, outcome, reason, "", costActual)
}

func (d *Dispatcher) logDispatchFull(agent string, beadID string, action mycroft.DispatchAction, outcome string, reason string, contextStr string, costActual float64) error {
	_, err := d.db.Exec(
		`INSERT INTO dispatch_log (ts, project, agent, bead, action, outcome, reason, context, cost_actual)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().Unix(), d.project, agent, beadID, string(action),
		nullString(outcome), nullString(reason), nullString(contextStr), costActual,
	)
	if err != nil {
		return fmt.Errorf("log dispatch: %w", err)
	}
	return nil
}

// DispatchHistory returns recent dispatch log entries.
func (d *Dispatcher) DispatchHistory(limit int) ([]DispatchEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.db.Query(
		`SELECT id, ts, agent, bead, action, outcome, reason, context
		 FROM dispatch_log WHERE project = ?
		 ORDER BY id DESC LIMIT ?`,
		d.project, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query dispatch history: %w", err)
	}
	defer rows.Close()

	var entries []DispatchEntry
	for rows.Next() {
		var e DispatchEntry
		var outcome, reason, ctx sql.NullString
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Agent, &e.Bead, &e.Action, &outcome, &reason, &ctx); err != nil {
			return nil, fmt.Errorf("scan dispatch entry: %w", err)
		}
		e.Outcome = outcome.String
		e.Reason = reason.String
		e.Context = ctx.String
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ShadowDigest returns shadow suggestions from dispatch_log for review.
func (d *Dispatcher) ShadowDigest(limit int) ([]DispatchEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := d.db.Query(
		`SELECT id, ts, agent, bead, action, outcome, reason, context
		 FROM dispatch_log WHERE project = ? AND action = 'shadow_suggest'
		 ORDER BY id DESC LIMIT ?`,
		d.project, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query shadow digest: %w", err)
	}
	defer rows.Close()

	var entries []DispatchEntry
	for rows.Next() {
		var e DispatchEntry
		var outcome, reason, ctx sql.NullString
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Agent, &e.Bead, &e.Action, &outcome, &reason, &ctx); err != nil {
			return nil, fmt.Errorf("scan shadow entry: %w", err)
		}
		e.Outcome = outcome.String
		e.Reason = reason.String
		e.Context = ctx.String
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// OverridePatterns summarizes rejection patterns from dispatch_log.
func (d *Dispatcher) OverridePatterns() ([]OverridePattern, error) {
	rows, err := d.db.Query(
		`SELECT reason, COUNT(*) as cnt
		 FROM dispatch_log
		 WHERE project = ? AND action IN ('suggest', 'manual_override')
		   AND outcome = 'rejected' AND reason != ''
		 GROUP BY reason
		 ORDER BY cnt DESC LIMIT 10`,
		d.project,
	)
	if err != nil {
		return nil, fmt.Errorf("query override patterns: %w", err)
	}
	defer rows.Close()

	var patterns []OverridePattern
	for rows.Next() {
		var p OverridePattern
		if err := rows.Scan(&p.Reason, &p.Count); err != nil {
			return nil, fmt.Errorf("scan pattern: %w", err)
		}
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}

// DispatchEntry represents a row from dispatch_log.
type DispatchEntry struct {
	ID        int64
	Timestamp int64
	Agent     string
	Bead      string
	Action    string
	Outcome   string
	Reason    string
	Context   string
}

// OverridePattern summarizes a rejection reason.
type OverridePattern struct {
	Reason string
	Count  int
}

// ContextJSON builds the context JSON for a dispatch log entry.
func ContextJSON(tier mycroft.Tier, costEstimate float64, reasoning string) string {
	ctx := map[string]interface{}{
		"tier_at_time":  int(tier),
		"cost_estimate": costEstimate,
		"reasoning":     reasoning,
	}
	data, _ := json.Marshal(ctx)
	return string(data)
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
