// Package db provides a unified SQLite open helper for Autarch tools.
// It enforces WAL mode, NORMAL synchronous, busy timeout, and connection
// pool limits as best practices for embedded SQLite usage.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver
)

// Open opens a SQLite database at the given path with production-hardened
// settings: WAL journal mode, NORMAL synchronous, 5s busy timeout, and
// a single connection (SQLite best practice for writers).
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	// Execute pragmas directly — modernc.org/sqlite does not support DSN params.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite pragma %q on %s: %w", p, path, err)
		}
	}

	return db, nil
}
