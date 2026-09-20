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
	"sort"
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
	// Events the projector declined to act on. They do not stop the batch --
	// one malformed event must not wedge the log -- but they are reported,
	// because a projector that silently skips is a projector nobody notices
	// has stopped working.
	Refusals []string
}

// Project applies every event the projector has not yet seen.
//
// The whole batch, including the cursor, is one transaction. Without that, a
// failure mid-batch leaves rows written and the cursor unmoved, so the next
// pass replays events that were already applied -- which for a /clear closes
// the current conversation link and writes a reversed lineage edge.
func Project(s *Store) (out ProjectResult, retErr error) {

	if err := s.EnsureSource(SourceAttribution, "derived", "projector", 30_000); err != nil {
		return out, err
	}
	// A pass that fails must say so on its own source. Recording only success
	// there, as the first version did, means a wedged projector reads healthy
	// while the live list freezes and is presented as current.
	defer func() {
		if retErr != nil {
			// The real message, not a placeholder: this row is where an
			// operator finds out why the live list stopped moving.
			_, _ = s.db.Exec(`UPDATE source SET status = 'error', last_error = ?
				WHERE source_id = ?`, retErr.Error(), SourceAttribution)
		}
	}()

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
			refusal, err := s.applySessionObserved(tx, p.eventID, p.observedMs, rec, p.convID.String, p.instID.String)
			if err != nil {
				return out, err
			}
			if refusal != "" {
				out.Refusals = append(out.Refusals, refusal)
			}
		case "pane.observed":
			// The latest roster at or before this event, so a claim cannot be
			// upgraded by an observation that some OTHER pane's root process
			// already contradicts. One indexed lookup per changed pane, and
			// on this estate a sweep changes none to one.
			at, ok, err := s.latestPaneRoster(tx, p.eventID)
			if err != nil {
				return out, err
			}
			var seen *rosterAt
			if ok {
				seen = &at
			}
			refused, err := s.verifyPane(tx, p.eventID, p.payload, seen)
			if err != nil {
				return out, err
			}
			if refused > 0 {
				out.Refusals = append(out.Refusals, fmt.Sprintf(
					"event %d: %d claim(s) on that pane name a different window than the live server; left unverified",
					p.eventID, refused))
			}
		case "scan.completed":
			// Routed by the source that swept, because a roster's shape is
			// its source's shape: the session sweep publishes instances and
			// probe verdicts, the tmux sweep publishes panes on one server.
			// Handing either roster to the other reconciler would present a
			// missing field as an empty estate -- and the session reconciler
			// refuses an absent instance list precisely so that cannot happen
			// quietly.
			var n int
			var refusal string
			var err error
			switch sourceOf(p.payload) {
			case SourceClaudeSessions:
				n, refusal, err = s.reconcileAbsences(tx, p.eventID, p.observedMs, p.payload)
			case SourceTmuxInventory:
				n, refusal, err = s.reconcilePaneAbsences(tx, p.eventID, p.observedMs, p.payload)
				if err == nil && refusal == "" {
					// Only a roster this pass accepted may drive a retry.
					if at, ok, rErr := s.latestPaneRoster(tx, p.eventID); rErr != nil {
						err = rErr
					} else if ok && at.eventID == p.eventID {
						var refused int
						refused, err = s.retryStuckClaims(tx, p.eventID, at)
						if refused > 0 {
							refusal = fmt.Sprintf("event %d: %d claim(s) name a pane the live server puts in a different window, or whose parent process is in another pane; left unverified",
								p.eventID, refused)
						}
					}
				}
			default:
				// A producer emitting completed sweeps that nothing
				// reconciles is a gap that must announce itself. Silence here
				// would mean a whole source's absences were never acted on
				// and nothing said so.
				refusal = fmt.Sprintf("scan roster in event %d names a source no reconciler handles; its absences were not acted on", p.eventID)
			}
			if err != nil {
				return out, err
			}
			out.Closed += n
			if refusal != "" {
				out.Refusals = append(out.Refusals, refusal)
			}
		}
		out.Applied++
		out.ThroughEventID = p.eventID
	}

	if out.ThroughEventID > 0 {
		// Refusals land here, in the row replay owns, rather than only in
		// source.last_error -- which the next successful pass overwrites, and
		// one sweep projects twice, so a refusal from the first pass survived
		// for milliseconds. A refusal is the projector declining to act on an
		// event; if the only lasting record of that is a log line, then for
		// every reader of this database it did not happen. The counter is a
		// projection like everything else and is rebuilt by a replay.
		var lastRefusal any
		var lastRefusalEvent any
		if n := len(out.Refusals); n > 0 {
			lastRefusal = out.Refusals[n-1]
			lastRefusalEvent = out.ThroughEventID
		}
		if _, err := tx.Exec(`INSERT INTO projection_state
			(projection, applied_through_event_id, updated_ms, refusal_count,
			 last_refusal_event_id, last_refusal)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(projection) DO UPDATE SET
			  applied_through_event_id = excluded.applied_through_event_id,
			  updated_ms = excluded.updated_ms,
			  refusal_count = projection_state.refusal_count + excluded.refusal_count,
			  last_refusal_event_id = COALESCE(excluded.last_refusal_event_id, projection_state.last_refusal_event_id),
			  last_refusal = COALESCE(excluded.last_refusal, projection_state.last_refusal)`,
			projectionName, out.ThroughEventID, s.now(), len(out.Refusals),
			lastRefusalEvent, lastRefusal); err != nil {
			return out, fmt.Errorf("save projection state: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return out, fmt.Errorf("commit projection: %w", err)
	}

	// The projector is a producer too. Without this its source row reads
	// "never succeeded" forever, which is the same lie as a dead watcher
	// reading as healthy, only in the other direction.
	//
	// Outside the transaction deliberately: these are mutable liveness
	// columns no projection reads, and a failure here must not discard a
	// committed batch. It is also reported rather than returned, so the
	// caller's remaining work is not abandoned over a bookkeeping write.
	note := any(nil)
	if len(out.Refusals) > 0 {
		note = strings.Join(out.Refusals, "; ")
	}
	if _, err := s.db.Exec(`UPDATE source SET status = 'ok', last_success_ms = ?, last_error = ?
		WHERE source_id = ?`, s.now(), note, SourceAttribution); err != nil {
		return out, fmt.Errorf("record projector liveness (the batch itself committed): %w", err)
	}
	return out, nil
}

func (s *Store) applySessionObserved(ex execer, eventID, observedMs int64, rec SessionRecord, convID, instID string) (string, error) {
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
		return "", fmt.Errorf("upsert conversation: %w", err)
	}

	// Proof of life after a closure reopens the instance. Four false deaths
	// have already been found in this code; a fifth has to be recoverable
	// rather than silently absorbed. Without this the instance stays ended,
	// fresh conversation links and bindings accumulate underneath it, and it
	// disappears from view while it goes on writing records.
	if err := s.reopenIfEnded(ex, eventID, instID); err != nil {
		return "", err
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
		return "", fmt.Errorf("upsert instance: %w", err)
	}

	if err := s.linkInstanceConversation(ex, eventID, instID, convID, seen); err != nil {
		return "", err
	}
	refusal, err := s.claimPane(ex, eventID, instID, rec, seen)
	if err != nil {
		return refusal, err
	}
	return refusal, s.attribute(ex, eventID, convID, rec, seen)
}

// reopenIfEnded undoes a closure that a later observation contradicts, and
// records that the contradiction happened.
func (s *Store) reopenIfEnded(ex execer, eventID int64, instID string) error {
	var endedMs, endEvent sql.NullInt64
	err := ex.QueryRow(`SELECT ended_ms, end_event_id FROM launch_instance WHERE instance_id = ?`,
		instID).Scan(&endedMs, &endEvent)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !endedMs.Valid) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read instance state: %w", err)
	}

	if _, err := ex.Exec(`UPDATE launch_instance
		SET ended_ms = NULL, end_basis = NULL, end_event_id = NULL,
		    reopened_count = reopened_count + 1, last_reopen_event_id = ?
		WHERE instance_id = ?`, eventID, instID); err != nil {
		return err
	}

	// Retract what the closure closed, not just the instance. The rows cite
	// the closure event, so the match is exact. Leaving them shut opened a
	// second binding beside the first and -- worse -- meant a /clear that
	// happened while the instance was wrongly closed found no open link, so
	// its lineage edge was never written and the succession was lost for good.
	if !endEvent.Valid {
		return nil
	}
	if _, err := ex.Exec(`UPDATE instance_conversation
		SET observed_to_ms = NULL, end_basis = NULL, end_event_id = NULL
		WHERE instance_id = ? AND end_basis = 'instance_ended' AND end_event_id = ?`,
		instID, endEvent.Int64); err != nil {
		return err
	}
	_, err = ex.Exec(`UPDATE pane_binding
		SET observed_to_ms = NULL, end_basis = NULL, end_event_id = NULL
		WHERE instance_id = ? AND end_basis = 'instance_ended' AND end_event_id = ?`,
		instID, endEvent.Int64)
	return err
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
func (s *Store) claimPane(ex execer, eventID int64, instID string, rec SessionRecord, seen int64) (string, error) {
	ref, ok := ParseTmuxRef(rec.Tmux)
	if !ok {
		return "", nil
	}

	var bindingID int64
	err := ex.QueryRow(`SELECT binding_id FROM pane_binding
		WHERE instance_id = ? AND pane_id = ? AND observed_to_ms IS NULL`, instID, ref.PaneID).Scan(&bindingID)
	if err == nil {
		// The CLAIMED columns are refreshed, because the record is their sole
		// author and a rewritten record is a newer claim. The observed ones
		// are not touched: once a sweep has verified this binding, window_id
		// and session_name_seen hold what the live server said, and letting a
		// session record write over them would put a claim in the columns
		// that are supposed to hold evidence -- and would do it invisibly,
		// since both columns would still look populated.
		_, err := ex.Exec(`UPDATE pane_binding
			SET claimed_session_name = ?, claimed_window_id = ?, last_event_id = ?,
			    window_id = CASE WHEN binding_basis = 'session_file_claim' THEN ? ELSE window_id END,
			    session_name_seen = CASE WHEN binding_basis = 'session_file_claim' THEN ? ELSE session_name_seen END
			WHERE binding_id = ?`,
			ref.SessionName, ref.WindowID, eventID, ref.WindowID, ref.SessionName, bindingID)
		return "", err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read open binding: %w", err)
	}

	// A pane a complete sweep has already watched vanish does not get a fresh
	// claim on the next rewrite of the record. Without this, every rewrite
	// reopened a claim on a dead pane, and it stayed open until the instance
	// ended -- a row saying "this agent is in %67" about a pane the registry
	// had itself recorded as gone. Bounded by the current roster rather than
	// forever: if a later sweep lists that pane again it is a place once more,
	// and the claim is allowed.
	var closedByAbsence int
	if err := ex.QueryRow(`SELECT COUNT(*) FROM pane_binding
		 WHERE instance_id = ? AND pane_id = ? AND end_basis = 'pane_absent_from_complete_scan'`,
		instID, ref.PaneID).Scan(&closedByAbsence); err != nil {
		return "", fmt.Errorf("read prior bindings: %w", err)
	}
	if closedByAbsence > 0 {
		at, ok, err := s.latestPaneRoster(ex, eventID)
		if err != nil {
			return "", err
		}
		if _, listed := at.present[ref.PaneID]; ok && !listed {
			return fmt.Sprintf("event %d: pane %s was watched to vanish and the latest sweep still does not list it; not reclaiming it", eventID, ref.PaneID), nil
		}
	}

	if _, err := ex.Exec(`INSERT INTO pane_binding
		(instance_id, window_id, pane_id, session_name_seen,
		 claimed_window_id, claimed_session_name, binding_basis, observed_from_ms,
		 first_event_id, last_event_id)
		VALUES (?, ?, ?, ?, ?, ?, 'session_file_claim', ?, ?, ?)`,
		instID, ref.WindowID, ref.PaneID, ref.SessionName,
		ref.WindowID, ref.SessionName, seen, eventID, eventID); err != nil {
		return "", err
	}
	// Deliberately NOT verified here. Every verification is authorised by a
	// complete sweep's roster, and the roster is applied after this -- which
	// is also when the same sweep's parent pids have been recorded. Verifying
	// on insert ran one step ahead of that, so a claim was always judged
	// before the process table had said anything about it, and the corroboration
	// that is the only defence against another server's identical pane id
	// could never be consulted. retryStuckClaims picks this up moments later,
	// in the same run of the watcher.
	return "", nil
}

// sourceOf reads the source id a roster declares.
//
// The event row carries a source_id too, but the roster's own field is what
// each reconciler scopes its queries by, so routing on anything else could
// hand a roster to a reconciler that then scopes itself somewhere else.
func sourceOf(payload string) string {
	var head struct {
		SourceID string `json:"source_id"`
	}
	if err := json.Unmarshal([]byte(payload), &head); err != nil {
		return ""
	}
	return head.SourceID
}

// ---------------------------------------------------------------- absence

type scanRoster struct {
	RecordsSeen int                `json:"records_seen"`
	SourceID    string             `json:"source_id"`
	Host        string             `json:"host"`
	Instances   *[]string          `json:"instances"`
	Changed     []string           `json:"changed"`
	Probed      *map[string]string `json:"probed"`
	// Not a pointer, unlike the fields above. Those are required because an
	// absent one would be read as an empty estate; this one is additive at
	// schema v4 and every event written before then legitimately has none.
	// Absent means the sweep predates parent capture, and an instance missing
	// from it means its process was not positively identified -- neither of
	// which is "this process has no parent", so both leave ppid alone.
	PPIDs   map[string]int64 `json:"ppids"`
	ProbeOK bool             `json:"probe_ok"`
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
func (s *Store) reconcileAbsences(ex execer, eventID, observedMs int64, payload string) (int, string, error) {
	var roster scanRoster
	if err := json.Unmarshal([]byte(payload), &roster); err != nil {
		return 0, fmt.Sprintf("scan roster in event %d will not parse (%v); refusing to read that as an empty estate", eventID, err), nil
	}
	// A refusal closes nothing and advances the cursor. Aborting the batch
	// instead would wedge the projector on this event forever, and a frozen
	// live list presented as current is its own kind of lie.
	if roster.Instances == nil || roster.Probed == nil {
		return 0, fmt.Sprintf("scan roster in event %d has no instance list or probe results; refusing to read that as an empty estate", eventID), nil
	}
	if roster.SourceID == "" {
		return 0, fmt.Sprintf("scan roster in event %d names no source, so its scope is unknown", eventID), nil
	}
	present := make(map[string]bool, len(*roster.Instances))
	for _, id := range *roster.Instances {
		present[id] = true
	}
	changed := make(map[string]bool, len(roster.Changed))
	for _, id := range roster.Changed {
		changed[id] = true
	}
	probed := *roster.Probed
	// The instances THIS run positively saw, as a JSON array for the parent
	// join. Being merely open is not evidence of being alive: an instance the
	// registry has not got round to closing can be long gone, and naming it as
	// something's parent is a stranger's lineage recorded as an agent's.
	aliveIDs := make([]string, 0, len(probed))
	for id, verdict := range probed {
		if verdict == ProbeAlive {
			aliveIDs = append(aliveIDs, id)
		}
	}
	sort.Strings(aliveIDs)
	aliveJSON, err := json.Marshal(aliveIDs)
	if err != nil {
		return 0, "", err
	}
	alive := string(aliveJSON)

	// Scoped to the source that ran the sweep. A sweep of the Claude session
	// directory has no standing over a Codex agent or a zklw one, and judging
	// them absent because they were never in scope is the same error again.
	//
	// Ended instances are included: a sweep that positively sees a process it
	// had written off is a contradiction, and the contradiction has to be
	// readable without waiting for that agent to write something.
	// Ordered: without it the outcome depended on the order SQLite happened to
	// return rows in, which makes a parent resolved before its child a
	// different projection from one resolved after -- and a replay a coin toss.
	rows, err := ex.Query(`
		SELECT li.instance_id, li.ended_ms FROM launch_instance li
		  JOIN event e ON e.event_id = li.first_event_id
		 WHERE e.source_id = ?
		 ORDER BY li.instance_id`, roster.SourceID)
	if err != nil {
		return 0, "", fmt.Errorf("find open instances: %w", err)
	}
	type known struct {
		id    string
		ended bool
	}
	var all []known
	for rows.Next() {
		var k known
		var ended sql.NullInt64
		if err := rows.Scan(&k.id, &ended); err != nil {
			rows.Close()
			return 0, "", err
		}
		k.ended = ended.Valid
		all = append(all, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, "", err
	}

	closed := 0
	for _, k := range all {
		id := k.id
		verdict := probed[id]

		if verdict == ProbeAlive {
			// Positive proof of life. Refresh the timestamp, and take back a
			// closure if one is standing.
			if _, err := ex.Exec(`UPDATE launch_instance SET last_alive_ms = ? WHERE instance_id = ?`,
				observedMs, id); err != nil {
				return closed, "", err
			}
			if ppid, ok := roster.PPIDs[id]; ok && ppid > 0 {
				if err := s.recordParent(ex, id, ppid, alive); err != nil {
					return closed, "", err
				}
			}
			if k.ended {
				if err := s.reopenIfEnded(ex, eventID, id); err != nil {
					return closed, "", err
				}
			}
			continue
		}
		if k.ended {
			continue
		}
		// Only a recorded death closes anything. Unknown, and not probed at
		// all, each withhold.
		if verdict != ProbeDead {
			continue
		}
		if changed[id] {
			// The record was rewritten during this very sweep. Whatever the
			// process table said a moment later, something was alive to write
			// it, and a contradiction withholds.
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
			return closed, "", err
		}
		closed++
	}
	return closed, "", nil
}

// recordParent stores a parent pid the probe read, and resolves it to another
// instance only when this registry already holds one with that pid.
//
// The join is deliberately narrow: same host, same pid domain, and open at the
// time -- a record we have, never a guess. A pid alone is not an identity, and
// naming a parent we cannot see would put an inference in a column the rest of
// the system will read as an observation.
//
// A ppid that resolves to nothing is still recorded. On this estate a
// human-launched agent's parent is its pane's login shell, which this registry
// does not track and never will; that is a real, useful parent, and dropping
// it because it is not one of ours would discard the very fact that
// distinguishes a human launch from a dispatched child.
func (s *Store) recordParent(ex execer, instanceID string, ppid int64, alive string) error {
	var parent sql.NullString
	// alive is the set this same ps run positively saw. A row that is merely
	// still open is not enough: an instance the registry has not yet closed
	// can be long gone, and a pid that is a live ppid while its instance is
	// dead belongs to a stranger who now holds that number.
	//
	// started_ms bounds it the other way -- a parent cannot have started after
	// its child -- and instance_id orders it, because without an ORDER BY the
	// answer depended on row order and so did every replay.
	err := ex.QueryRow(`SELECT parent.instance_id FROM launch_instance parent
		 JOIN launch_instance child ON child.instance_id = ?
		 JOIN json_each(?) live ON live.value = parent.instance_id
		WHERE parent.pid = ? AND parent.host = child.host
		  AND parent.pid_domain = child.pid_domain
		  AND parent.instance_id <> child.instance_id
		  AND parent.ended_ms IS NULL
		  AND parent.started_ms <= child.started_ms
		ORDER BY parent.started_ms DESC, parent.instance_id LIMIT 1`,
		instanceID, alive, ppid).Scan(&parent)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("resolve parent of %s: %w", instanceID, err)
	}
	// parent_instance_id is only ever set, never cleared by a later sweep that
	// could not resolve it: the parent exiting does not retract the fact that
	// it was the parent.
	// COALESCE on both: the launch parent is a fact about a launch, and the
	// later value is not a correction of it. A reparented process reports
	// ppid 1 -- or a subreaper on Linux -- and overwriting with that destroys
	// exactly what this field was captured for, the difference between an
	// agent a human started and one another agent dispatched.
	_, err = ex.Exec(`UPDATE launch_instance
		SET ppid = COALESCE(ppid, ?), parent_instance_id = COALESCE(parent_instance_id, ?)
		WHERE instance_id = ?`, ppid, parent, instanceID)
	return err
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
	if err := s.resetProjections(); err != nil {
		return ProjectResult{}, err
	}
	return Project(s)
}

// resetProjections drops and recreates the projection tables in one
// transaction. SQLite DDL is transactional, so a failure part way through
// leaves the previous projections intact rather than empty ones.
func (s *Store) resetProjections() error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin reset: %w", err)
	}
	defer tx.Rollback()

	tables := ProjectionTables()
	for i := len(tables) - 1; i >= 0; i-- {
		if _, err := tx.Exec("DROP TABLE IF EXISTS " + tables[i]); err != nil {
			return fmt.Errorf("drop %s: %w", tables[i], err)
		}
	}
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("recreate projections: %w", err)
	}
	// projection_state is itself derived, so it is dropped rather than merely
	// emptied. Deleting its rows left the TABLE at its old shape, and the
	// schema's CREATE ... IF NOT EXISTS is a no-op against an existing one --
	// so a version that added a column to it produced a migration that
	// reported success and then failed on the first write. Caught on a copy of
	// the live database, which is the only reason it is not in the live one.
	if _, err := tx.Exec(`DROP TABLE IF EXISTS projection_state`); err != nil {
		return fmt.Errorf("drop projection state: %w", err)
	}
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("recreate projection state: %w", err)
	}
	return tx.Commit()
}

// Migrate brings a database stamped at an older schema version up to this
// build, for version bumps that touched only projections.
//
// The alternative an operator otherwise has is deleting the file, and the
// file holds the spine -- the sole authority, carrying observations of
// processes that can never be observed again. A projection-only bump does not
// require that, and offering it as the only option would be the most
// expensive kind of data loss this design was built to prevent.
func Migrate(s *Store) (int, ProjectResult, error) {
	var from int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&from); err != nil {
		return 0, ProjectResult{}, err
	}
	if from == SchemaVersion {
		return from, ProjectResult{}, nil
	}
	if from > SchemaVersion {
		return from, ProjectResult{}, fmt.Errorf("database is schema v%d, newer than this build's v%d", from, SchemaVersion)
	}
	if err := s.resetProjections(); err != nil {
		return from, ProjectResult{}, err
	}
	if _, err := s.db.Exec(fmt.Sprintf("PRAGMA user_version=%d", SchemaVersion)); err != nil {
		return from, ProjectResult{}, fmt.Errorf("stamp schema version: %w", err)
	}
	res, err := Project(s)
	return from, res, err
}
