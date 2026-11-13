package mycroft

import (
	"database/sql"
	"fmt"
	"time"
)

// ApprovalLog records explicit spawn approvals in the existing dispatch_log table.
type ApprovalLog struct {
	db      *sql.DB
	project string
}

func NewApprovalLog(db *sql.DB, project string) *ApprovalLog {
	if project == "" {
		project = "demarch"
	}
	return &ApprovalLog{db: db, project: project}
}
func (d *ApprovalLog) LogApprovalWith(agent, beadID string, outcome DispatchOutcome, reason string) error {
	return d.logDispatchFull(agent, beadID, ActionSuggest, string(outcome), reason, "", 0)
}
func (d *ApprovalLog) logDispatchFull(agent string, beadID string, action DispatchAction, outcome string, reason string, contextStr string, costActual float64) error {
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

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
