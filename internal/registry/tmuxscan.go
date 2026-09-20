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
	"strconv"
	"strings"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
)

// SourceTmuxInventory is the producer id for the tmux pane sweep. It is the
// only thing that can turn a session record's claim about where it lives into
// a verified binding.
const SourceTmuxInventory = "tmux-inventory"

// DefaultTmuxSocket is the canonical server's socket.
//
// Not os.TempDir(): that honours $TMPDIR, which on macOS is a per-user folder
// under /var, while tmux puts its socket under /tmp regardless. Pointing the
// inventory at $TMPDIR finds no server and -- before the emptiness rule below
// -- reported a live estate of 106 panes as zero.
//
// $TMUX wins when set, since it names the server this process is actually
// inside. Resolved through CanonicalSocket because tmux reports /private/tmp
// while the conventional path says /tmp, and a binding keyed on one cannot
// match a binding keyed on the other.
func DefaultTmuxSocket() string {
	if env := os.Getenv("TMUX"); env != "" {
		if socket, _, ok := strings.Cut(env, ","); ok && socket != "" {
			return CanonicalSocket(socket)
		}
	}
	return CanonicalSocket(filepath.Join("/tmp", fmt.Sprintf("tmux-%d", os.Getuid()), "default"))
}

// ScanTmuxPanes reads the live pane inventory once.
//
// A tmux that cannot be reached produces an incomplete sweep and an error on
// the source, and changes nothing else. No binding is closed, no instance is
// ended, and no conversation is demoted: an inventory that failed is not an
// inventory that came back empty.
func ScanTmuxPanes(s *Store, socket string) (ScanResult, error) {
	return ScanTmuxPanesWith(s, socket, nil)
}

// ScanTmuxPanesWith is ScanTmuxPanes against a supplied command runner, so the
// empty-inventory and failed-read paths can be exercised without a live server.
func ScanTmuxPanesWith(s *Store, socket string, runner agenttransport.Runner) (ScanResult, error) {
	if err := s.EnsureSource(SourceTmuxInventory, "tmux_inventory", socket, 30_000); err != nil {
		return ScanResult{}, err
	}
	scanID, err := s.BeginScan(SourceTmuxInventory)
	if err != nil {
		return ScanResult{}, err
	}
	out := ScanResult{ScanID: scanID}

	panes, err := agenttransport.NewTmux(runner, socket).List(context.Background())
	if err == nil && len(panes) == 0 {
		// A running tmux server always has at least one pane, so an empty
		// inventory means we did not reach the server -- the transport folds
		// "no server running" and "cannot connect" into an empty success.
		// Reported as zero, that is precisely "we could not look" rendering
		// as "there is nothing there"; it turned a live 106-pane estate into
		// a complete sweep of nothing before this check existed.
		err = fmt.Errorf("no panes returned: the server at %s was not reached", socket)
	}
	if err != nil {
		out.Err = err
		_ = s.FinishScan(scanID, SourceTmuxInventory, false, 0, err)
		return out, fmt.Errorf("tmux inventory on %s: %w", socket, err)
	}

	// Deterministic order, so a replay assigns event ids in the same sequence.
	sort.Slice(panes, func(i, j int) bool { return panes[i].Target.PaneKey() < panes[j].Target.PaneKey() })

	// Dedupe against each pane's PREVIOUS observation, not against all of
	// history. A content-addressed key over history suppresses a revert: a
	// pane going A, B, then back to A emits nothing for the return, and the
	// latest recorded observation stays B -- stale data read as current.
	lastSeen, err := s.lastPaneObservations()
	if err != nil {
		out.Err = err
		_ = s.FinishScan(scanID, SourceTmuxInventory, false, 0, err)
		return out, err
	}

	// The roster this sweep will publish: the one thing that makes a pane's
	// ABSENCE recordable. Without it nothing in the log ever says "a complete
	// sweep of this server ran and did not list that pane", so a binding could
	// only ever be closed by its instance dying -- and a claim could be
	// verified against an observation from a pane that died days ago.
	var server agenttransport.Target
	roster := []string{}
	for _, p := range panes {
		t := p.Target
		t.Socket = CanonicalSocket(t.Socket)
		if err := t.Validate(); err != nil {
			// An incompletely identified pane cannot verify anything, and
			// pretending otherwise would forge a binding.
			out.Unparsable = append(out.Unparsable, p.Target.PaneID)
			continue
		}
		if server.Socket == "" {
			server = t
		} else if t.Socket != server.Socket || t.ServerPID != server.ServerPID ||
			t.ServerStarted != server.ServerStarted {
			// One List call reaches one server, so this cannot happen -- and
			// if it does, the roster's scope is not what it claims and the
			// sweep must not authorise any absence. Counted as unparsable so
			// the sweep is incomplete and publishes no roster at all.
			out.Unparsable = append(out.Unparsable, t.PaneID)
			continue
		}
		// Factored: every pane on one server shares its socket, pid and start,
		// so the roster names them once and lists only what differs. Stored
		// whole, 107 panes cost ~6KB per sweep and 18MB a day at a 30s
		// interval; factored they cost ~1KB and 3MB. The full pane key is
		// reconstructed by paneRoster.index, the only other place that knows
		// the layout.
		entry := t.PaneID + ":" + strconv.FormatInt(t.PanePID, 10)
		if p.Dead {
			// remain-on-exit keeps a pane listed after its process is gone, at
			// the pid of the process that exited. Present, and not a place
			// anything is running: recorded as its own fact rather than folded
			// into either, because folding it into "absent" would close a
			// binding on a pane that is still there, and folding it into
			// "present" is what makes a dead pane read as live.
			entry += ":dead"
		}
		roster = append(roster, entry)
		out.RecordsSeen++

		body, err := json.Marshal(struct {
			agenttransport.Target
			SessionName string `json:"session_name"`
			Command     string `json:"command"`
			Dead        bool   `json:"dead"`
		}{t, p.SessionName, p.Command, p.Dead})
		if err != nil {
			out.Err = err
			_ = s.FinishScan(scanID, SourceTmuxInventory, false, out.RecordsSeen, err)
			return out, err
		}
		sum := sha256.Sum256(body)
		digest := hex.EncodeToString(sum[:])[:16]
		if lastSeen[t.PaneID] == digest {
			out.Skipped++
			continue
		}
		if _, inserted, err := s.AppendEvent(Event{
			SourceID: SourceTmuxInventory,
			ScanID:   scanID,
			// The scan id makes a revert a distinct key, so A -> B -> A
			// records the return rather than colliding with the first A.
			DedupeKey: fmt.Sprintf("pane|%s|%s|%d", digest, t.PaneID, scanID),
			Kind:      "pane.observed",
			Payload:   body,
		}); err != nil {
			out.Err = err
			_ = s.FinishScan(scanID, SourceTmuxInventory, false, out.RecordsSeen, err)
			return out, err
		} else if inserted {
			out.Inserted++
		} else {
			out.Skipped++
		}
	}

	out.Complete = len(out.Unparsable) == 0
	if out.Complete {
		// Published only by a complete sweep, and only by one that reached a
		// server: a roster is a claim about what IS there, and every pane it
		// omits is a pane it is asserting is gone. A sweep that could not
		// identify one pane has no standing to assert that about the other
		// hundred and six.
		//
		// Sorted so a replay of one sweep hashes and reads identically.
		sort.Strings(roster)
		body, err := json.Marshal(struct {
			SourceID      string    `json:"source_id"`
			Host          string    `json:"host"`
			Socket        string    `json:"socket"`
			ServerPID     int64     `json:"server_pid"`
			ServerStarted int64     `json:"server_started"`
			RecordsSeen   int       `json:"records_seen"`
			Panes         *[]string `json:"panes"`
		}{SourceTmuxInventory, s.host, server.Socket, server.ServerPID, server.ServerStarted,
			out.RecordsSeen, &roster})
		if err != nil {
			out.Err = err
			_ = s.FinishScan(scanID, SourceTmuxInventory, false, out.RecordsSeen, err)
			return out, err
		}
		if _, _, err := s.AppendEvent(Event{
			SourceID:  SourceTmuxInventory,
			ScanID:    scanID,
			DedupeKey: fmt.Sprintf("scan:%d", scanID),
			Kind:      "scan.completed",
			Payload:   body,
		}); err != nil {
			out.Err = err
			_ = s.FinishScan(scanID, SourceTmuxInventory, false, out.RecordsSeen, err)
			return out, err
		}
	}
	// An incomplete sweep marks its source, exactly as the sessions path does.
	// A sweep that could not identify every pane is a gap, and a source that
	// reads ok after a gap is the quiet lie this registry exists to refuse.
	var gap error
	if !out.Complete {
		gap = fmt.Errorf("%d pane(s) incompletely identified: %v", len(out.Unparsable), out.Unparsable)
	}
	if err := s.FinishScan(scanID, SourceTmuxInventory, out.Complete, out.RecordsSeen, gap); err != nil {
		return out, err
	}
	out.Err = gap
	return out, nil
}

// ---------------------------------------------------------------- the roster

// paneRoster is what one complete tmux sweep published: the server it reached
// and every pane that server listed.
//
// It is the only record of a pane's ABSENCE. Nothing else can be: the sweep
// dedupes unchanged panes, so the events it emits describe only what moved,
// and a pane that vanished emits nothing at all. Deriving absence from missing
// events would read a quiet pane as a dead one -- the same mistake the session
// roster exists to prevent, in the other half of the estate.
type paneRoster struct {
	SourceID      string `json:"source_id"`
	Host          string `json:"host"`
	Socket        string `json:"socket"`
	ServerPID     int64  `json:"server_pid"`
	ServerStarted int64  `json:"server_started"`
	RecordsSeen   int    `json:"records_seen"`
	// A pointer so an absent field is distinguishable from an empty list. A
	// missing roster unmarshals to nil and is refused; an empty one would say
	// the server has no panes, which is a thing a running server cannot be.
	Panes *[]string `json:"panes"`
}

// index reads the factored roster back into pane id -> pane pid.
//
// The layout is written in exactly one other place, where the sweep builds it.
// A line that does not parse is dropped rather than guessed at, and the caller
// is told, because a roster read as smaller than it is would close bindings
// for panes that were listed.
func (r paneRoster) index() (map[string]int64, int) {
	out := map[string]int64{}
	bad := 0
	if r.Panes == nil {
		return out, 0
	}
	for _, entry := range *r.Panes {
		id, rest, ok := strings.Cut(entry, ":")
		if !ok || !strings.HasPrefix(id, "%") {
			bad++
			continue
		}
		// A trailing ":dead" is the pane's own state, not part of its
		// identity: the pane is listed either way, which is what presence
		// means here.
		pidText, _, _ := strings.Cut(rest, ":")
		pid, err := strconv.ParseInt(pidText, 10, 64)
		if err != nil {
			bad++
			continue
		}
		out[id] = pid
	}
	return out, bad
}

// latestPaneRoster returns the most recent complete tmux sweep at or before
// the event being applied.
//
// Bounded by event id so a replay sees exactly the knowledge the original pass
// had. Reaching forward would let a rebuild verify a binding using a sweep
// that had not happened yet, which is the same class of error as probing the
// live process table during replay.
func (s *Store) latestPaneRoster(ex execer, eventID int64) (rosterAt, bool, error) {
	var at rosterAt
	var payload string
	err := ex.QueryRow(`SELECT event_id, observed_ms, payload FROM event
		 WHERE kind = 'scan.completed' AND source_id = ? AND event_id <= ?
		 ORDER BY event_id DESC LIMIT 1`, SourceTmuxInventory, eventID).
		Scan(&at.eventID, &at.observedMs, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return rosterAt{}, false, nil
	}
	if err != nil {
		return rosterAt{}, false, fmt.Errorf("read latest pane roster: %w", err)
	}
	if err := json.Unmarshal([]byte(payload), &at.roster); err != nil {
		return rosterAt{}, false, nil
	}
	if at.roster.Panes == nil || at.roster.Socket == "" {
		return rosterAt{}, false, nil
	}
	at.present, _ = at.roster.index()
	return at, true, nil
}

// rosterAt is a roster together with the event that published it, so anything
// that acts on the roster can cite the sweep rather than the moment it looked.
type rosterAt struct {
	roster     paneRoster
	present    map[string]int64
	eventID    int64
	observedMs int64
}

// ---------------------------------------------------------------- verifying

// verifyPane upgrades a claimed binding in place once the live server confirms
// where that pane actually is. It returns how many claims it refused to
// upgrade, which is a fact an operator needs and which silence would hide.
//
// In place, not as a new row: the binding is the same observation, better
// identified. Closing the claim and opening a verified one would invent a
// discontinuity the agent never experienced. The generated pane_key recomputes
// on the update, so the row moves from its 'claimed:' key onto the real one.
//
// Two things it does NOT do, both corrected at schema v4:
//
// It no longer matches on pane id alone. A pane id is unique only within one
// server, and '%67' exists on every tmux server that has ever had 67 panes. On
// this estate there is one socket, so the old query was right by accident;
// the moment a second server appears it would upgrade a claim using another
// machine's pane and the binding would point somewhere real and wrong. The
// claim carries no server identity of its own -- that is what makes it a claim
// -- so the corroboration available is the window id the provider wrote in the
// same breath as the pane id. Measured 2026-09-19 across every live record:
// claimed window agrees with the live server 11 times out of 11.
//
// And it no longer overwrites what the record claimed. Session name is the
// reason: it disagrees 3 times in those same 11, and all three disagreements
// are the same pane -- 'tmux-organizer' against 'iterm[autarch - e4be...',
// 'iterm]' against 'iterm[]', and one trailing space. Those are two true
// readings of one pane, not a correction, and a row that adopts the server's
// answer can no longer be asked whether the two ever differed. That is also
// why the window id gates verification while the session name cannot: one
// disagrees never, the other disagrees a quarter of the time.
func (s *Store) verifyPane(ex execer, eventID int64, payload string, at *rosterAt) (int, error) {
	var obs struct {
		agenttransport.Target
		SessionName string `json:"session_name"`
	}
	if err := json.Unmarshal([]byte(payload), &obs); err != nil {
		return 0, fmt.Errorf("pane payload in event %d: %w", eventID, err)
	}
	if obs.PaneID == "" {
		return 0, nil
	}
	// An observation missing any part of its server identity cannot verify
	// anything: writing it would produce a row whose pane_key falls back to
	// 'claimed:' while claiming basis 'tmux_inventory', which the table's own
	// CHECK refuses -- correctly.
	if err := obs.Target.Validate(); err != nil {
		return 0, nil
	}

	// Already-verified rows first: refresh what the server says about a pane
	// this observation identifies by its FULL key. Without this a verified
	// binding froze at the moment of verification, so after a break-pane,
	// join-pane, cross-window swap-pane or session rename it kept the old
	// window and session indefinitely -- while the roster went on refreshing
	// last_present_ms, because a roster carries only %id:pane_pid. The row
	// then read "confirmed present 30 seconds ago" beside a window the pane
	// left days ago, and window_id is exactly what an exposure would join on.
	if _, err := ex.Exec(`
		UPDATE pane_binding
		   SET tmux_session_id = ?, window_id = ?, session_name_seen = ?, last_event_id = ?
		 WHERE observed_to_ms IS NULL
		   AND binding_basis = 'tmux_inventory'
		   AND socket = ? AND server_pid = ? AND server_started = ?
		   AND pane_id = ? AND pane_pid = ?`,
		obs.SessionID, obs.WindowID, obs.SessionName, eventID,
		obs.Socket, obs.ServerPID, obs.ServerStarted, obs.PaneID, obs.PanePID); err != nil {
		return 0, err
	}

	res, err := ex.Exec(`
		UPDATE pane_binding
		   SET socket = ?, server_pid = ?, server_started = ?, pane_pid = ?,
		       tmux_session_id = ?, window_id = ?, session_name_seen = ?,
		       binding_basis = 'tmux_inventory', last_event_id = ?,
		       corroborated_by = CASE
		         WHEN (SELECT li.ppid FROM launch_instance li
		                WHERE li.instance_id = pane_binding.instance_id) = ?
		         THEN 'parent_pid' ELSE NULL END
		 WHERE observed_to_ms IS NULL
		   AND pane_id = ?
		   AND binding_basis = 'session_file_claim'
		   AND (claimed_window_id = '' OR claimed_window_id = ?)
		   AND NOT EXISTS (SELECT 1 FROM launch_instance li, json_each(?) other
		                    WHERE li.instance_id = pane_binding.instance_id
		                      AND li.ppid IS NOT NULL AND li.ppid = other.value)`,
		obs.Socket, obs.ServerPID, obs.ServerStarted, obs.PanePID,
		obs.SessionID, obs.WindowID, obs.SessionName, eventID, obs.PanePID,
		obs.PaneID, obs.WindowID, elsewhereOnThisServer(at, obs.PaneID))
	if err != nil {
		return 0, err
	}
	upgraded, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	_ = upgraded

	// Anything still claiming this pane was refused, by the window gate or by
	// the process table. Counted rather than resolved: a pane genuinely moved
	// by break-pane looks exactly like a claim pointing at the wrong server,
	// and nothing here can tell them apart. Both stay claimed, which is the
	// honest state; the count is what makes a stuck one visible instead of
	// merely quiet.
	var refused int
	if err := ex.QueryRow(`SELECT COUNT(*) FROM pane_binding
		 WHERE observed_to_ms IS NULL AND pane_id = ?
		   AND binding_basis = 'session_file_claim'`, obs.PaneID).Scan(&refused); err != nil {
		return 0, err
	}
	return refused, nil
}

// elsewhereOnThisServer renders, as a JSON array, the root pids of every pane
// on the swept server EXCEPT the one being verified.
//
// A claim whose agent's parent is one of those is a claim about a different
// pane: pids are host-unique, so a parent that is some other pane's root
// process is positive evidence of a mismatch, and positive evidence is the
// only thing allowed to refuse anything here. This does not make corroboration
// a gate -- a dispatched child's parent is the shim and appears in no roster,
// so it contradicts nothing and verifies as before.
//
// It is the only mitigation available for a claim arriving from a server this
// registry does not sweep. The window gate cannot see that case at all: window
// ids are per-server counters exactly as pane ids are, so another server's
// @0.%0 passes against this server's @0.%0, and an estate with one socket
// cannot sample how often that happens.
func elsewhereOnThisServer(at *rosterAt, paneID string) string {
	if at == nil {
		return "[]"
	}
	pids := make([]int64, 0, len(at.present))
	for id, pid := range at.present {
		if id != paneID {
			pids = append(pids, pid)
		}
	}
	sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
	body, err := json.Marshal(pids)
	if err != nil {
		return "[]"
	}
	return string(body)
}

// verifyFromLatestObservation verifies a binding that was just claimed, using
// the most recent pane observation the log already holds.
//
// Without this a claim can wait indefinitely. The inventory dedupes unchanged
// panes, so a pane that has not moved emits no event -- and a conversation
// that arrives in an already-quiet pane would stay merely "claimed" until that
// pane happened to change. The same shape as a deduped session record reading
// as a departed agent: an absence of new data taken for an absence of fact.
//
// The freshness rule is the roster, not the clock. "Recent" cannot mean a
// young observation, because a pane that has sat unchanged for a week has a
// week-old observation and is perfectly alive. It means: the last complete
// sweep listed this pane, at this pane pid, on this server. A pane that died
// days ago is absent from that roster no matter how vivid its last
// observation was, and before this check a fresh claim could be verified
// against it -- the registry asserting a live agent sat in a pane that no
// longer existed.
func (s *Store) verifyFromLatestObservation(ex execer, eventID int64, paneID string) (int, error) {
	at, ok, err := s.latestPaneRoster(ex, eventID)
	if err != nil {
		return 0, err
	}
	if !ok {
		// No complete sweep has ever run, so nothing has been confirmed and
		// the claim stays a claim. Not an error: it is the true state.
		return 0, nil
	}
	roster := at.roster
	panePID, listed := at.present[paneID]
	if !listed {
		return 0, nil
	}

	var payload string
	err = ex.QueryRow(`SELECT payload FROM event
		WHERE kind = 'pane.observed'
		  AND event_id <= ?
		  AND json_extract(payload, '$.pane_id') = ?
		  AND json_extract(payload, '$.socket') = ?
		  AND json_extract(payload, '$.server_pid') = ?
		  AND json_extract(payload, '$.server_started') = ?
		  AND json_extract(payload, '$.pane_pid') = ?
		ORDER BY event_id DESC LIMIT 1`,
		eventID, paneID, roster.Socket, roster.ServerPID, roster.ServerStarted, panePID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		// The roster lists the pane but no observation of it at that identity
		// survives at or before this event. Nothing to verify from, and
		// inventing one from the roster alone would forge the window and
		// session the roster does not carry.
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("look up pane %s: %w", paneID, err)
	}
	refused, err := s.verifyPane(ex, eventID, payload, &at)
	if err != nil {
		return refused, err
	}
	// Presence comes from the roster that authorised this verification, and
	// cites it. Leaving it unset would produce the one state last_present_ms
	// exists to abolish -- a binding marked verified that nothing has ever
	// confirmed is present -- when in fact the sweep we just consulted listed
	// the pane. The timestamp is that sweep's, not this moment's, so a replay
	// reproduces it exactly.
	_, err = ex.Exec(`UPDATE pane_binding
		SET last_present_ms = ?, last_present_event_id = ?
		WHERE observed_to_ms IS NULL AND pane_id = ?
		  AND binding_basis = 'tmux_inventory' AND last_event_id = ?`,
		at.observedMs, at.eventID, paneID, eventID)
	return refused, err
}

// ---------------------------------------------------------------- absence

// reconcilePaneAbsences closes the bindings a complete sweep did not list, and
// refreshes the presence timestamp of the ones it did.
//
// Scoped to one server incarnation. A sweep of this socket says nothing about
// a pane on another socket, and nothing about panes that belonged to a
// previous server on this one: those died with their server, and they are
// closed by their instances ending rather than by an absence this sweep never
// observed.
//
// Claims are never closed here. A session_file_claim carries no server
// identity, so "not on the server we swept" and "on a server we did not sweep"
// are the same observation, and closing on it would be exactly the inference
// this registry refuses. A claim in a dead pane is closed when its instance
// ends, which is a positive fact about the process.
func (s *Store) reconcilePaneAbsences(ex execer, eventID, observedMs int64, payload string) (int, string, error) {
	var roster paneRoster
	if err := json.Unmarshal([]byte(payload), &roster); err != nil {
		return 0, fmt.Sprintf("pane roster in event %d will not parse (%v); refusing to read that as an empty server", eventID, err), nil
	}
	if roster.Panes == nil {
		return 0, fmt.Sprintf("pane roster in event %d has no pane list; refusing to read that as an empty server", eventID), nil
	}
	if roster.Socket == "" || roster.ServerPID == 0 || roster.ServerStarted == 0 {
		return 0, fmt.Sprintf("pane roster in event %d names no server, so its scope is unknown", eventID), nil
	}
	present, bad := roster.index()
	if bad > 0 {
		// A roster read as smaller than it is closes bindings for panes that
		// were listed. Nothing is worth salvaging from a partial read.
		return 0, fmt.Sprintf("pane roster in event %d has %d unreadable entries; refusing to act on a partial list", eventID, bad), nil
	}
	if len(present) == 0 {
		return 0, fmt.Sprintf("pane roster in event %d lists no panes, which a running server cannot do", eventID), nil
	}

	rows, err := ex.Query(`SELECT binding_id, pane_id, pane_pid FROM pane_binding
		 WHERE observed_to_ms IS NULL
		   AND binding_basis = 'tmux_inventory'
		   AND socket = ? AND server_pid = ? AND server_started = ?`,
		roster.Socket, roster.ServerPID, roster.ServerStarted)
	if err != nil {
		return 0, "", fmt.Errorf("find open bindings: %w", err)
	}
	type open struct {
		id      int64
		paneID  string
		panePID int64
	}
	var bindings []open
	for rows.Next() {
		var b open
		if err := rows.Scan(&b.id, &b.paneID, &b.panePID); err != nil {
			rows.Close()
			return 0, "", err
		}
		bindings = append(bindings, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, "", err
	}

	closed := 0
	for _, b := range bindings {
		pid, listed := present[b.paneID]
		switch {
		case listed && pid == b.panePID:
			// Positive confirmation, the pane-level analogue of a liveness
			// probe answering alive.
			//
			// Corroboration is re-evaluated here rather than only at the
			// moment of verification, because it is a statement about present
			// agreement, not a historical one. Computed once at upgrade it was
			// NULL forever on every binding verified before its process had
			// been probed -- which, replaying a log that predates parent
			// capture, is all of them.
			if _, err := ex.Exec(`UPDATE pane_binding
				SET last_present_ms = ?, last_present_event_id = ?, last_event_id = ?,
				    corroborated_by = CASE
				      WHEN (SELECT li.ppid FROM launch_instance li
				             WHERE li.instance_id = pane_binding.instance_id) = pane_binding.pane_pid
				      THEN 'parent_pid' ELSE NULL END
				WHERE binding_id = ?`, observedMs, eventID, eventID, b.id); err != nil {
				return closed, "", err
			}
		case listed:
			// The pane id survives but its root process does not: the pane was
			// respawned. A distinct fact from the pane going away, and the
			// enum has carried a name for it since v3 with nothing to write
			// it. The instance is untouched -- a respawned pane says nothing
			// about whether the agent that was in it is still running.
			if err := s.closeBinding(ex, b.id, "pane_pid_changed", observedMs, eventID); err != nil {
				return closed, "", err
			}
			closed++
		default:
			if err := s.closeBinding(ex, b.id, "pane_absent_from_complete_scan", observedMs, eventID); err != nil {
				return closed, "", err
			}
			closed++
		}
	}
	return closed, "", nil
}

// retryStuckClaims gives every open claim another chance against the roster
// this sweep just published.
//
// Verification used to be attempted exactly once, when a claim was first
// inserted. A claim that missed that one chance stayed claimed however many
// complete sweeps went on listing its pane, however plainly a matching
// observation sat in the log, and however well the window agreed -- it moved
// only if the pane's own digest happened to change. One missed chance was
// permanent for a quiet pane, and a quiet pane is where a waiting agent sits.
//
// Driven by the roster rather than by a timer, so it is a positive observation
// and replays identically.
func (s *Store) retryStuckClaims(ex execer, eventID int64, at rosterAt) (int, error) {
	rows, err := ex.Query(`SELECT DISTINCT pane_id FROM pane_binding
		 WHERE observed_to_ms IS NULL AND binding_basis = 'session_file_claim'
		 ORDER BY pane_id`)
	if err != nil {
		return 0, fmt.Errorf("find stuck claims: %w", err)
	}
	var panes []string
	for rows.Next() {
		var pane string
		if err := rows.Scan(&pane); err != nil {
			rows.Close()
			return 0, err
		}
		if _, listed := at.present[pane]; listed {
			panes = append(panes, pane)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	refused := 0
	for _, pane := range panes {
		n, err := s.verifyFromLatestObservation(ex, eventID, pane)
		if err != nil {
			return refused, err
		}
		refused += n
	}
	return refused, nil
}

func (s *Store) closeBinding(ex execer, bindingID int64, basis string, endedMs, eventID int64) error {
	_, err := ex.Exec(`UPDATE pane_binding
		SET observed_to_ms = ?, end_basis = ?, end_event_id = ?, last_event_id = ?
		WHERE binding_id = ? AND observed_to_ms IS NULL`,
		endedMs, basis, eventID, eventID, bindingID)
	if err != nil {
		return fmt.Errorf("close binding %d: %w", bindingID, err)
	}
	return nil
}

// lastPaneObservations maps each pane to the digest of its most recent
// recorded observation, so a sweep can tell an unchanged pane from one that
// has returned to a state it held before.
func (s *Store) lastPaneObservations() (map[string]string, error) {
	rows, err := s.db.Query(`
		SELECT json_extract(payload, '$.pane_id'), dedupe_key FROM event
		 WHERE kind = 'pane.observed'
		   AND event_id IN (SELECT MAX(event_id) FROM event WHERE kind = 'pane.observed'
		                     GROUP BY json_extract(payload, '$.pane_id'))`)
	if err != nil {
		return nil, fmt.Errorf("read last pane observations: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var pane, key string
		if err := rows.Scan(&pane, &key); err != nil {
			return nil, err
		}
		// pane|<digest>|<pane id>|<scan id>
		if parts := strings.Split(key, "|"); len(parts) == 4 {
			out[pane] = parts[1]
		}
	}
	return out, rows.Err()
}
