package mycroft

import (
	"path/filepath"
	"testing"
	"time"
)

func TestOpenDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.db")

	db, err := OpenDB(path)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	// Verify all tables exist by inserting test data.
	now := time.Now().Unix()

	// dispatch_log
	_, err = db.Exec(`INSERT INTO dispatch_log (ts, agent, bead, action) VALUES (?, ?, ?, ?)`,
		now, "grey-area", "Demarch-42", "shadow_suggest")
	if err != nil {
		t.Fatalf("insert dispatch_log: %v", err)
	}

	// tier_state
	_, err = db.Exec(`INSERT INTO tier_state (key, value) VALUES (?, ?)`,
		"current_tier", "0")
	if err != nil {
		t.Fatalf("insert tier_state: %v", err)
	}

	// tier_transitions
	_, err = db.Exec(`INSERT INTO tier_transitions (ts, from_tier, to_tier, trigger) VALUES (?, ?, ?, ?)`,
		now, 0, 1, "manual")
	if err != nil {
		t.Fatalf("insert tier_transitions: %v", err)
	}

	// recovery_log
	_, err = db.Exec(`INSERT INTO recovery_log (ts, agent, bead, action, status) VALUES (?, ?, ?, ?, ?)`,
		now, "grey-area", "Demarch-42", "patch_save", "completed")
	if err != nil {
		t.Fatalf("insert recovery_log: %v", err)
	}

	// Verify reads work.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM dispatch_log").Scan(&count)
	if err != nil {
		t.Fatalf("count dispatch_log: %v", err)
	}
	if count != 1 {
		t.Errorf("dispatch_log count: got %d, want 1", count)
	}
}

func TestOpenDBIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.db")

	// Open twice — should not error on CREATE IF NOT EXISTS.
	db1, err := OpenDB(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	db1.Close()

	db2, err := OpenDB(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	db2.Close()
}

func TestDispatchLogFields(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "decisions.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()
	_, err = db.Exec(`INSERT INTO dispatch_log
		(ts, project, agent, bead, action, outcome, reason, context, cost_actual)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		now, "demarch", "mistake-not", "Demarch-99", "suggest",
		"accepted", "looks good", `{"tier_at_time":1}`, 1.23)
	if err != nil {
		t.Fatalf("insert full row: %v", err)
	}

	var reason, outcome string
	var cost float64
	err = db.QueryRow("SELECT outcome, reason, cost_actual FROM dispatch_log WHERE bead = ?",
		"Demarch-99").Scan(&outcome, &reason, &cost)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if outcome != "accepted" {
		t.Errorf("outcome: got %q, want %q", outcome, "accepted")
	}
	if reason != "looks good" {
		t.Errorf("reason: got %q, want %q", reason, "looks good")
	}
	if cost != 1.23 {
		t.Errorf("cost: got %f, want 1.23", cost)
	}
}
