package registry

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The projector turns the event log into the identity tables. It is the only
// writer of those tables, it reads events in event_id order, and it records
// how far it got -- so the projections can be dropped and rebuilt from the log
// alone.
//
// It derives; it does not observe. Nothing here consults the clock, the
// process table or the live server: every fact it needs was recorded by a
// producer at observation time and travels in the event. A projector that
// probed during replay would judge the past by the present, and an agent whose
// closure was once withheld would be closed retroactively while still running.
//
// One exception, and it is bounded: an attribution attempt is appended to the
// log under its own producer with a content-derived key, so a replay
// re-derives the same reason and inserts nothing new.

const projectionName = "registry"

// SourceAttribution is the producer id for reasons the projector derives.
const SourceAttribution = "registry-attribution"

// execer is the shared surface of *sql.DB and *sql.Tx, so every write below
// can be run inside the batch transaction.
type execer interface {
	Exec(string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

// ProjectResult reports what one pass applied.
type ProjectResult struct {
	Applied        int
	ThroughEventID int64
	Closed         int
}

// Project applies every event the projector has not yet seen.
//
// The whole batch, including the cursor, is one transaction. Without that, a
// failure mid-batch leaves rows written and the cursor unmoved, so the next
// pass replays events that were already applied -- which for a /clear closes
// the current conversation link and writes a reversed lineage edge.
func Project(s *Store) (ProjectResult, error) {
	var out ProjectResult

	if err := s.EnsureSource(SourceAttribution, "derived", "projector", 0); err != nil {
		return out, err
	}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return out, fmt.Errorf("begin projection: %w", err)
	}
	defer tx.Rollback()

	var through int64
	err = tx.QueryRow(`SELECT applied_through_event_id FROM projection_state WHERE projection = ?`,
		projectionName).Scan(&through)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, fmt.Errorf("read projection state: %w", err)
	}

	type pending struct {
		eventID    int64
		kind       string
		observedMs int64
		convID     sql.NullString
		instID     sql.NullString
		payload    string
	}
	var batch []pending

	rows, err := tx.Query(`SELECT event_id, kind, observed_ms, conversation_id, instance_id, payload
		FROM event WHERE event_id > ? ORDER BY event_id`, through)
	if err != nil {
		return out, fmt.Errorf("read events: %w", err)
	}
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.eventID, &p.kind, &p.observedMs, &p.convID, &p.instID, &p.payload); err != nil {
			rows.Close()
			return out, fmt.Errorf("scan event: %w", err)
		}
		batch = append(batch, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}

	for _, p := range batch {
		switch p.kind {
		case "session.observed":
			var rec SessionRecord
			if err := json.Unmarshal([]byte(p.payload), &rec); err != nil {
				return out, fmt.Errorf("event %d payload: %w", p.eventID, err)
			}
			if err := s.applySessionObserved(tx, p.eventID, p.observedMs, rec, p.convID.String, p.instID.String); err != nil {
				return out, err
			}
		case "pane.observed":
			if err := s.verifyPane(tx, p.eventID, p.payload); err != nil {
				return out, err
			}
		case "scan.completed":
			n, err := s.reconcileAbsences(tx, p.eventID, p.observedMs, p.payload)
			if err != nil {
				return out, err
			}
			out.Closed += n
		}
		out.Applied++
		out.ThroughEventID = p.eventID
	}

	if out.ThroughEventID > 0 {
		if _, err := tx.Exec(`INSERT INTO projection_state (projection, applied_through_event_id, updated_ms)
			VALUES (?, ?, ?)
			ON CONFLICT(projection) DO UPDATE SET applied_through_event_id = excluded.applied_through_event_id,
			                                      updated_ms = excluded.updated_ms`,
			projectionName, out.ThroughEventID, s.now()); err != nil {
			return out, fmt.Errorf("save projection state: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return out, fmt.Errorf("commit projection: %w", err)
	}

	// The projector is a producer too. Without this its source row reads
	// "never succeeded" forever, which is the same lie as a dead watcher
	// reading as healthy, only in the other direction.
	if _, err := s.db.Exec(`UPDATE source SET status = 'ok', last_success_ms = ?, last_error = NULL
		WHERE source_id = ?`, s.now(), SourceAttribution); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Store) applySessionObserved(ex execer, eventID, observedMs int64, rec SessionRecord, convID, instID string) error {
	seen := rec.UpdatedAt
	if seen == 0 {
		seen = rec.StartedAt
	}

	if _, err := ex.Exec(`
		INSERT INTO conversation (conversation_id, provider, host, provider_session_id,
		    display_name, display_name_source, display_name_since_ms,
		    first_seen_ms, last_seen_ms, first_event_id, last_event_id)
		VALUES (?, 'claude', ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(conversation_id) DO UPDATE SET
		    display_name = excluded.display_name,
		    display_name_source = excluded.display_name_source,
		    display_name_since_ms = excluded.display_name_since_ms,
		    last_seen_ms = MAX(conversation.last_seen_ms, excluded.last_seen_ms),
		    last_event_id = excluded.last_event_id`,
		convID, s.host, rec.SessionID, rec.Name, rec.NameSource, nullZero(rec.NameSince),
		seen, seen, eventID, eventID); err != nil {
		return fmt.Errorf("upsert conversation: %w", err)
	}

	if _, err := ex.Exec(`
		INSERT INTO launch_instance (instance_id, host, pid_domain, pid, started_ms, proc_start_raw,
		    launch_cwd, entrypoint, kind, agent_version, messaging_socket, bridge_session_id,
		    first_event_id, last_event_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(instance_id) DO UPDATE SET
		    launch_cwd = excluded.launch_cwd,
		    agent_version = excluded.agent_version,
		    bridge_session_id = excluded.bridge_session_id,
		    last_event_id = excluded.last_event_id`,
		instID, s.host, rec.PIDDomain, rec.PID, rec.StartedAt, rec.ProcStart,
		rec.CWD, rec.Entrypoint, rec.Kind, rec.Version, rec.MessagingSocket, rec.BridgeSessionID,
		eventID, eventID); err != nil {
		return fmt.Errorf("upsert instance: %w", err)
	}

	if err := s.linkInstanceConversation(ex, eventID, instID, convID, seen); err != nil {
		return err
	}
	if err := s.claimPane(ex, eventID, instID, rec, seen); err != nil {
		return err
	}
	return s.attribute(ex, eventID, convID, rec, seen)
}

// linkInstanceConversation maintains the time-bounded link. /clear replaces
// sessionId inside a living process, so the previous link is closed with a
// named basis and a lineage edge records the succession -- rather than the row
// being overwritten, which would erase that a /clear ever happened.
func (s *Store) linkInstanceConversation(ex execer, eventID int64, instID, convID string, seen int64) error {
	var openConv string
	err := ex.QueryRow(`SELECT conversation_id FROM instance_conversation
		WHERE instance_id = ? AND observed_to_ms IS NULL`, instID).Scan(&openConv)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// nothing open
	case err != nil:
		return fmt.Errorf("read open conversation link: %w", err)
	case openConv == convID:
		_, err := ex.Exec(`UPDATE instance_conversation SET last_event_id = ?
			WHERE instance_id = ? AND observed_to_ms IS NULL`, eventID, instID)
		return err
	default:
		if _, err := ex.Exec(`UPDATE instance_conversation
			SET observed_to_ms = ?, end_basis = 'session_id_changed', end_event_id = ?, last_event_id = ?
			WHERE instance_id = ? AND observed_to_ms IS NULL`, seen, eventID, eventID, instID); err != nil {
			return fmt.Errorf("close previous conversation link: %w", err)
		}
		if _, err := ex.Exec(`INSERT INTO conversation_lineage
			(parent_conversation_id, child_conversation_id, relation, basis, confidence, observed_ms, event_id)
			VALUES (?, ?, 'clear', 'same_process_session_change', 1.0, ?, ?)
			ON CONFLICT DO NOTHING`, openConv, convID, seen, eventID); err != nil {
			return fmt.Errorf("record lineage: %w", err)
		}
	}

	_, err = ex.Exec(`INSERT INTO instance_conversation
		(instance_id, conversation_id, observed_from_ms, first_event_id, last_event_id)
		VALUES (?, ?, ?, ?, ?)`, instID, convID, seen, eventID, eventID)
	return err
}

// claimPane records where the record says it is.
//
// The lookup is by (instance, pane id), NOT by the claimed pane_key. Once a
// binding has been verified its generated key is the real one, so a key-based
// lookup misses, inserts a second claim, and then collides on the unique index
// when that claim is verified onto the key the first row already holds. That
// crashed the projector on the fourth live sweep and left a duplicate binding
// behind.
func (s *Store) claimPane(ex execer, eventID int64, instID string, rec SessionRecord, seen int64) error {
	ref, ok := ParseTmuxRef(rec.Tmux)
	if !ok {
		return nil
	}

	var bindingID int64
	err := ex.QueryRow(`SELECT binding_id FROM pane_binding
		WHERE instance_id = ? AND pane_id = ? AND observed_to_ms IS NULL`, instID, ref.PaneID).Scan(&bindingID)
	if err == nil {
		// The session name is refreshed because it is dated evidence that
		// attribution and lineage both read; it is not part of the key, so a
		// rename cannot fork the binding.
		_, err := ex.Exec(`UPDATE pane_binding SET session_name_seen = ?, window_id = ?, last_event_id = ?
			WHERE binding_id = ?`, ref.SessionName, ref.WindowID, eventID, bindingID)
		return err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read open binding: %w", err)
	}

	if _, err := ex.Exec(`INSERT INTO pane_binding
		(instance_id, window_id, pane_id, session_name_seen, binding_basis, observed_from_ms,
		 first_event_id, last_event_id)
		VALUES (?, ?, ?, ?, 'session_file_claim', ?, ?, ?)`,
		instID, ref.WindowID, ref.PaneID, ref.SessionName, seen, eventID, eventID); err != nil {
		return err
	}
	// Verify immediately against what the log already knows about this pane,
	// rather than waiting for that pane to change.
	return s.verifyFromLatestObservation(ex, eventID, ref.PaneID)
}

// ---------------------------------------------------------------- absence

type scanRoster struct {
	RecordsSeen int                `json:"records_seen"`
	SourceID    string             `json:"source_id"`
	Host        string             `json:"host"`
	Instances   *[]string          `json:"instances"`
	Probed      *map[string]string `json:"probed"`
}

// reconcileAbsences closes what a complete sweep did not see.
//
// The roster and the liveness results both come from the sweep's own event.
// The roster cannot be derived from which events the sweep produced, because
// an unchanged record is deduped and emits nothing -- an event-derived roster
// reads every quiet agent as departed. And the probe cannot be run here,
// because replay would answer it with today's process table.
//
// Both fields are required rather than optional. A payload missing its roster
// would unmarshal to an empty one, and an empty roster says everything is
// gone: the precise shape of an absence of data being read as data.
func (s *Store) reconcileAbsences(ex execer, eventID, observedMs int64, payload string) (int, error) {
	var roster scanRoster
	if err := json.Unmarshal([]byte(payload), &roster); err != nil {
		return 0, fmt.Errorf("scan roster in event %d: %w", eventID, err)
	}
	if roster.Instances == nil || roster.Probed == nil {
		return 0, fmt.Errorf("scan roster in event %d has no instance list or probe results; refusing to treat that as an empty estate", eventID)
	}
	if roster.SourceID == "" {
		return 0, fmt.Errorf("scan roster in event %d names no source; its scope is unknown", eventID)
	}
	present := make(map[string]bool, len(*roster.Instances))
	for _, id := range *roster.Instances {
		present[id] = true
	}
	probed := *roster.Probed

	// Scoped to the source that ran the sweep. A sweep of the Claude session
	// directory has no standing over a Codex agent or a zklw one, and judging
	// them absent because they were never in scope is the same error again.
	rows, err := ex.Query(`
		SELECT li.instance_id FROM launch_instance li
		  JOIN event e ON e.event_id = li.first_event_id
		 WHERE li.ended_ms IS NULL AND e.source_id = ?`, roster.SourceID)
	if err != nil {
		return 0, fmt.Errorf("find open instances: %w", err)
	}
	var open []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		open = append(open, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	closed := 0
	for _, id := range open {
		// Only a recorded death closes anything. Alive, unknown, and not
		// probed at all each withhold.
		if probed[id] != ProbeDead {
			continue
		}
		basis := "absent_from_complete_scan"
		if present[id] {
			// The record is still on disk but its process is gone: the
			// provider leaves no tombstone, so without this the record reads
			// as live forever.
			basis = "process_exited"
		}
		if err := s.closeInstance(ex, id, basis, observedMs, eventID); err != nil {
			return closed, err
		}
		closed++
	}
	return closed, nil
}

func (s *Store) closeInstance(ex execer, instanceID, basis string, endedMs, eventID int64) error {
	if _, err := ex.Exec(`UPDATE launch_instance
		SET ended_ms = ?, end_basis = ?, end_event_id = ?
		WHERE instance_id = ? AND ended_ms IS NULL`, endedMs, basis, eventID, instanceID); err != nil {
		return fmt.Errorf("close instance %s: %w", instanceID, err)
	}
	if _, err := ex.Exec(`UPDATE instance_conversation
		SET observed_to_ms = ?, end_basis = 'instance_ended', end_event_id = ?
		WHERE instance_id = ? AND observed_to_ms IS NULL`, endedMs, eventID, instanceID); err != nil {
		return err
	}
	_, err := ex.Exec(`UPDATE pane_binding
		SET observed_to_ms = ?, end_basis = 'instance_ended', end_event_id = ?
		WHERE instance_id = ? AND observed_to_ms IS NULL`, endedMs, eventID, instanceID)
	return err
}

// ---------------------------------------------------------------- attribution

type attemptedBasis struct {
	Basis  string `json:"basis"`
	Result string `json:"result"`
}

// attribute records positive associations, and when none can be made, appends
// a reason. #unassigned is then able to say why rather than shrug.
func (s *Store) attribute(ex execer, eventID int64, convID string, rec SessionRecord, seen int64) error {
	var attempts []attemptedBasis

	project, reason := projectFromCWD(rec.CWD)
	if project != "" {
		if err := s.associate(ex, eventID, convID, project, "launch_cwd", 0.9, seen); err != nil {
			return err
		}
	} else {
		attempts = append(attempts, attemptedBasis{"launch_cwd", reason})
	}

	if ref, ok := ParseTmuxRef(rec.Tmux); ok {
		named, reason := projectFromSessionName(ref.SessionName)
		if named != "" {
			if err := s.associate(ex, eventID, convID, named, "tmux_session_name", 0.6, seen); err != nil {
				return err
			}
		} else {
			attempts = append(attempts, attemptedBasis{"tmux_session_name", reason})
		}
	} else {
		attempts = append(attempts, attemptedBasis{"tmux_session_name", "record claims no tmux pane"})
	}

	if len(attempts) < 2 {
		return nil
	}
	body, err := json.Marshal(struct {
		Conversation string           `json:"conversation_id"`
		Attempted    []attemptedBasis `json:"attempted"`
	}{convID, attempts})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	_, _, err = s.appendEventTx(ex, Event{
		SourceID:       SourceAttribution,
		DedupeKey:      "attr:" + convID + ":" + hex.EncodeToString(sum[:])[:16],
		Kind:           "attribution.attempted",
		OccurredMs:     seen,
		ConversationID: convID,
		Payload:        body,
	})
	return err
}

func (s *Store) associate(ex execer, eventID int64, convID, project, basis string, confidence float64, seen int64) error {
	_, err := ex.Exec(`
		INSERT INTO project_association (conversation_id, project_key, basis, confidence, observed_ms, event_id)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(conversation_id, project_key, basis) DO UPDATE SET
		    confidence = excluded.confidence, observed_ms = excluded.observed_ms, event_id = excluded.event_id`,
		convID, project, basis, confidence, seen, eventID)
	return err
}

// projectFromCWD reads a project out of a launch directory.
//
// The umbrella directory itself is not a project, and on this estate ten of
// twelve live agents sit in exactly that directory -- so this basis fails far
// more often than it succeeds, and saying so is the point.
func projectFromCWD(cwd string) (string, string) {
	if cwd == "" {
		return "", "record carries no cwd"
	}
	clean := filepath.Clean(cwd)
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "home directory unresolvable"
	}
	root := filepath.Join(home, "projects")
	if clean == root {
		return "", "launched in the umbrella directory, which is not a project"
	}
	rel, err := filepath.Rel(root, clean)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", "launched outside the projects tree"
	}
	return strings.SplitN(rel, string(filepath.Separator), 2)[0], ""
}

// projectFromSessionName reads the "{terminal}[{project}(@{agent})? - {uuid}"
// convention out of a tmux session name. The bracket varies -- "[", "]" and
// "[]" all occur on this estate -- so the first of either opens the project
// segment.
func projectFromSessionName(name string) (string, string) {
	cut := strings.IndexAny(name, "[]")
	if cut < 0 {
		return "", "session name follows no project convention"
	}
	rest := strings.TrimLeft(name[cut:], "[]")
	if i := strings.Index(rest, " - "); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.IndexAny(rest, "|@"); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "session name has an empty project segment"
	}
	return rest, ""
}

func nullZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// Rebuild drops every projection and replays the log into fresh ones.
//
// This is the operation the no-foreign-key-into-a-projection rule exists to
// make possible, and the reason identifiers are deterministic: the durable
// tables keep naming conversations through the rebuild, and a replay
// regenerates exactly the ids they name.
func Rebuild(s *Store) (ProjectResult, error) {
	tables := ProjectionTables()
	for i := len(tables) - 1; i >= 0; i-- {
		if _, err := s.db.Exec("DROP TABLE IF EXISTS " + tables[i]); err != nil {
			return ProjectResult{}, fmt.Errorf("drop %s: %w", tables[i], err)
		}
	}
	if _, err := s.db.Exec(schema); err != nil {
		return ProjectResult{}, fmt.Errorf("recreate projections: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM projection_state WHERE projection = ?`, projectionName); err != nil {
		return ProjectResult{}, fmt.Errorf("clear projection state: %w", err)
	}
	return Project(s)
}
