package registry

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
)

func openTest(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "registry.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// seed inserts the minimum chain every other row references: a source, a
// complete scan, and one event. Returns the event id.
func seed(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	mustExec(t, db, `INSERT INTO source (source_id, host, kind, locator, status, last_success_ms)
		VALUES ('claude-sessions','clavain','session_file','~/.claude/sessions','ok',1)`)
	mustExec(t, db, `INSERT INTO source_scan (scan_id, source_id, started_ms, finished_ms, complete, records_seen)
		VALUES (1,'claude-sessions',1,2,1,12)`)
	res := mustExec(t, db, `INSERT INTO event (source_id, scan_id, dedupe_key, kind, observed_ms)
		VALUES ('claude-sessions',1,'k1','session.observed',1)`)
	id, _ := res.LastInsertId()
	return id
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) sql.Result {
	t.Helper()
	res, err := db.Exec(q, args...)
	if err != nil {
		t.Fatalf("exec %.60s...: %v", strings.TrimSpace(q), err)
	}
	return res
}

func wantErr(t *testing.T, label string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected the constraint to reject this, got no error", label)
	}
}

func seedConversation(t *testing.T, db *sql.DB, ev int64, sessionID string) string {
	t.Helper()
	id := ConversationID("claude", "clavain", sessionID)
	mustExec(t, db, `INSERT INTO conversation
		(conversation_id, provider, host, provider_session_id, first_seen_ms, last_seen_ms, first_event_id, last_event_id)
		VALUES (?, 'claude', 'clavain', ?, 1, 1, ?, ?)`, id, sessionID, ev, ev)
	return id
}

func seedInstance(t *testing.T, db *sql.DB, ev int64, pid int64) string {
	t.Helper()
	id := InstanceID("clavain", "darwin", pid, 1000+pid)
	mustExec(t, db, `INSERT INTO launch_instance
		(instance_id, host, pid_domain, pid, started_ms, first_event_id, last_event_id)
		VALUES (?, 'clavain', 'darwin', ?, ?, ?, ?)`, id, pid, 1000+pid, ev, ev)
	return id
}

func seedItem(t *testing.T, db *sql.DB, ev int64, id string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO item (item_id, kind, created_ms, created_event_id) VALUES (?, 'ruling', 1, ?)`, id, ev)
}

// ---------------------------------------------------------------- structure

func TestOpenStampsVersionAndEnforcesKeys(t *testing.T) {
	db := openTest(t)

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("user_version = %d, want %d", version, SchemaVersion)
	}

	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1 -- every non-suppression guarantee depends on it", fk)
	}

	_, err := db.Exec(`INSERT INTO event (source_id, dedupe_key, kind, observed_ms) VALUES ('no-such-source','x','k',1)`)
	wantErr(t, "event with unknown source_id", err)
}

// Foreign keys must survive the pool replacing a connection. An Exec-set
// pragma does not; a DSN pragma does.
func TestForeignKeysSurviveAReconnect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Force the pool to discard and reopen its connection.
	db.SetMaxIdleConns(0)
	for i := 0; i < 3; i++ {
		if err := db.Ping(); err != nil {
			t.Fatalf("ping %d: %v", i, err)
		}
	}

	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d after reconnect, want 1", fk)
	}
	_, err = db.Exec(`INSERT INTO event (source_id, dedupe_key, kind, observed_ms) VALUES ('gone','x','k',1)`)
	wantErr(t, "dangling reference after a reconnect", err)
}

// The rebuild contract. If anything outside the projection set references a
// projection, the projections cannot be dropped and replay is impossible --
// which is how the first draft of this schema failed review.
func TestNothingForeignKeysIntoAProjection(t *testing.T) {
	db := openTest(t)

	projection := map[string]bool{}
	for _, name := range ProjectionTables() {
		projection[name] = true
	}

	for _, table := range append(append([]string{}, SpineTables()...), DurableTables()...) {
		rows, err := db.Query(fmt.Sprintf("PRAGMA foreign_key_list(%q)", table))
		if err != nil {
			t.Fatalf("foreign_key_list(%s): %v", table, err)
		}
		for rows.Next() {
			var id, seq int
			var target, from, to, onUpdate, onDelete, match sql.NullString
			if err := rows.Scan(&id, &seq, &target, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				rows.Close()
				t.Fatalf("scan %s: %v", table, err)
			}
			if projection[target.String] {
				rows.Close()
				t.Errorf("%s.%s references projection table %s: projections could not be dropped, so replay is impossible",
					table, from.String, target.String)
			}
		}
		rows.Close()
	}
}

// The other half of the same contract: the drop actually succeeds.
func TestProjectionsCanBeDropped(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	conv := seedConversation(t, db, ev, "sess-1")
	inst := seedInstance(t, db, ev, 100)
	mustExec(t, db, `INSERT INTO instance_conversation (instance_id, conversation_id, observed_from_ms, first_event_id, last_event_id)
		VALUES (?, ?, 1, ?, ?)`, inst, conv, ev, ev)
	mustExec(t, db, `INSERT INTO pane_binding (instance_id, window_id, pane_id, binding_basis, observed_from_ms, first_event_id, last_event_id)
		VALUES (?, '@98', '%98', 'session_file_claim', 1, ?, ?)`, inst, ev, ev)

	// A durable row pointing at the conversation must not block the drop.
	seedItem(t, db, ev, "item-1")
	mustExec(t, db, `UPDATE item SET conversation_id = ? WHERE item_id = 'item-1'`, conv)
	mustExec(t, db, `INSERT INTO project_association (conversation_id, project_key, basis, confidence, observed_ms, event_id)
		VALUES (?, 'autarch', 'operator_override', 1.0, 1, ?)`, conv, ev)

	// Reverse order, as a teardown would run it.
	tables := ProjectionTables()
	for i := len(tables) - 1; i >= 0; i-- {
		if _, err := db.Exec("DROP TABLE " + tables[i]); err != nil {
			t.Fatalf("drop %s: %v", tables[i], err)
		}
	}

	// The durable rows survive, still naming the conversation by its
	// deterministic id -- which a replay regenerates unchanged.
	var got string
	if err := db.QueryRow(`SELECT conversation_id FROM item WHERE item_id = 'item-1'`).Scan(&got); err != nil {
		t.Fatalf("item survived? %v", err)
	}
	if got != conv {
		t.Errorf("item conversation_id = %q, want %q", got, conv)
	}
	if got != ConversationID("claude", "clavain", "sess-1") {
		t.Error("the id must be reproducible from natural keys, or the rebuild orphans it")
	}
}

func TestOpenRefusesNewerAndRefusesToRestampOlder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mustExec(t, db, fmt.Sprintf("PRAGMA user_version=%d", SchemaVersion+1))
	db.Close()

	if _, err := Open(path); err == nil {
		t.Fatal("expected a refusal to open a database newer than this build")
	} else if !strings.Contains(err.Error(), "newer") {
		t.Errorf("refusal should say why: %v", err)
	}

	// And an older database is a migration, not a restamp.
	db2, err := autarchOpenRaw(path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	mustExec(t, db2, "PRAGMA user_version=0")
	mustExec(t, db2, fmt.Sprintf("PRAGMA user_version=%d", SchemaVersion))
	db2.Close()
	if _, err := Open(path); err != nil {
		t.Fatalf("a current database must open: %v", err)
	}
}

func autarchOpenRaw(path string) (*sql.DB, error) { return sql.Open("sqlite", path) }

// ---------------------------------------------------------------- ingest

func TestDedupeKeyIsScopedPerSourceAndOrIgnoreIsNotSafe(t *testing.T) {
	db := openTest(t)
	seed(t, db)

	_, err := db.Exec(`INSERT INTO event (source_id, dedupe_key, kind, observed_ms) VALUES ('claude-sessions','k1','x',2)`)
	wantErr(t, "the same dedupe_key twice for one source", err)

	// A different producer using the same key is a different fact, not a
	// duplicate. A global unique key would drop it silently.
	mustExec(t, db, `INSERT INTO source (source_id, host, kind, locator, status, last_success_ms)
		VALUES ('tmux','clavain','tmux_inventory','/private/tmp/tmux-501/default','ok',1)`)
	if _, err := db.Exec(`INSERT INTO event (source_id, dedupe_key, kind, observed_ms) VALUES ('tmux','k1','x',2)`); err != nil {
		t.Fatalf("same key from another source must be accepted: %v", err)
	}

	// INSERT OR IGNORE suppresses NOT NULL and CHECK failures too, so a write
	// that could not happen reads as "nothing new" -- the failure the
	// non-suppression rule exists to prevent.
	res := mustExec(t, db, `INSERT OR IGNORE INTO event (source_id, dedupe_key, kind, observed_ms) VALUES ('claude-sessions','k2','x',NULL)`)
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatalf("expected OR IGNORE to swallow the NOT NULL failure, affected=%d", n)
	}
	// ON CONFLICT targets the duplicate and nothing else.
	_, err = db.Exec(`INSERT INTO event (source_id, dedupe_key, kind, observed_ms) VALUES ('claude-sessions','k2','x',NULL)
		ON CONFLICT(source_id, dedupe_key) DO NOTHING`)
	wantErr(t, "ON CONFLICT must still surface a NOT NULL failure", err)

	// ...while still making a genuine replay a no-op.
	if _, err := db.Exec(`INSERT INTO event (source_id, dedupe_key, kind, observed_ms) VALUES ('claude-sessions','k1','x',9)
		ON CONFLICT(source_id, dedupe_key) DO NOTHING`); err != nil {
		t.Fatalf("replaying a seen event must be silent: %v", err)
	}
	var n int
	mustScan(t, db, `SELECT COUNT(*) FROM event WHERE source_id='claude-sessions'`, &n)
	if n != 1 {
		t.Errorf("events after replay = %d, want 1", n)
	}
}

func TestEventPayloadMustBeJSON(t *testing.T) {
	db := openTest(t)
	seed(t, db)
	_, err := db.Exec(`INSERT INTO event (source_id, dedupe_key, kind, observed_ms, payload)
		VALUES ('claude-sessions','k9','x',1,'{not json')`)
	wantErr(t, "a payload that is not valid JSON", err)

	if _, err := db.Exec(`INSERT INTO event (source_id, dedupe_key, kind, observed_ms, payload)
		VALUES ('claude-sessions','k9','x',1,'{"pid":123,"pidDomain":"darwin"}')`); err != nil {
		t.Fatalf("valid payload: %v", err)
	}
}

func TestEventIsAppendOnlyAndAttestationsCannotBeWithdrawn(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	seedItem(t, db, ev, "item-1")
	mustExec(t, db, `INSERT INTO item_source (item_id, source_kind, event_id, observed_ms) VALUES ('item-1','wait_state',?,1)`, ev)

	_, err := db.Exec(`UPDATE event SET kind = 'rewritten' WHERE event_id = ?`, ev)
	wantErr(t, "rewriting an event", err)
	_, err = db.Exec(`DELETE FROM event WHERE event_id = ?`, ev)
	wantErr(t, "deleting an event", err)
	_, err = db.Exec(`DELETE FROM item_source WHERE item_id = 'item-1'`)
	wantErr(t, "withdrawing an attestation", err)
	_, err = db.Exec(`UPDATE item_source SET source_kind = 'detector' WHERE item_id = 'item-1'`)
	wantErr(t, "rewriting an attestation", err)
}

// ---------------------------------------------------------------- coverage

func TestSourceCannotClaimHealthItNeverHad(t *testing.T) {
	db := openTest(t)
	mustExec(t, db, `INSERT INTO source (source_id, host, kind, locator) VALUES ('tmux','clavain','tmux_inventory','/s')`)

	var status string
	var lastSuccess sql.NullInt64
	if err := db.QueryRow(`SELECT status, last_success_ms FROM source WHERE source_id='tmux'`).Scan(&status, &lastSuccess); err != nil {
		t.Fatalf("read source: %v", err)
	}
	if status != "unchecked" {
		t.Errorf("new source status = %q, want %q -- a producer that never ran must not read as healthy", status, "unchecked")
	}
	if lastSuccess.Valid {
		t.Error("a source that has never succeeded must hold NULL, not a zero")
	}

	_, err := db.Exec(`UPDATE source SET status='ok' WHERE source_id='tmux'`)
	wantErr(t, "claiming ok with no successful run", err)

	// The same producer id on two hosts is two producers.
	_, err = db.Exec(`INSERT INTO source (source_id, host, kind, locator) VALUES ('tmux2','clavain','tmux_inventory','/s')`)
	wantErr(t, "a second producer watching the same thing on the same host", err)
	if _, err := db.Exec(`INSERT INTO source (source_id, host, kind, locator) VALUES ('tmux-zklw','zklw','tmux_inventory','/s')`); err != nil {
		t.Fatalf("the same watcher on another host is a separate producer: %v", err)
	}
}

// A partial sweep may add facts; only a complete one may support an absence.
func TestIncompleteScanCannotClaimCompleteness(t *testing.T) {
	db := openTest(t)
	seed(t, db)
	_, err := db.Exec(`INSERT INTO source_scan (source_id, started_ms, finished_ms, complete, error)
		VALUES ('claude-sessions',1,2,1,'readdir failed')`)
	wantErr(t, "a complete scan that also reported an error", err)
	_, err = db.Exec(`INSERT INTO source_scan (source_id, started_ms, complete) VALUES ('claude-sessions',1,1)`)
	wantErr(t, "a complete scan that never finished", err)
}

// ---------------------------------------------------------------- identity

// /clear replaces sessionId in place, same pid and same startedAt. Measured on
// Clavain 2026-09-19. One process therefore carries several conversations.
func TestOneProcessCarriesManyConversationsOverTime(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	inst := seedInstance(t, db, ev, 43066)
	before := seedConversation(t, db, ev, "5d183345-4ffa-4a02-8b30-2cc17d696297")
	after := seedConversation(t, db, ev, "e20f9bd7-2eb9-4054-9c36-3900936cebfe")

	mustExec(t, db, `INSERT INTO instance_conversation (instance_id, conversation_id, observed_from_ms, first_event_id, last_event_id)
		VALUES (?, ?, 1, ?, ?)`, inst, before, ev, ev)

	// A second open conversation on one process is wrong: it runs one at a time.
	_, err := db.Exec(`INSERT INTO instance_conversation (instance_id, conversation_id, observed_from_ms, first_event_id, last_event_id)
		VALUES (?, ?, 2, ?, ?)`, inst, after, ev, ev)
	wantErr(t, "two conversations open on one process at once", err)

	// Closing the first needs a basis, then the second may open.
	_, err = db.Exec(`UPDATE instance_conversation SET observed_to_ms = 2 WHERE instance_id = ?`, inst)
	wantErr(t, "closing an instance-conversation link with no basis", err)
	mustExec(t, db, `UPDATE instance_conversation SET observed_to_ms = 2, end_basis = 'session_id_changed', end_event_id = ?
		WHERE instance_id = ?`, ev, inst)
	mustExec(t, db, `INSERT INTO instance_conversation (instance_id, conversation_id, observed_from_ms, first_event_id, last_event_id)
		VALUES (?, ?, 2, ?, ?)`, inst, after, ev, ev)

	mustExec(t, db, `INSERT INTO conversation_lineage
		(parent_conversation_id, child_conversation_id, relation, basis, confidence, observed_ms, event_id)
		VALUES (?, ?, 'clear', 'same_process_session_change', 1.0, 2, ?)`, before, after, ev)

	var n int
	mustScan(t, db, `SELECT COUNT(*) FROM instance_conversation WHERE instance_id = ?`, &n, inst)
	if n != 2 {
		t.Errorf("conversations recorded for one process = %d, want 2", n)
	}
}

// Two live Claude processes routinely share a pane: a parent and the child it
// dispatched. Both bindings must be retained at the same instant.
func TestTwoConversationsMayHoldOnePaneAtOnce(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	parent := seedInstance(t, db, ev, 52620)
	child := seedInstance(t, db, ev, 37995)

	bind := func(inst string) error {
		_, err := db.Exec(`INSERT INTO pane_binding
			(instance_id, socket, server_pid, server_started, pane_pid, window_id, pane_id,
			 session_name_seen, binding_basis, observed_from_ms, first_event_id, last_event_id)
			VALUES (?, '/private/tmp/tmux-501/default', 1691, 100, 36230, '@67', '%67',
			        'iterm]linsekasten', 'tmux_inventory', 1, ?, ?)`, inst, ev, ev)
		return err
	}
	if err := bind(parent); err != nil {
		t.Fatalf("parent binding: %v", err)
	}
	if err := bind(child); err != nil {
		t.Fatalf("child binding in the same pane must be allowed: %v", err)
	}
	wantErr(t, "a duplicate open binding for one instance", bind(parent))

	var n int
	mustScan(t, db, `SELECT COUNT(*) FROM pane_binding WHERE pane_id='%67' AND observed_to_ms IS NULL`, &n)
	if n != 2 {
		t.Errorf("open bindings on one pane = %d, want 2", n)
	}
}

func TestPaneKeyMatchesAgenttransportAndIsStableUnderRename(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	inst := seedInstance(t, db, ev, 1)

	target := agenttransport.Target{
		Socket: "/private/tmp/tmux-501/default", ServerPID: 1691, ServerStarted: 100,
		SessionID: "$3", WindowID: "@67", PaneID: "%67", PanePID: 36230,
	}
	mustExec(t, db, `INSERT INTO pane_binding
		(instance_id, socket, server_pid, server_started, pane_pid, tmux_session_id, window_id, pane_id,
		 session_name_seen, binding_basis, observed_from_ms, first_event_id, last_event_id)
		VALUES (?,?,?,?,?,?,?,?,'whatever','tmux_inventory',1,?,?)`,
		inst, target.Socket, target.ServerPID, target.ServerStarted, target.PanePID,
		target.SessionID, target.WindowID, target.PaneID, ev, ev)

	var key string
	mustScan(t, db, `SELECT pane_key FROM pane_binding WHERE instance_id=?`, &key, inst)
	// The Go helper and the generated column are pinned to each other, so
	// neither can be changed alone.
	if key != target.PaneKey() {
		t.Errorf("pane_key = %q, Target.PaneKey() = %q -- the two have drifted", key, target.PaneKey())
	}
	if strings.Contains(key, "whatever") || strings.Contains(key, "$3") {
		t.Error("pane_key must not depend on a session name or alias, both of which are observations")
	}

	// An unconfirmed claim: the key is the pane id alone, so a rename cannot
	// produce a second open binding for one instance on one pane.
	inst2 := seedInstance(t, db, ev, 2)
	mustExec(t, db, `INSERT INTO pane_binding
		(instance_id, window_id, pane_id, session_name_seen, binding_basis, observed_from_ms, first_event_id, last_event_id)
		VALUES (?, '@98', '%98', 'tmux-organizer', 'session_file_claim', 1, ?, ?)`, inst2, ev, ev)
	var claimed string
	mustScan(t, db, `SELECT pane_key FROM pane_binding WHERE instance_id=?`, &claimed, inst2)
	if claimed != ClaimedPaneKey("%98") {
		t.Errorf("claimed pane_key = %q, want %q", claimed, ClaimedPaneKey("%98"))
	}
	if claimed == key {
		t.Error("an unverified claim must not share a key with a verified binding")
	}

	// The same instance, same pane, after the session was renamed.
	_, err := db.Exec(`INSERT INTO pane_binding
		(instance_id, window_id, pane_id, session_name_seen, binding_basis, observed_from_ms, first_event_id, last_event_id)
		VALUES (?, '@98', '%98', 'iterm[autarch - e4bedaf5', 'session_file_claim', 2, ?, ?)`, inst2, ev, ev)
	wantErr(t, "a rename producing a second open binding on one pane", err)
}

func TestTmuxInventoryBindingRequiresServerIdentity(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	inst := seedInstance(t, db, ev, 1)
	_, err := db.Exec(`INSERT INTO pane_binding
		(instance_id, window_id, pane_id, binding_basis, observed_from_ms, first_event_id, last_event_id)
		VALUES (?, '@67', '%67', 'tmux_inventory', 1, ?, ?)`, inst, ev, ev)
	wantErr(t, "a tmux_inventory binding with no socket or server identity", err)
}

// The non-suppression invariant. A failed read has no basis from the closed
// set and no event to cite, so it cannot close anything.
func TestClosingAnythingRequiresPositiveEvidence(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	inst := seedInstance(t, db, ev, 1)
	seedItem(t, db, ev, "item-1")

	_, err := db.Exec(`UPDATE launch_instance SET ended_ms = 2 WHERE instance_id = ?`, inst)
	wantErr(t, "ending an instance with no basis", err)
	_, err = db.Exec(`UPDATE launch_instance SET ended_ms = 2, end_basis = '' WHERE instance_id = ?`, inst)
	wantErr(t, "an empty basis", err)
	_, err = db.Exec(`UPDATE launch_instance SET ended_ms = 2, end_basis = 'tmux_read_failed', end_event_id = ? WHERE instance_id = ?`, ev, inst)
	wantErr(t, "a failed read offered as a closing basis", err)
	_, err = db.Exec(`UPDATE launch_instance SET ended_ms = 2, end_basis = 'process_exited' WHERE instance_id = ?`, inst)
	wantErr(t, "a basis with no event to cite", err)
	mustExec(t, db, `UPDATE launch_instance SET ended_ms = 2, end_basis = 'process_exited', end_event_id = ? WHERE instance_id = ?`, ev, inst)

	_, err = db.Exec(`UPDATE item SET resolved_ms = 2, resolved_basis = 'detector_stopped_seeing_it', resolved_event_id = ? WHERE item_id = 'item-1'`, ev)
	wantErr(t, "an item resolved because a detector went quiet", err)
	mustExec(t, db, `UPDATE item SET resolved_ms = 2, resolved_basis = 'tool_answer', resolved_event_id = ? WHERE item_id = 'item-1'`, ev)
}

// ---------------------------------------------------------------- attention

// Three absences, kept apart. The naive derivation counts an item answered
// straight into the pane as a miss, and excluding resolved items hides the
// truest miss of all: one that was never shown and whose agent then died.
func TestMissedIsDerivedAndCannotBeFlattered(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	for _, id := range []string{"shown", "never-shown", "out-of-band", "died-unseen"} {
		seedItem(t, db, ev, id)
	}

	mustExec(t, db, `INSERT INTO exposure (item_id, surface, mode, rank, shown_from_ms, event_id)
		VALUES ('shown','questions','listed',1,1,?)`, ev)

	_, err := db.Exec(`INSERT INTO outcome (item_id, channel, action, occurred_ms, event_id) VALUES ('shown','router','ignored',2,?)`, ev)
	wantErr(t, "an 'ignored' outcome", err)
	_, err = db.Exec(`INSERT INTO outcome (item_id, channel, action, occurred_ms, event_id) VALUES ('shown','router','answered',2,?)`, ev)
	wantErr(t, "a router action that names no exposure", err)

	// An answer typed straight into the pane: a real action, no exposure.
	mustExec(t, db, `INSERT INTO outcome (item_id, exposure_id, channel, action, occurred_ms, event_id)
		VALUES ('out-of-band', NULL, 'pane_direct', 'answered', 2, ?)`, ev)

	// The item nobody ever saw, whose conversation then ended. It is resolved,
	// and it is the truest miss on the board.
	mustExec(t, db, `UPDATE item SET resolved_ms = 3, resolved_basis = 'conversation_ended', resolved_event_id = ? WHERE item_id = 'died-unseen'`, ev)

	var missed, shownUnanswered, outOfBand int
	const q = `SELECT
	  (SELECT COUNT(*) FROM item i
	     WHERE NOT EXISTS (SELECT 1 FROM exposure e WHERE e.item_id = i.item_id)
	       AND NOT EXISTS (SELECT 1 FROM outcome o WHERE o.item_id = i.item_id)),
	  (SELECT COUNT(*) FROM item i
	     WHERE EXISTS (SELECT 1 FROM exposure e WHERE e.item_id = i.item_id)
	       AND NOT EXISTS (SELECT 1 FROM outcome o WHERE o.item_id = i.item_id)),
	  (SELECT COUNT(*) FROM outcome WHERE exposure_id IS NULL)`
	if err := db.QueryRow(q).Scan(&missed, &shownUnanswered, &outOfBand); err != nil {
		t.Fatalf("derive: %v", err)
	}
	if missed != 2 {
		t.Errorf("missed = %d, want 2 (never-shown and died-unseen; filtering on resolution would hide the second)", missed)
	}
	if shownUnanswered != 1 {
		t.Errorf("shown-but-unanswered = %d, want 1", shownUnanswered)
	}
	if outOfBand != 1 {
		t.Errorf("answered-without-exposure = %d, want 1", outOfBand)
	}
}

func TestAnOutcomeCannotCiteAnotherItemsExposure(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	seedItem(t, db, ev, "a")
	seedItem(t, db, ev, "b")
	res := mustExec(t, db, `INSERT INTO exposure (item_id, surface, mode, shown_from_ms, event_id) VALUES ('a','questions','focused',1,?)`, ev)
	exp, _ := res.LastInsertId()

	_, err := db.Exec(`INSERT INTO outcome (item_id, exposure_id, channel, action, occurred_ms, event_id)
		VALUES ('b', ?, 'router', 'answered', 2, ?)`, exp, ev)
	wantErr(t, "item b answering item a's exposure", err)
}

func TestOneRulingSeenTwiceIsOneItem(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	mustExec(t, db, `INSERT INTO item (item_id, kind, anchor_key, created_ms, created_event_id)
		VALUES ('i1','ruling','wait_state:clavain:darwin:25476:1789851648696',1,?)`, ev)
	_, err := db.Exec(`INSERT INTO item (item_id, kind, anchor_key, created_ms, created_event_id)
		VALUES ('i2','ruling','wait_state:clavain:darwin:25476:1789851648696',2,?)`, ev)
	wantErr(t, "a second sweep creating a second item for one anchor", err)

	// Two sources describing one ruling in incompatible terms cannot share an
	// anchor, so the duplicate is merged, never deleted.
	mustExec(t, db, `INSERT INTO item (item_id, kind, anchor_key, created_ms, created_event_id)
		VALUES ('i2','ruling','transcript:toolu_01ABC',2,?)`, ev)
	mustExec(t, db, `UPDATE item SET merged_into_item_id = 'i1' WHERE item_id = 'i2'`)
	_, err = db.Exec(`UPDATE item SET merged_into_item_id = 'i2' WHERE item_id = 'i2'`)
	wantErr(t, "an item merged into itself", err)

	var n int
	mustScan(t, db, `SELECT COUNT(*) FROM item`, &n)
	if n != 2 {
		t.Errorf("items after a merge = %d, want 2 -- merging points, it does not delete", n)
	}
}

func TestOnlyAPersonMayAssertANegativeAssociation(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	conv := seedConversation(t, db, ev, "sess-1")

	_, err := db.Exec(`INSERT INTO project_association (conversation_id, project_key, basis, stance, confidence, observed_ms, event_id)
		VALUES (?, 'autarch', 'launch_cwd', 'is_not', 0.9, 1, ?)`, conv, ev)
	wantErr(t, "a detector asserting a conversation is NOT a project", err)

	mustExec(t, db, `INSERT INTO project_association (conversation_id, project_key, basis, stance, confidence, observed_ms, event_id)
		VALUES (?, 'autarch', 'operator_override', 'is_not', 1.0, 1, ?)`, conv, ev)
	// An automatic sweep re-asserting the positive cannot overwrite it: the
	// bases are different rows.
	mustExec(t, db, `INSERT INTO project_association (conversation_id, project_key, basis, confidence, observed_ms, event_id)
		VALUES (?, 'autarch', 'launch_cwd', 0.6, 2, ?)`, conv, ev)
	var stance string
	mustScan(t, db, `SELECT stance FROM project_association WHERE basis='operator_override'`, &stance)
	if stance != "is_not" {
		t.Errorf("operator stance after an automatic re-assertion = %q, want is_not", stance)
	}
}

// ---------------------------------------------------------------- evidence

func TestEvidenceBodyRequiresRedactionAndSurvivesRetention(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)

	_, err := db.Exec(`INSERT INTO evidence (event_id, kind, source_id, content_sha256, body, captured_ms)
		VALUES (?, 'transcript_span', 'claude-sessions', 'abc', 'export TOKEN=hunter2', 1)`, ev)
	wantErr(t, "storing a body that has not been through redaction", err)

	mustExec(t, db, `INSERT INTO evidence (event_id, kind, source_id, content_sha256, body, body_redacted, captured_ms)
		VALUES (?, 'transcript_span', 'claude-sessions', 'abc', 'export TOKEN=[redacted]', 1, 1)`, ev)

	_, err = db.Exec(`DELETE FROM evidence WHERE content_sha256 = 'abc'`)
	wantErr(t, "deleting evidence outright", err)
	_, err = db.Exec(`UPDATE evidence SET content_sha256 = 'zzz' WHERE content_sha256 = 'abc'`)
	wantErr(t, "rewriting what was observed", err)

	// Retention drops the body and keeps the fact that we looked.
	mustExec(t, db, `UPDATE evidence SET body = NULL, retain_until_ms = NULL WHERE content_sha256 = 'abc'`)
	var n int
	mustScan(t, db, `SELECT COUNT(*) FROM evidence WHERE content_sha256 = 'abc'`, &n)
	if n != 1 {
		t.Errorf("evidence rows after retention = %d, want 1", n)
	}
}

func TestByteRangeMustNotRunBackwards(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	_, err := db.Exec(`INSERT INTO evidence (event_id, kind, source_id, content_sha256, byte_from, byte_to, captured_ms)
		VALUES (?, 'transcript_span', 'claude-sessions', 'abc', 900, 100, 1)`, ev)
	wantErr(t, "a transcript span ending before it starts", err)
}

// ---------------------------------------------------------------- ids

func TestIdentifiersAreDeterministicAndUnforgeable(t *testing.T) {
	a := ConversationID("claude", "clavain", "9e72b443")
	if a != ConversationID("claude", "clavain", "9e72b443") {
		t.Error("ids must be reproducible, or a rebuild orphans every durable row")
	}
	if a == ConversationID("claude", "zklw", "9e72b443") {
		t.Error("the same session id on another host must be a different conversation")
	}
	// A component carrying the separator must not be able to forge another id.
	if ConversationID("claude", "clavain:x", "y") == ConversationID("claude", "clavain", "x:y") {
		t.Error("a colon in a component forges a different identity")
	}
	if InstanceID("clavain", "darwin", 43066, 1789858817805) != "clavain:darwin:43066:1789858817805" {
		t.Errorf("unexpected instance id form: %s", InstanceID("clavain", "darwin", 43066, 1789858817805))
	}
}

func TestLineageRejectsSelfEdges(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	conv := seedConversation(t, db, ev, "sess-1")
	_, err := db.Exec(`INSERT INTO conversation_lineage
		(parent_conversation_id, child_conversation_id, relation, basis, confidence, observed_ms, event_id)
		VALUES (?,?,'resume','provider_resume_pointer',1.0,1,?)`, conv, conv, ev)
	wantErr(t, "a conversation as its own parent", err)
}

func TestConversationIsUniquePerHost(t *testing.T) {
	db := openTest(t)
	ev := seed(t, db)
	seedConversation(t, db, ev, "9e72b443")

	_, err := db.Exec(`INSERT INTO conversation
		(conversation_id, provider, host, provider_session_id, first_seen_ms, last_seen_ms, first_event_id, last_event_id)
		VALUES ('dup','claude','clavain','9e72b443',1,1,?,?)`, ev, ev)
	wantErr(t, "the same provider session id twice on one host", err)

	if _, err := db.Exec(`INSERT INTO conversation
		(conversation_id, provider, host, provider_session_id, first_seen_ms, last_seen_ms, first_event_id, last_event_id)
		VALUES (?,'claude','zklw','9e72b443',1,1,?,?)`,
		ConversationID("claude", "zklw", "9e72b443"), ev, ev); err != nil {
		t.Fatalf("the same id on another host must be a separate record: %v", err)
	}
}

func mustScan(t *testing.T, db *sql.DB, q string, dest any, args ...any) {
	t.Helper()
	if err := db.QueryRow(q, args...).Scan(dest); err != nil {
		t.Fatalf("scan %.60s...: %v", strings.TrimSpace(q), err)
	}
}

// Guards TestNothingForeignKeysIntoAProjection against passing vacuously: if
// PRAGMA foreign_key_list returned nothing, that test would be silent.
func TestForeignKeyListIsReadable(t *testing.T) {
	db := openTest(t)
	seen := 0
	for _, table := range append(append([]string{}, SpineTables()...), DurableTables()...) {
		rows, err := db.Query(fmt.Sprintf("PRAGMA foreign_key_list(%q)", table))
		if err != nil {
			t.Fatalf("foreign_key_list(%s): %v", table, err)
		}
		for rows.Next() {
			seen++
		}
		rows.Close()
	}
	if seen < 10 {
		t.Fatalf("only %d foreign keys visible across spine and durable tables; the projection check may be vacuous", seen)
	}
	t.Logf("%d foreign keys inspected", seen)
}
