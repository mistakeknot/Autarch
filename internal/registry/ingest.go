package registry

import (
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
	"syscall"
	"time"
)

// SourceClaudeSessions is the producer id for the read-only watcher over the
// Claude session directory. It writes nothing there and holds no lock; the
// provider is the sole author of those files.
const SourceClaudeSessions = "claude-sessions"

// Store is the write side of the registry. Every write goes through here so
// the ingest idioms -- ON CONFLICT rather than OR IGNORE, scans bracketing
// their events -- have one definition.
type Store struct {
	db   *sql.DB
	host string
	now  func() int64
}

// NewStore wraps an open registry database. host identifies the machine these
// observations came from; it is part of every conversation's identity, so an
// estate spanning two machines cannot merge into one history.
func NewStore(db *sql.DB, host string) *Store {
	return &Store{db: db, host: host, now: func() int64 { return time.Now().UnixMilli() }}
}

// DB exposes the handle for readers and tests.
func (s *Store) DB() *sql.DB { return s.db }

// Host returns the machine these observations are attributed to.
func (s *Store) Host() string { return s.host }

// DefaultHost is the machine name, normalised. os.Hostname returns
// "Clavain.local" under mDNS while every other reference to the machine says
// "clavain", and an identity that depends on which one answered is not an
// identity.
func DefaultHost() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "unknown-host"
	}
	return strings.ToLower(strings.TrimSuffix(name, ".local"))
}

// DefaultSessionDir is where the Claude CLI publishes its session records.
func DefaultSessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude/sessions"
	}
	return filepath.Join(home, ".claude", "sessions")
}

// EnsureSource registers a producer. It is deliberately not an upsert of the
// liveness columns: a restart must not reset a source's history, and a source
// that has never succeeded must keep reading as unchecked.
func (s *Store) EnsureSource(sourceID, kind, locator string, expectedIntervalMs int64) error {
	var interval any
	if expectedIntervalMs > 0 {
		interval = expectedIntervalMs
	}
	_, err := s.db.Exec(`
		INSERT INTO source (source_id, host, kind, locator, expected_interval_ms)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET locator = excluded.locator,
		                                     expected_interval_ms = excluded.expected_interval_ms`,
		sourceID, s.host, kind, locator, interval)
	if err != nil {
		return fmt.Errorf("ensure source %s: %w", sourceID, err)
	}
	return nil
}

// BeginScan opens a sweep and returns its id. Events carry it, so an absence
// claim can cite the sweep that supports it.
func (s *Store) BeginScan(sourceID string) (int64, error) {
	now := s.now()
	if _, err := s.db.Exec(`UPDATE source SET last_attempt_ms = ? WHERE source_id = ?`, now, sourceID); err != nil {
		return 0, fmt.Errorf("mark attempt: %w", err)
	}
	res, err := s.db.Exec(`INSERT INTO source_scan (source_id, started_ms) VALUES (?, ?)`, sourceID, now)
	if err != nil {
		return 0, fmt.Errorf("begin scan: %w", err)
	}
	return res.LastInsertId()
}

// FinishScan closes a sweep. A sweep that errored is never complete, and a
// source that errored never claims ok -- which is what keeps "we could not
// look" from rendering as "there is nothing there."
func (s *Store) FinishScan(scanID int64, sourceID string, complete bool, recordsSeen int, scanErr error) error {
	now := s.now()
	var errText any
	if scanErr != nil {
		errText = scanErr.Error()
		complete = false
	}
	flag := 0
	if complete {
		flag = 1
	}
	if _, err := s.db.Exec(`UPDATE source_scan SET finished_ms = ?, complete = ?, records_seen = ?, error = ?
		WHERE scan_id = ?`, now, flag, recordsSeen, errText, scanID); err != nil {
		return fmt.Errorf("finish scan: %w", err)
	}
	if scanErr != nil {
		_, err := s.db.Exec(`UPDATE source SET status = 'error', last_error = ? WHERE source_id = ?`,
			scanErr.Error(), sourceID)
		return err
	}
	_, err := s.db.Exec(`UPDATE source SET status = 'ok', last_success_ms = ?, last_error = NULL
		WHERE source_id = ?`, now, sourceID)
	return err
}

// Event is one observation destined for the log.
type Event struct {
	SourceID       string
	ScanID         int64
	DedupeKey      string
	Kind           string
	OccurredMs     int64 // 0 means the provider offered no timestamp
	ConversationID string
	InstanceID     string
	Payload        []byte
}

// AppendEvent writes an event, returning its id and whether this call was the
// one that inserted it.
//
// ON CONFLICT targets the duplicate specifically. INSERT OR IGNORE would also
// swallow NOT NULL and CHECK failures, turning a write that could not happen
// into "nothing new" -- the exact suppression this registry exists to prevent.
func (s *Store) AppendEvent(e Event) (int64, bool, error) {
	var occurred, scan, conv, inst any
	if e.OccurredMs != 0 {
		occurred = e.OccurredMs
	}
	if e.ScanID != 0 {
		scan = e.ScanID
	}
	if e.ConversationID != "" {
		conv = e.ConversationID
	}
	if e.InstanceID != "" {
		inst = e.InstanceID
	}
	payload := e.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}

	res, err := s.db.Exec(`
		INSERT INTO event (source_id, scan_id, dedupe_key, kind, occurred_ms, observed_ms,
		                   conversation_id, instance_id, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id, dedupe_key) DO NOTHING`,
		e.SourceID, scan, e.DedupeKey, e.Kind, occurred, s.now(), conv, inst, string(payload))
	if err != nil {
		return 0, false, fmt.Errorf("append event %s: %w", e.DedupeKey, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var id int64
		err := s.db.QueryRow(`SELECT event_id FROM event WHERE source_id = ? AND dedupe_key = ?`,
			e.SourceID, e.DedupeKey).Scan(&id)
		return id, false, err
	}
	id, err := res.LastInsertId()
	return id, true, err
}

// AddEvidence records the raw observation behind an event. Bodies are never
// stored here: the session records are structured provider metadata and go in
// the payload, while anything free-form must pass through redaction first.
func (s *Store) AddEvidence(eventID int64, kind, sourceID, locator, sha string) error {
	_, err := s.db.Exec(`INSERT INTO evidence (event_id, kind, source_id, locator, content_sha256, captured_ms)
		VALUES (?, ?, ?, ?, ?, ?)`, eventID, kind, sourceID, locator, sha, s.now())
	return err
}

// ---------------------------------------------------------------- the record

// SessionRecord is the Claude CLI's published session file.
//
// The format is undocumented, so it is treated as version-stamped evidence
// with a loud fallback, never as a contract. The full raw bytes go into the
// event payload; these fields are only what the projections read.
type SessionRecord struct {
	PID             int64  `json:"pid"`
	SessionID       string `json:"sessionId"`
	CWD             string `json:"cwd"`
	StartedAt       int64  `json:"startedAt"`
	ProcStart       string `json:"procStart"`
	Version         string `json:"version"`
	Kind            string `json:"kind"`
	Entrypoint      string `json:"entrypoint"`
	PIDDomain       string `json:"pidDomain"`
	Tmux            string `json:"tmux"`
	MessagingSocket string `json:"messagingSocketPath"`
	Name            string `json:"name"`
	NameSource      string `json:"nameSource"`
	NameSince       int64  `json:"nameSince"`
	Status          string `json:"status"`
	UpdatedAt       int64  `json:"updatedAt"`
	StatusUpdatedAt int64  `json:"statusUpdatedAt"`
	BridgeSessionID string `json:"bridgeSessionId"`
	WaitingFor      string `json:"waitingFor"`
}

// TmuxRef is a pane location as a session record claims it, with no live
// server consulted.
type TmuxRef struct {
	SessionName string
	WindowID    string
	PaneID      string
}

// ParseTmuxRef reads the provider's "<session>:@<window>.%<pane>" string.
//
// Split on the LAST colon, not the first: session names on this estate carry
// brackets, pipes, spaces and trailing separators -- "iterm]shadow-workipedia - "
// ends in a space before its colon, and splitting leftmost would truncate it.
func ParseTmuxRef(s string) (TmuxRef, bool) {
	cut := strings.LastIndex(s, ":")
	if cut < 0 {
		return TmuxRef{}, false
	}
	name, loc := s[:cut], s[cut+1:]
	dot := strings.LastIndex(loc, ".")
	if dot < 0 {
		return TmuxRef{}, false
	}
	window, pane := loc[:dot], loc[dot+1:]
	if !strings.HasPrefix(window, "@") || !strings.HasPrefix(pane, "%") {
		return TmuxRef{}, false
	}
	return TmuxRef{SessionName: name, WindowID: window, PaneID: pane}, true
}

// ---------------------------------------------------------------- the sweep

// ScanResult reports what one sweep saw. Complete is false whenever anything
// prevented a full enumeration, and only a complete sweep may support a claim
// that something is gone.
type ScanResult struct {
	ScanID      int64
	Complete    bool
	RecordsSeen int
	Inserted    int
	Skipped     int
	Unparsable  []string
	Err         error
}

// ScanClaudeSessions reads the session directory once and appends an event per
// record. It opens no file for writing and takes no lock.
//
// A directory it cannot read produces an incomplete scan and an error on the
// source. It never produces an empty estate.
func ScanClaudeSessions(s *Store, dir string) (ScanResult, error) {
	if err := s.EnsureSource(SourceClaudeSessions, "session_file", dir, 30_000); err != nil {
		return ScanResult{}, err
	}
	scanID, err := s.BeginScan(SourceClaudeSessions)
	if err != nil {
		return ScanResult{}, err
	}
	out := ScanResult{ScanID: scanID}

	entries, err := os.ReadDir(dir)
	if err != nil {
		out.Err = err
		_ = s.FinishScan(scanID, SourceClaudeSessions, false, 0, err)
		return out, fmt.Errorf("read session dir %s: %w", dir, err)
	}

	// Deterministic order, so a replay of one sweep assigns event ids in the
	// same sequence it did the first time.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var firstFailure error
	// Every instance this sweep laid eyes on, whether or not it had changed.
	// Dedupe means an unchanged record emits no event, so a roster built from
	// events alone would read "unchanged" as "gone" -- which is the exact
	// confusion between silence and absence this registry exists to refuse.
	seen := make([]string, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			// A record we could not read is a gap in this sweep, so the sweep
			// is not complete and cannot support an absence.
			out.Unparsable = append(out.Unparsable, name)
			if firstFailure == nil {
				firstFailure = err
			}
			continue
		}
		var rec SessionRecord
		if err := json.Unmarshal(raw, &rec); err != nil || rec.PID == 0 || rec.SessionID == "" {
			out.Unparsable = append(out.Unparsable, name)
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: unrecognised session record", name)
			}
			continue
		}
		out.RecordsSeen++

		instID := InstanceID(s.host, rec.PIDDomain, rec.PID, rec.StartedAt)
		seen = append(seen, instID)

		sum := sha256.Sum256(raw)
		sha := hex.EncodeToString(sum[:])
		id, inserted, err := s.AppendEvent(Event{
			SourceID:       SourceClaudeSessions,
			ScanID:         scanID,
			DedupeKey:      "session:" + name + ":" + sha[:16],
			Kind:           "session.observed",
			OccurredMs:     rec.UpdatedAt,
			ConversationID: ConversationID("claude", s.host, rec.SessionID),
			InstanceID:     instID,
			Payload:        raw,
		})
		if err != nil {
			out.Err = err
			_ = s.FinishScan(scanID, SourceClaudeSessions, false, out.RecordsSeen, err)
			return out, err
		}
		if inserted {
			out.Inserted++
			if err := s.AddEvidence(id, "session_file", SourceClaudeSessions, path, sha); err != nil {
				out.Err = err
				_ = s.FinishScan(scanID, SourceClaudeSessions, false, out.RecordsSeen, err)
				return out, err
			}
		} else {
			out.Skipped++
		}
	}

	out.Complete = firstFailure == nil
	if out.Complete {
		// A sweep that enumerated everything says so in the log, not only in
		// the scan table. An absence claim has to cite an event, and without
		// this a sweep that saw nothing could cite nothing -- so an estate
		// that genuinely emptied could never be recorded as having emptied.
		roster, err := json.Marshal(struct {
			RecordsSeen int      `json:"records_seen"`
			Instances   []string `json:"instances"`
		}{out.RecordsSeen, seen})
		if err != nil {
			out.Err = err
			_ = s.FinishScan(scanID, SourceClaudeSessions, false, out.RecordsSeen, err)
			return out, err
		}
		if _, _, err := s.AppendEvent(Event{
			SourceID:  SourceClaudeSessions,
			ScanID:    scanID,
			DedupeKey: fmt.Sprintf("scan:%d", scanID),
			Kind:      "scan.completed",
			Payload:   roster,
		}); err != nil {
			out.Err = err
			_ = s.FinishScan(scanID, SourceClaudeSessions, false, out.RecordsSeen, err)
			return out, err
		}
	}
	if err := s.FinishScan(scanID, SourceClaudeSessions, out.Complete, out.RecordsSeen, firstFailure); err != nil {
		return out, err
	}
	out.Err = firstFailure
	return out, nil
}

// processAlive reports whether a pid currently exists.
//
// It is never sufficient on its own: pids recur, so a live pid may belong to
// something else entirely. It is used only to withhold a closure that a
// complete scan would otherwise justify -- it can keep a record open, never
// close one.
func processAlive(pid int64) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(int(pid), 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
