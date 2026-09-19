// Package db provides a unified SQLite open helper for Autarch tools.
// It enforces WAL mode, NORMAL synchronous, busy timeout, and connection
// pool limits as best practices for embedded SQLite usage.
package db

import (
	"database/sql"
	"fmt"
	"strings"

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

	// Executed rather than passed in the DSN, which OpenWith uses instead.
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

// OpenWith opens a SQLite database with the same hardening as Open, plus any
// extra pragmas, applied through the DSN rather than executed on one
// connection.
//
// The distinction matters for pragmas that are connection state rather than
// database state. foreign_keys is the load-bearing example: an Exec sets it on
// whichever connection happens to be current, and a reconnect after a dropped
// connection silently brings the replacement up with enforcement off. A DSN
// pragma is reapplied to every connection the pool ever opens.
//
// Each pragma is written in SQLite's DSN form, e.g. "foreign_keys(1)".
func OpenWith(path string, pragmas ...string) (*sql.DB, error) {
	dsn := path + "?_pragma=" + strings.Join(append([]string{
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
		"busy_timeout(5000)",
	}, pragmas...), "&_pragma=")

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	// sql.Open is lazy; force a connection so a bad path fails here rather
	// than at the caller's first query, matching Open's behaviour.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	return db, nil
}
