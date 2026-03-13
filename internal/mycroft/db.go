package mycroft

import (
	"database/sql"
	"fmt"

	autarchdb "github.com/mistakeknot/autarch/pkg/db"
)

// Schema version for migrations.
const schemaVersion = 1

const schema = `
CREATE TABLE IF NOT EXISTS dispatch_log (
  id INTEGER PRIMARY KEY,
  ts INTEGER NOT NULL,
  project TEXT NOT NULL DEFAULT 'demarch',
  agent TEXT NOT NULL,
  bead TEXT NOT NULL,
  action TEXT NOT NULL,
  outcome TEXT,
  reason TEXT,
  context TEXT,
  cost_actual REAL
);

CREATE TABLE IF NOT EXISTS tier_state (
  key TEXT NOT NULL,
  project TEXT NOT NULL DEFAULT 'demarch',
  value TEXT NOT NULL,
  PRIMARY KEY (key, project)
);

CREATE TABLE IF NOT EXISTS tier_transitions (
  id INTEGER PRIMARY KEY,
  ts INTEGER NOT NULL,
  project TEXT NOT NULL DEFAULT 'demarch',
  from_tier INTEGER NOT NULL,
  to_tier INTEGER NOT NULL,
  trigger TEXT NOT NULL,
  evidence TEXT
);

CREATE TABLE IF NOT EXISTS recovery_log (
  id INTEGER PRIMARY KEY,
  ts INTEGER NOT NULL,
  agent TEXT NOT NULL,
  bead TEXT NOT NULL,
  action TEXT NOT NULL,
  status TEXT NOT NULL,
  error TEXT,
  context TEXT
);

CREATE INDEX IF NOT EXISTS idx_dispatch_log_ts ON dispatch_log(ts);
CREATE INDEX IF NOT EXISTS idx_dispatch_log_agent ON dispatch_log(agent);
CREATE INDEX IF NOT EXISTS idx_dispatch_log_bead ON dispatch_log(bead);
CREATE INDEX IF NOT EXISTS idx_recovery_log_status ON recovery_log(status);
`

// OpenDB opens or creates the decisions database at the given path.
// Uses the shared Autarch SQLite helper (WAL mode, single connection).
func OpenDB(path string) (*sql.DB, error) {
	db, err := autarchdb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open mycroft db: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init mycroft schema: %w", err)
	}

	return db, nil
}
