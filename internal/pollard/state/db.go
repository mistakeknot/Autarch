// Package state provides SQLite-based state management for Pollard.
package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	autarchdb "github.com/mistakeknot/autarch/pkg/db"
)

// DB wraps the SQLite database connection.
type DB struct {
	db      *sql.DB
	dbPath  string
}

// HunterRun represents a single run of a hunter.
type HunterRun struct {
	ID                 int64
	HunterName         string
	StartedAt          time.Time
	CompletedAt        *time.Time
	Status             string // running, success, failed
	SourcesCollected   int
	InsightsGenerated  int
	ErrorMessage       string
}

// RateLimit tracks API rate limit status.
type RateLimit struct {
	APIName           string
	RequestsRemaining int
	ResetAt           time.Time
}

// Open opens or creates the state database.
func Open(projectPath string) (*DB, error) {
	pollardDir := filepath.Join(projectPath, ".pollard")
	if err := os.MkdirAll(pollardDir, 0755); err != nil {
		return nil, fmt.Errorf("create .pollard dir: %w", err)
	}

	dbPath := filepath.Join(pollardDir, "state.db")
	db, err := autarchdb.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &DB{db: db, dbPath: dbPath}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *DB) Close() error {
	return s.db.Close()
}

// migrate creates the database schema.
func (s *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS hunter_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hunter_name TEXT NOT NULL,
		started_at TEXT NOT NULL,
		completed_at TEXT,
		status TEXT NOT NULL,
		sources_collected INTEGER DEFAULT 0,
		insights_generated INTEGER DEFAULT 0,
		error_message TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_hunter_runs_name ON hunter_runs(hunter_name);
	CREATE INDEX IF NOT EXISTS idx_hunter_runs_started ON hunter_runs(started_at);

	CREATE TABLE IF NOT EXISTS rate_limits (
		api_name TEXT PRIMARY KEY,
		requests_remaining INTEGER NOT NULL,
		reset_at TEXT NOT NULL
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

// StartRun records the start of a hunter run.
func (s *DB) StartRun(hunterName string) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO hunter_runs (hunter_name, started_at, status) VALUES (?, ?, ?)`,
		hunterName, time.Now().Format(time.RFC3339), "running",
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// CompleteRun marks a run as completed.
func (s *DB) CompleteRun(runID int64, success bool, sourcesCollected, insightsGenerated int, errMsg string) error {
	status := "success"
	if !success {
		status = "failed"
	}
	_, err := s.db.Exec(
		`UPDATE hunter_runs SET
			completed_at = ?,
			status = ?,
			sources_collected = ?,
			insights_generated = ?,
			error_message = ?
		WHERE id = ?`,
		time.Now().Format(time.RFC3339), status, sourcesCollected, insightsGenerated, errMsg, runID,
	)
	return err
}

// LastRun returns the most recent run for a hunter.
func (s *DB) LastRun(hunterName string) (*HunterRun, error) {
	row := s.db.QueryRow(
		`SELECT id, hunter_name, started_at, completed_at, status, sources_collected, insights_generated, error_message
		FROM hunter_runs WHERE hunter_name = ? ORDER BY started_at DESC LIMIT 1`,
		hunterName,
	)

	var run HunterRun
	var startedAt, completedAt, errMsg sql.NullString
	err := row.Scan(
		&run.ID, &run.HunterName, &startedAt, &completedAt, &run.Status,
		&run.SourcesCollected, &run.InsightsGenerated, &errMsg,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	run.StartedAt, _ = time.Parse(time.RFC3339, startedAt.String)
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		run.CompletedAt = &t
	}
	run.ErrorMessage = errMsg.String
	return &run, nil
}

// RecentRuns returns recent runs for all hunters.
func (s *DB) RecentRuns(limit int) ([]*HunterRun, error) {
	rows, err := s.db.Query(
		`SELECT id, hunter_name, started_at, completed_at, status, sources_collected, insights_generated, error_message
		FROM hunter_runs ORDER BY started_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*HunterRun
	for rows.Next() {
		var run HunterRun
		var startedAt, completedAt, errMsg sql.NullString
		if err := rows.Scan(
			&run.ID, &run.HunterName, &startedAt, &completedAt, &run.Status,
			&run.SourcesCollected, &run.InsightsGenerated, &errMsg,
		); err != nil {
			return nil, err
		}
		run.StartedAt, _ = time.Parse(time.RFC3339, startedAt.String)
		if completedAt.Valid {
			t, _ := time.Parse(time.RFC3339, completedAt.String)
			run.CompletedAt = &t
		}
		run.ErrorMessage = errMsg.String
		runs = append(runs, &run)
	}
	return runs, rows.Err()
}

// GetRateLimit returns the current rate limit status for an API.
func (s *DB) GetRateLimit(apiName string) (*RateLimit, error) {
	row := s.db.QueryRow(
		`SELECT api_name, requests_remaining, reset_at FROM rate_limits WHERE api_name = ?`,
		apiName,
	)

	var rl RateLimit
	var resetAt string
	err := row.Scan(&rl.APIName, &rl.RequestsRemaining, &resetAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rl.ResetAt, _ = time.Parse(time.RFC3339, resetAt)
	return &rl, nil
}

// SetRateLimit updates the rate limit status for an API.
func (s *DB) SetRateLimit(apiName string, remaining int, resetAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO rate_limits (api_name, requests_remaining, reset_at) VALUES (?, ?, ?)`,
		apiName, remaining, resetAt.Format(time.RFC3339),
	)
	return err
}

// ShouldRun checks if a hunter should run based on its schedule.
func (s *DB) ShouldRun(hunterName string, interval time.Duration) (bool, error) {
	lastRun, err := s.LastRun(hunterName)
	if err != nil {
		return false, err
	}
	if lastRun == nil {
		return true, nil // Never run before
	}
	if lastRun.Status == "running" {
		return false, nil // Still running
	}
	return time.Since(lastRun.StartedAt) >= interval, nil
}

// Stats returns overall statistics.
type Stats struct {
	TotalRuns       int
	SuccessfulRuns  int
	FailedRuns      int
	TotalSources    int
	TotalInsights   int
	LastRunAt       *time.Time
}

// GetStats returns overall hunter statistics.
func (s *DB) GetStats() (*Stats, error) {
	var stats Stats

	row := s.db.QueryRow(`SELECT COUNT(*) FROM hunter_runs`)
	if err := row.Scan(&stats.TotalRuns); err != nil {
		return nil, err
	}

	row = s.db.QueryRow(`SELECT COUNT(*) FROM hunter_runs WHERE status = 'success'`)
	if err := row.Scan(&stats.SuccessfulRuns); err != nil {
		return nil, err
	}

	row = s.db.QueryRow(`SELECT COUNT(*) FROM hunter_runs WHERE status = 'failed'`)
	if err := row.Scan(&stats.FailedRuns); err != nil {
		return nil, err
	}

	row = s.db.QueryRow(`SELECT COALESCE(SUM(sources_collected), 0) FROM hunter_runs`)
	if err := row.Scan(&stats.TotalSources); err != nil {
		return nil, err
	}

	row = s.db.QueryRow(`SELECT COALESCE(SUM(insights_generated), 0) FROM hunter_runs`)
	if err := row.Scan(&stats.TotalInsights); err != nil {
		return nil, err
	}

	var lastRunAt sql.NullString
	row = s.db.QueryRow(`SELECT MAX(started_at) FROM hunter_runs`)
	if err := row.Scan(&lastRunAt); err != nil {
		return nil, err
	}
	if lastRunAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastRunAt.String)
		stats.LastRunAt = &t
	}

	return &stats, nil
}
