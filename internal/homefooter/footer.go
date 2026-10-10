// Package homefooter reads Home's data.db, read-only, and prints the footer a vizier reply carries:
// what is open for mk, what is parked, which picks are new since the caller last looked, and which
// delivery obligations are still owed. It never writes to Home; its only write is the caller's own
// cursor file.
package homefooter

import (
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver
)

const epoch = "1970-01-01T00:00:00Z"

// stampLayout matches the picked_at values Home stores (UTC, milliseconds).
const stampLayout = "2006-01-02T15:04:05.000Z"

// Options select the database, the caller's cursor and whether the cursor moves.
type Options struct {
	// DB is the path of Home's data.db.
	DB string
	// Cursor is the caller's cursor file. Empty means no cursor: every pick is "new" and nothing is written.
	Cursor string
	// Peek leaves the cursor where it is.
	Peek bool
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// DefaultDB finds Home's database: $AUTARCH_HOME_DB, else $HOME_BB_DATA/plugins/autarch/data.db.
func DefaultDB() (string, error) {
	if p := os.Getenv("AUTARCH_HOME_DB"); p != "" {
		return p, nil
	}
	if d := os.Getenv("HOME_BB_DATA"); d != "" {
		return filepath.Join(d, "plugins", "autarch", "data.db"), nil
	}
	return "", fmt.Errorf("no Home data.db known: set AUTARCH_HOME_DB or HOME_BB_DATA, or pass --db")
}

// Print writes the footer to w.
func Print(w io.Writer, o Options) error {
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	// Taken before any read, so a pick that lands while the footer is built is shown next time, not lost. It is one
	// millisecond early: picked_at has millisecond precision and is compared with >, so a pick committed in the
	// same millisecond as the start is shown again rather than skipped.
	start := now().UTC().Add(-time.Millisecond).Format(stampLayout)
	since := epoch
	if o.Cursor != "" {
		if b, err := os.ReadFile(o.Cursor); err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				since = s
			}
		}
	}

	if _, err := os.Stat(o.DB); err != nil {
		return fmt.Errorf("home database: %w", err)
	}
	q := url.Values{}
	q.Set("mode", "ro")
	q.Set("_pragma", "busy_timeout(5000)")
	db, err := sql.Open("sqlite", "file:"+(&url.URL{Path: o.DB}).EscapedPath()+"?"+q.Encode())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	later, err := laterTasks(db)
	if err != nil {
		return err
	}
	open, parked, err := cards(db, later)
	if err != nil {
		return err
	}
	picks, err := newPicks(db, since)
	if err != nil {
		return err
	}
	owed, err := obligations(db)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "To decide (Home): %s\n", strings.Join(open, "; "))
	fmt.Fprintf(w, "Parked (Later): %s\n", strings.Join(parked, " "))
	fmt.Fprintf(w, "New picks since %s:\n", since)
	for _, l := range picks {
		fmt.Fprintln(w, l)
	}
	fmt.Fprintln(w, "Undelivered obligations:")
	for _, l := range owed {
		fmt.Fprintln(w, l)
	}

	if o.Peek || o.Cursor == "" {
		return nil
	}
	return writeCursor(o.Cursor, start)
}

// laterTasks is the set of task ids mk set aside: the keys of the JSON object in settings_kv 'later'.
func laterTasks(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT key FROM json_each(COALESCE((SELECT value FROM settings_kv WHERE key = 'later'), '{}'))`)
	if err != nil {
		return nil, fmt.Errorf("read later: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}

func cards(db *sql.DB, later map[string]bool) (open, parked []string, err error) {
	rows, err := db.Query(`SELECT task_id, COALESCE(card_key, ''), state, COALESCE(title, '') FROM cards
		WHERE state IN ('open', 'ruled') AND deleted_at IS NULL ORDER BY card_key`)
	if err != nil {
		return nil, nil, fmt.Errorf("read cards: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, key, state, title string
		if err := rows.Scan(&id, &key, &state, &title); err != nil {
			return nil, nil, err
		}
		switch {
		case later[id]:
			parked = append(parked, key)
		case state == "open":
			open = append(open, fmt.Sprintf("%s (%s)", key, truncate(title, 60)))
		}
	}
	return open, parked, rows.Err()
}

func newPicks(db *sql.DB, since string) ([]string, error) {
	rows, err := db.Query(`SELECT c.card_key, p.option_id, p.picked_at, COALESCE(p.reason, '')
		FROM picks p JOIN cards c ON p.decision_id LIKE 'card-' || c.task_id || '-g%'
		WHERE p.picked_at > ? ORDER BY p.picked_at`, since)
	if err != nil {
		return nil, fmt.Errorf("read picks: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var key, opt, at, reason string
		if err := rows.Scan(&key, &opt, &at, &reason); err != nil {
			return nil, err
		}
		l := fmt.Sprintf("  %s -> %s at %s", key, opt, at)
		if reason != "" {
			l += " | " + truncate(strings.ReplaceAll(reason, "\n", " "), 120)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func obligations(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT o.kind, o.state, COALESCE(o.recipient, '-'), COALESCE(c.card_key, o.decision_id), COALESCE(o.last_error, '')
		FROM obligations o LEFT JOIN cards c ON o.decision_id LIKE 'card-' || c.task_id || '-g%'
		WHERE o.state NOT IN ('done', 'dismissed') AND o.voided_at IS NULL ORDER BY o.rowid`)
	if err != nil {
		return nil, fmt.Errorf("read obligations: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var kind, state, to, ref, lastErr string
		if err := rows.Scan(&kind, &state, &to, &ref, &lastErr); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("  %s %s -> %s (%s) %s", kind, state, to, ref, lastErr))
	}
	return out, rows.Err()
}

// truncate cuts to n characters (not bytes), as sqlite's substr does.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// writeCursor replaces the cursor file atomically, creating its directory.
func writeCursor(path, stamp string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cursor-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(stamp + "\n"); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
