package registry

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	for _, p := range panes {
		t := p.Target
		t.Socket = CanonicalSocket(t.Socket)
		if err := t.Validate(); err != nil {
			// An incompletely identified pane cannot verify anything, and
			// pretending otherwise would forge a binding.
			out.Unparsable = append(out.Unparsable, p.Target.PaneID)
			continue
		}
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
		if _, inserted, err := s.AppendEvent(Event{
			SourceID:  SourceTmuxInventory,
			ScanID:    scanID,
			DedupeKey: "pane:" + t.PaneKey() + ":" + hex.EncodeToString(sum[:])[:16],
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

// verifyPane upgrades a claimed binding in place once the live server confirms
// where that pane actually is.
//
// In place, not as a new row: the binding is the same observation, better
// identified. Closing the claim and opening a verified one would invent a
// discontinuity the agent never experienced. The generated pane_key recomputes
// on the update, so the row moves from its 'claimed:' key onto the real one.
func (s *Store) verifyPane(ex execer, eventID int64, payload string) error {
	var obs struct {
		agenttransport.Target
		SessionName string `json:"session_name"`
	}
	if err := json.Unmarshal([]byte(payload), &obs); err != nil {
		return fmt.Errorf("pane payload in event %d: %w", eventID, err)
	}
	if obs.PaneID == "" {
		return nil
	}

	_, err := ex.Exec(`
		UPDATE pane_binding
		   SET socket = ?, server_pid = ?, server_started = ?, pane_pid = ?,
		       tmux_session_id = ?, window_id = ?, session_name_seen = ?,
		       binding_basis = 'tmux_inventory', last_event_id = ?
		 WHERE observed_to_ms IS NULL
		   AND pane_id = ?
		   AND binding_basis = 'session_file_claim'`,
		obs.Socket, obs.ServerPID, obs.ServerStarted, obs.PanePID,
		obs.SessionID, obs.WindowID, obs.SessionName, eventID, obs.PaneID)
	return err
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
// Bounded to observations at or before the event being applied, so a replay
// cannot reach forward into knowledge the original pass did not have.
func (s *Store) verifyFromLatestObservation(ex execer, eventID int64, paneID string) error {
	var payload string
	err := ex.QueryRow(`SELECT payload FROM event
		WHERE kind = 'pane.observed'
		  AND event_id <= ?
		  AND json_extract(payload, '$.pane_id') = ?
		ORDER BY event_id DESC LIMIT 1`, eventID, paneID).Scan(&payload)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look up pane %s: %w", paneID, err)
	}
	return s.verifyPane(ex, eventID, payload)
}
