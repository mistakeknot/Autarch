package registry

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The projector turns the event log into the identity tables. It is the only
// writer of those tables, it reads events in event_id order, and it records
// how far it got -- so the projections can be dropped and rebuilt without
// re-reading the world.
//
// It derives; it does not observe. The one exception is an attribution
// attempt, which is appended to the log under its own producer with a
// content-derived key, so a replay re-derives the same reason and inserts
// nothing new.

const projectionName = "registry"

// SourceAttribution is the producer id for reasons the projector derives.
const SourceAttribution = "registry-attribution"

// ProjectResult reports what one pass applied.
type ProjectResult struct {
	Applied        int
	ThroughEventID int64
	Closed         int
}

// Project applies every event the projector has not yet seen.
func Project(s *Store) (ProjectResult, error) {
	var out ProjectResult

	if err := s.EnsureSource(SourceAttribution, "derived", "projector", 0); err != nil {
		return out, err
	}

	var through int64
	err := s.db.QueryRow(`SELECT applied_through_event_id FROM projection_state WHERE projection = ?`,
		projectionName).Scan(&through)
	if err != nil && err != sql.ErrNoRows {
		return out, fmt.Errorf("read projection state: %w", err)
	}

	rows, err := s.db.Query(`SELECT event_id, kind, scan_id, occurred_ms, conversation_id, instance_id, payload
		FROM event WHERE event_id > ? ORDER BY event_id`, through)
	if err != nil {
		return out, fmt.Errorf("read events: %w", err)
	}

	type pending struct {
		eventID int64
		kind    string
		scanID  sql.NullInt64
		convID  sql.NullString
		instID  sql.NullString
		payload string
	}
	var batch []pending
	for rows.Next() {
		var p pending
		var occurred sql.NullInt64
		if err := rows.Scan(&p.eventID, &p.kind, &p.scanID, &occurred, &p.convID, &p.instID, &p.payload); err != nil {
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
				// The payload is the raw provider record and was parsed once
				// already at ingest. If it will not parse now the log is
				// corrupt, which is worth stopping for.
				return out, fmt.Errorf("event %d payload: %w", p.eventID, err)
			}
			if err := s.applySessionObserved(p.eventID, rec, p.convID.String, p.instID.String); err != nil {
				return out, err
			}
		case "pane.observed":
			if err := s.verifyPane(p.eventID, p.payload); err != nil {
				return out, err
			}
		case "scan.completed":
			n, err := s.reconcileAbsences(p.eventID, p.payload)
			if err != nil {
				return out, err
			}
			out.Closed += n
		}
		out.Applied++
		out.ThroughEventID = p.eventID
	}

	// The projector is a producer too. Without this its source row reads
	// "never succeeded" forever, which is the same lie as a dead watcher
	// reading as healthy, only in the other direction.
	if _, err := s.db.Exec(`UPDATE source SET status = 'ok', last_success_ms = ?, last_error = NULL
		WHERE source_id = ?`, s.now(), SourceAttribution); err != nil {
		return out, err
	}

	if out.ThroughEventID > 0 {
		if _, err := s.db.Exec(`INSERT INTO projection_state (projection, applied_through_event_id, updated_ms)
			VALUES (?, ?, ?)
			ON CONFLICT(projection) DO UPDATE SET applied_through_event_id = excluded.applied_through_event_id,
			                                      updated_ms = excluded.updated_ms`,
			projectionName, out.ThroughEventID, s.now()); err != nil {
			return out, fmt.Errorf("save projection state: %w", err)
		}
	}
	return out, nil
}

func (s *Store) applySessionObserved(eventID int64, rec SessionRecord, convID, instID string) error {
	seen := rec.UpdatedAt
	if seen == 0 {
		seen = rec.StartedAt
	}

	if _, err := s.db.Exec(`
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

	if _, err := s.db.Exec(`
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

	if err := s.linkInstanceConversation(eventID, instID, convID, seen); err != nil {
		return err
	}
	if err := s.claimPane(eventID, instID, rec, seen); err != nil {
		return err
	}
	return s.attribute(eventID, convID, rec, seen)
}

// linkInstanceConversation maintains the time-bounded link. /clear replaces
// sessionId inside a living process, so the previous link is closed with a
// named basis and a lineage edge records the succession -- rather than the
// row being overwritten, which would erase that a /clear ever happened.
func (s *Store) linkInstanceConversation(eventID int64, instID, convID string, seen int64) error {
	var openConv string
	err := s.db.QueryRow(`SELECT conversation_id FROM instance_conversation
		WHERE instance_id = ? AND observed_to_ms IS NULL`, instID).Scan(&openConv)
	switch {
	case err == sql.ErrNoRows:
		// nothing open
	case err != nil:
		return fmt.Errorf("read open conversation link: %w", err)
	case openConv == convID:
		_, err := s.db.Exec(`UPDATE instance_conversation SET last_event_id = ?
			WHERE instance_id = ? AND observed_to_ms IS NULL`, eventID, instID)
		return err
	default:
		if _, err := s.db.Exec(`UPDATE instance_conversation
			SET observed_to_ms = ?, end_basis = 'session_id_changed', end_event_id = ?, last_event_id = ?
			WHERE instance_id = ? AND observed_to_ms IS NULL`, seen, eventID, eventID, instID); err != nil {
			return fmt.Errorf("close previous conversation link: %w", err)
		}
		if _, err := s.db.Exec(`INSERT INTO conversation_lineage
			(parent_conversation_id, child_conversation_id, relation, basis, confidence, observed_ms, event_id)
			VALUES (?, ?, 'clear', 'same_process_session_change', 1.0, ?, ?)
			ON CONFLICT DO NOTHING`, openConv, convID, seen, eventID); err != nil {
			return fmt.Errorf("record lineage: %w", err)
		}
	}

	_, err = s.db.Exec(`INSERT INTO instance_conversation
		(instance_id, conversation_id, observed_from_ms, first_event_id, last_event_id)
		VALUES (?, ?, ?, ?, ?)`, instID, convID, seen, eventID, eventID)
	return err
}

// claimPane records where the record says it is. B1 has no tmux inventory, so
// every binding it writes is a claim: keyed on the pane id alone, and never
// mistakable for a verified one.
func (s *Store) claimPane(eventID int64, instID string, rec SessionRecord, seen int64) error {
	ref, ok := ParseTmuxRef(rec.Tmux)
	if !ok {
		return nil
	}
	key := ClaimedPaneKey(ref.PaneID)

	var bindingID int64
	err := s.db.QueryRow(`SELECT binding_id FROM pane_binding
		WHERE instance_id = ? AND pane_key = ? AND observed_to_ms IS NULL`, instID, key).Scan(&bindingID)
	if err == nil {
		// The session name is refreshed because it is dated evidence that
		// attribution and lineage both read; it is not part of the key, so
		// a rename cannot fork the binding.
		_, err := s.db.Exec(`UPDATE pane_binding SET session_name_seen = ?, window_id = ?, last_event_id = ?
			WHERE binding_id = ?`, ref.SessionName, ref.WindowID, eventID, bindingID)
		return err
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("read open binding: %w", err)
	}

	if _, err := s.db.Exec(`INSERT INTO pane_binding
		(instance_id, window_id, pane_id, session_name_seen, binding_basis, observed_from_ms,
		 first_event_id, last_event_id)
		VALUES (?, ?, ?, ?, 'session_file_claim', ?, ?, ?)`,
		instID, ref.WindowID, ref.PaneID, ref.SessionName, seen, eventID, eventID); err != nil {
		return err
	}
	// Verify immediately against what the log already knows about this pane,
	// rather than waiting for that pane to change.
	return s.verifyFromLatestObservation(eventID, ref.PaneID)
}

// ---------------------------------------------------------------- absence

// reconcileAbsences closes what a complete sweep did not see.
//
// The roster comes from the sweep's own event, not from which events it
// produced: an unchanged record is deduped and emits nothing, so an
// event-derived roster would read every quiet agent as departed.
//
// A complete scan alone is still not enough, because a record can vanish while
// its process lives. The liveness check can only WITHHOLD a closure, never
// cause one -- pids recur, so a live pid proves nothing about whose it is,
// while a dead one corroborates an absence the scan already established.
func (s *Store) reconcileAbsences(eventID int64, payload string) (int, error) {
	var roster struct {
		Instances []string `json:"instances"`
	}
	if err := json.Unmarshal([]byte(payload), &roster); err != nil {
		return 0, fmt.Errorf("scan roster in event %d: %w", eventID, err)
	}
	present := make(map[string]bool, len(roster.Instances))
	for _, id := range roster.Instances {
		present[id] = true
	}

	rows, err := s.db.Query(`SELECT instance_id, pid FROM launch_instance WHERE ended_ms IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("find open instances: %w", err)
	}
	type candidate struct {
		id  string
		pid int64
	}
	var absent []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.pid); err != nil {
			rows.Close()
			return 0, err
		}
		if !present[c.id] {
			absent = append(absent, c)
		}
	}
	rows.Close()

	closed := 0
	for _, c := range absent {
		if processAlive(c.pid) {
			continue
		}
		now := s.now()
		if _, err := s.db.Exec(`UPDATE launch_instance
			SET ended_ms = ?, end_basis = 'absent_from_complete_scan', end_event_id = ?
			WHERE instance_id = ? AND ended_ms IS NULL`, now, eventID, c.id); err != nil {
			return closed, fmt.Errorf("close instance %s: %w", c.id, err)
		}
		if _, err := s.db.Exec(`UPDATE instance_conversation
			SET observed_to_ms = ?, end_basis = 'instance_ended', end_event_id = ?
			WHERE instance_id = ? AND observed_to_ms IS NULL`, now, eventID, c.id); err != nil {
			return closed, err
		}
		if _, err := s.db.Exec(`UPDATE pane_binding
			SET observed_to_ms = ?, end_basis = 'instance_ended', end_event_id = ?
			WHERE instance_id = ? AND observed_to_ms IS NULL`, now, eventID, c.id); err != nil {
			return closed, err
		}
		closed++
	}
	return closed, nil
}

// ---------------------------------------------------------------- attribution

type attemptedBasis struct {
	Basis  string `json:"basis"`
	Result string `json:"result"`
}

// attribute records positive associations, and when none can be made, appends
// a reason. #unassigned is then able to say why rather than shrug.
func (s *Store) attribute(eventID int64, convID string, rec SessionRecord, seen int64) error {
	var attempts []attemptedBasis

	project, reason := projectFromCWD(rec.CWD)
	if project != "" {
		if err := s.associate(eventID, convID, project, "launch_cwd", 0.9, seen); err != nil {
			return err
		}
	} else {
		attempts = append(attempts, attemptedBasis{"launch_cwd", reason})
	}

	if ref, ok := ParseTmuxRef(rec.Tmux); ok {
		named, reason := projectFromSessionName(ref.SessionName)
		if named != "" {
			if err := s.associate(eventID, convID, named, "tmux_session_name", 0.6, seen); err != nil {
				return err
			}
		} else {
			attempts = append(attempts, attemptedBasis{"tmux_session_name", reason})
		}
	} else {
		attempts = append(attempts, attemptedBasis{"tmux_session_name", "record claims no tmux pane"})
	}

	// A conversation that got at least one association is attributed; only a
	// total failure needs a recorded reason.
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
	_, _, err = s.AppendEvent(Event{
		SourceID:       SourceAttribution,
		DedupeKey:      "attr:" + convID + ":" + hex.EncodeToString(sum[:])[:16],
		Kind:           "attribution.attempted",
		OccurredMs:     seen,
		ConversationID: convID,
		Payload:        body,
	})
	return err
}

func (s *Store) associate(eventID int64, convID, project, basis string, confidence float64, seen int64) error {
	_, err := s.db.Exec(`
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
	home, err := homeDir()
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

func homeDir() (string, error) { return os.UserHomeDir() }

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
