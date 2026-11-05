//go:build unix

package homerun

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Errors a caller distinguishes.
var (
	// ErrNonceUnavailable: the nonce was never issued, is for another host, has expired or was already
	// consumed. The cases are not told apart to a requester; none of them can be retried.
	ErrNonceUnavailable = errors.New("nonce unavailable")
	// ErrBusy: another attempt holds the card (authorized, started or uncertain), or the global cap for the privilege is reached.
	// The request is not queued and the nonce is not consumed.
	ErrBusy = errors.New("busy")
	// ErrBadTransition: an audit step that the attempt's sequence does not allow.
	ErrBadTransition = errors.New("illegal audit transition")
)

// Store is the verifier's durable state: single-use nonces, attempts and the audit table, in one SQLite
// database (WAL, synchronous=FULL). The database file belongs to the verifier alone.
type Store struct {
	db   *sql.DB
	lim  Limits
	mir  *mirror
	gate *ReviewGate
	now  func() time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS nonces(
  nonce       TEXT PRIMARY KEY,
  host        TEXT NOT NULL,
  issued_at   INTEGER NOT NULL,
  expires_at  INTEGER NOT NULL,
  consumed_at INTEGER,
  attempt     TEXT
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS attempts(
  attempt        TEXT PRIMARY KEY,
  card_id        TEXT NOT NULL,
  op             TEXT NOT NULL,
  privilege      TEXT NOT NULL,
  script_sha256  TEXT NOT NULL,
  nonce          TEXT NOT NULL UNIQUE REFERENCES nonces(nonce),
  identity       TEXT NOT NULL,
  state          TEXT NOT NULL CHECK(state IN('AUTHORIZED','STARTED','ENDED','UNCERTAIN','RESOLVED','ABORTED_BEFORE_START')),
  created_at     INTEGER NOT NULL
) WITHOUT ROWID;
DROP INDEX IF EXISTS attempts_one_active_per_card;
CREATE UNIQUE INDEX IF NOT EXISTS attempts_one_held_per_card ON attempts(card_id)
  WHERE state IN('AUTHORIZED','STARTED','UNCERTAIN');
CREATE TABLE IF NOT EXISTS audit(
  seq     INTEGER PRIMARY KEY AUTOINCREMENT,
  attempt TEXT NOT NULL REFERENCES attempts(attempt),
  kind    TEXT NOT NULL,
  body    TEXT NOT NULL
);
CREATE TRIGGER IF NOT EXISTS audit_no_update BEFORE UPDATE ON audit BEGIN SELECT RAISE(ABORT,'audit is append-only'); END;
CREATE TRIGGER IF NOT EXISTS audit_no_delete BEFORE DELETE ON audit BEGIN SELECT RAISE(ABORT,'audit is append-only'); END;
CREATE TRIGGER IF NOT EXISTS nonce_no_revive BEFORE UPDATE OF consumed_at ON nonces
  WHEN OLD.consumed_at IS NOT NULL BEGIN SELECT RAISE(ABORT,'a consumed nonce never revives'); END;
CREATE TRIGGER IF NOT EXISTS nonce_no_delete BEFORE DELETE ON nonces BEGIN SELECT RAISE(ABORT,'nonces are kept'); END;
`

// Open opens (creating, mode 0600) the state database at path. The directory must already exist.
func Open(path string, lim Limits) (*Store, error) {
	if err := lim.Validate(); err != nil {
		return nil, err
	}
	if path == "" || strings.ContainsAny(path, "?#\x00") {
		return nil, errors.New("homerun: unusable state database path")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("homerun: %w", err)
	}
	f.Close()
	dsn := "file:" + path + "?_txlock=immediate&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("homerun: %w", err)
	}
	if err := migrateReviewRefusals(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("homerun: migrate review refusals: %w", err)
	}
	if _, err := db.Exec(schema + schemaLimits + reviewSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("homerun: schema: %w", err)
	}
	return &Store{db: db, lim: lim, now: time.Now}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// SetMirror turns on the append-only JSON-lines mirror of the audit table at path and catches it up.
func (s *Store) SetMirror(ctx context.Context, path string) error {
	s.mir = &mirror{path: path}
	return s.syncMirror(ctx)
}

func (s *Store) syncMirror(ctx context.Context) error {
	if s.mir == nil {
		return nil
	}
	if err := s.mir.sync(func(after int64) ([]Record, error) { return s.recordsAfter(ctx, after) }); err != nil {
		return fmt.Errorf("%w: %v", ErrMirror, err)
	}
	return nil
}

// Nonce is a verifier-issued single-use value bound to a host.
type Nonce struct {
	Value     string
	Host      string
	IssuedAt  int64
	ExpiresAt int64
}

// IssueNonce creates a 128-bit nonce for host, valid for ttl (at most the approval TTL).
func (s *Store) IssueNonce(ctx context.Context, host string, ttl time.Duration, now time.Time) (Nonce, error) {
	if !reHex32.MatchString(host) {
		return Nonce{}, errors.New("homerun: host must be a 32-character lower-case hex machine id")
	}
	if ttl <= 0 || ttl > s.lim.ApprovalTTL {
		return Nonce{}, fmt.Errorf("homerun: nonce lifetime must be in (0, %s]", s.lim.ApprovalTTL)
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return Nonce{}, err
	}
	n := Nonce{Value: hex.EncodeToString(raw[:]), Host: host, IssuedAt: now.Unix(), ExpiresAt: now.Add(ttl).Unix()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO nonces(nonce,host,issued_at,expires_at) VALUES(?,?,?,?)`,
		n.Value, n.Host, n.IssuedAt, n.ExpiresAt)
	return n, err
}

// Attempt is a consumed nonce's execution record.
type Attempt struct {
	ID             string
	CardID         string
	Op             string
	Privilege      string
	ScriptSHA256   string
	Nonce          string
	EnvelopeDigest string
	Signer         Signer
	State          Kind
	// RuntimeMax is the signed runtime_max_s, already checked against the policy cap. The launcher sets
	// the unit's RuntimeMaxSec from it and from nothing else.
	RuntimeMax time.Duration
}

// ConsumeRequest is a verified approval ready to be spent. The envelope must already have passed
// VerifyApproval; Consume repeats the checks that touch state.
type ConsumeRequest struct {
	Env            *Envelope
	EnvelopeDigest string
	Signer         Signer
	Host           string // this machine's id; the nonce must have been issued for it
	Now            time.Time
}

type identity struct {
	EnvelopeDigest string `json:"envelope_digest"`
	Signer         Signer `json:"signer"`
	Nonce          string `json:"nonce"`
	Op             string `json:"op"`
	Privilege      string `json:"privilege"`
	CardID         string `json:"card_id"`
}

// Consume spends the envelope's nonce and records the attempt as AUTHORIZED, all in one transaction:
//
//	UPDATE nonces SET consumed_at=?, attempt=? WHERE nonce=? AND host=? AND consumed_at IS NULL AND expires_at>?
//
// must change exactly one row. Concurrent calls with one nonce result in exactly one success. When the card
// already has an active attempt or the privilege's cap is reached the call returns ErrBusy and the nonce
// is left unspent. If the audit mirror cannot be updated the attempt stays committed and ErrMirror is
// returned; the caller must not launch (a later Recover marks it ABORTED_BEFORE_START).
func (s *Store) Consume(ctx context.Context, req ConsumeRequest) (*Attempt, error) {
	e := req.Env
	if e == nil || !reHex64.MatchString(req.EnvelopeDigest) {
		return nil, errors.New("homerun: consume needs a verified envelope and its digest")
	}
	if err := e.CheckHost(req.Host); err != nil {
		return nil, err
	}
	if err := e.CheckFresh(req.Now); err != nil {
		return nil, err
	}
	// Defensive: a verified envelope is already in range, but Consume must not convert an
	// out-of-range integer to a Duration (it would wrap) or spend a nonce for it.
	if e.RuntimeMaxS < 1 || e.RuntimeMaxS > s.lim.runtimeCap(e.Op) {
		return nil, errors.New("homerun: runtime_max_s is outside the allowed range")
	}
	if err := s.requireUnlocked(ctx); err != nil {
		return nil, err
	}
	receiptExp, err := s.gateReview(ctx, req)
	if err != nil {
		return nil, err
	}
	id, err := newULID(req.Now)
	if err != nil {
		return nil, err
	}
	idJSON, err := json.Marshal(identity{EnvelopeDigest: req.EnvelopeDigest, Signer: req.Signer, Nonce: e.Nonce, Op: e.Op, Privilege: e.Privilege, CardID: e.CardID})
	if err != nil {
		return nil, err
	}
	now := req.Now.Unix()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx,
		`UPDATE nonces SET consumed_at=?, attempt=? WHERE nonce=? AND host=? AND consumed_at IS NULL AND expires_at>?`,
		now, id, e.Nonce, req.Host, now)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, ErrNonceUnavailable
	}

	// The UPDATE above took the write lock, which may have been awaited for seconds, so the receipt that
	// passed the gate is rechecked against the store's clock now. Nothing is committed yet: a refusal rolls
	// the transaction back and the nonce stays unspent.
	if s.now().Unix() >= receiptExp {
		tx.Rollback() //nolint:errcheck
		return nil, s.recordRefusal(ctx, req, &ReviewError{Reason: ReviewRefused, Detail: "review receipt expired before the nonce was spent"})
	}

	var onCard int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM attempts WHERE card_id=? AND state IN('AUTHORIZED','STARTED','UNCERTAIN')`, e.CardID).Scan(&onCard); err != nil {
		return nil, err
	}
	var onPriv int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM attempts WHERE privilege=? AND state IN('AUTHORIZED','STARTED','UNCERTAIN')`, e.Privilege).Scan(&onPriv); err != nil {
		return nil, err
	}
	capacity := s.lim.MaxActiveAsMk
	if e.Privilege == PrivRoot {
		capacity = s.lim.MaxActiveRoot
	}
	if onCard > 0 || onPriv >= capacity {
		return nil, ErrBusy
	}
	// Each refusal below rolls the transaction back, so the nonce stays unspent.
	if e.Op != OpCheck {
		if err := checkCitedReceipt(ctx, tx, s.lim, e, req.Now); err != nil {
			return nil, err
		}
	}
	if err := enforceRateLimits(ctx, tx, s.lim, e, req.Now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO attempts(attempt,card_id,op,privilege,script_sha256,nonce,identity,state,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		id, e.CardID, e.Op, e.Privilege, e.ScriptSHA256, e.Nonce, string(idJSON), string(KindAuthorized), now); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrBusy
		}
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO attempt_bind(attempt,card_rev,deps_digest) VALUES(?,?,?)`, id, e.CardRev, e.DepsDigest); err != nil {
		return nil, err
	}
	if err := insertAudit(ctx, tx, id, KindAuthorized, req.Now, string(idJSON), nil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	a := &Attempt{ID: id, CardID: e.CardID, Op: e.Op, Privilege: e.Privilege, ScriptSHA256: e.ScriptSHA256, Nonce: e.Nonce,
		EnvelopeDigest: req.EnvelopeDigest, Signer: req.Signer, State: KindAuthorized,
		RuntimeMax: time.Duration(e.RuntimeMaxS) * time.Second}
	if err := s.syncMirror(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

// insertAudit appends one record: the attempt's identity fields plus the kind-specific ones in mutate.
func insertAudit(ctx context.Context, tx *sql.Tx, attempt string, kind Kind, at time.Time, identityJSON string, mutate func(*Record)) error {
	var r Record
	if err := json.Unmarshal([]byte(identityJSON), &r); err != nil {
		return err
	}
	r.Attempt, r.Kind, r.At = attempt, kind, at.UTC().Format(time.RFC3339Nano)
	if mutate != nil {
		mutate(&r)
	}
	r.Seq = 0
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit(attempt,kind,body) VALUES(?,?,?)`, attempt, string(kind), string(body))
	return err
}

// transition moves an attempt along its audit sequence in one transaction.
func (s *Store) transition(ctx context.Context, attempt string, to Kind, at time.Time, check func(scriptSHA string) error, mutate func(*Record), after func(ctx context.Context, tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var state, ident, sha string
	if err := tx.QueryRowContext(ctx, `SELECT state, identity, script_sha256 FROM attempts WHERE attempt=?`, attempt).Scan(&state, &ident, &sha); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: unknown attempt", ErrBadTransition)
		}
		return err
	}
	ok := false
	for _, p := range legalPrev[to] {
		if Kind(state) == p {
			ok = true
		}
	}
	if !ok {
		return fmt.Errorf("%w: %s after %s", ErrBadTransition, to, state)
	}
	if check != nil {
		if err := check(sha); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE attempts SET state=? WHERE attempt=?`, string(to), attempt); err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, attempt, to, at, ident, mutate); err != nil {
		return err
	}
	if after != nil {
		if err := after(ctx, tx); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.syncMirror(ctx)
}

// UnitName is the deterministic systemd unit name for an attempt. The launcher must start the attempt's
// unit under exactly this name, so that recovery can find it without having seen STARTED.
func UnitName(attempt string) string { return "homerun-" + strings.ToLower(attempt) + ".service" }

// ErrLaunchIntent: the attempt has a recorded launch intent, so it cannot be called "never launched".
var ErrLaunchIntent = errors.New("launch was intended; reconcile with RecoverUnits")

// ErrUnreconciled is returned by RecoverUnits when an attempt with a recorded launch intent could not be
// matched to a unit (no lookup, lookup error, malformed answer). The attempt stays active and holds its
// card and its cap until a later run settles it; nothing is guessed.
var ErrUnreconciled = errors.New("attempt with a launch intent left unreconciled")

// RecordLaunchIntent durably records, before the unit is launched, that the launcher is about to start the
// attempt's unit, and returns its name (UnitName). It must have returned before the launcher starts the
// unit. It is idempotent for an attempt still AUTHORIZED and refused once the attempt has moved on.
func (s *Store) RecordLaunchIntent(ctx context.Context, attempt string, at time.Time) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback() //nolint:errcheck
	var state string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM attempts WHERE attempt=?`, attempt).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: unknown attempt", ErrBadTransition)
		}
		return "", err
	}
	if Kind(state) != KindAuthorized {
		return "", fmt.Errorf("%w: launch intent after %s", ErrBadTransition, state)
	}
	unit := UnitName(attempt)
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO attempt_launch(attempt,unit,intended_at) VALUES(?,?,?)`, attempt, unit, at.Unix()); err != nil {
		return "", err
	}
	return unit, tx.Commit()
}

// launchUnit returns the attempt's intended unit, if an intent was recorded.
func launchUnit(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, attempt string) (string, bool, error) {
	var u string
	err := q.QueryRowContext(ctx, `SELECT unit FROM attempt_launch WHERE attempt=?`, attempt).Scan(&u)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return u, err == nil, err
}

// RecordStarted records STARTED after the unit has been launched. The launch intent must have been
// recorded first, and the unit must be the intended one.
func (s *Store) RecordStarted(ctx context.Context, attempt string, info StartInfo, at time.Time) error {
	return s.transition(ctx, attempt, KindStarted, at,
		func(sha string) error { return info.validate(sha) },
		func(r *Record) { r.Unit, r.InvocationID, r.CASSHA = info.Unit, info.InvocationID, info.CASSHA },
		func(ctx context.Context, tx *sql.Tx) error {
			u, ok, err := launchUnit(ctx, tx, attempt)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("%w: STARTED without a recorded launch intent", ErrBadTransition)
			}
			if u != info.Unit {
				return fmt.Errorf("%w: STARTED names unit %q, the intended unit is %q", ErrBadTransition, info.Unit, u)
			}
			return nil
		})
}

// RecordEnded records ENDED with the outcome of a unit that ran to its end (success, failure, timeout or
// a signal).
func (s *Store) RecordEnded(ctx context.Context, attempt string, info EndInfo, at time.Time) error {
	if err := info.validate(); err != nil {
		return err
	}
	return s.transition(ctx, attempt, KindEnded, at, nil, func(r *Record) { applyEnd(r, info) }, endReceipt(attempt, info, at))
}

func applyEnd(r *Record, info EndInfo) {
	st, ob, ms := info.ExitStatus, info.OutputBytes, info.Duration.Milliseconds()
	r.Result, r.ExitStatus, r.OutputSHA256, r.OutputBytes, r.DurationMS = info.Result, &st, info.OutputSHA256, &ob, &ms
	if !info.EndedAt.IsZero() {
		r.EndedAt = info.EndedAt.UTC().Format(time.RFC3339Nano)
	}
}

// RecordAborted records ABORTED_BEFORE_START for an attempt whose unit never launched. It is refused once
// a launch intent exists: whether the unit ran is then for RecoverUnits to find out.
func (s *Store) RecordAborted(ctx context.Context, attempt, reason string, at time.Time) error {
	return s.transition(ctx, attempt, KindAborted, at, nil, func(r *Record) { r.Note = reason },
		func(ctx context.Context, tx *sql.Tx) error {
			if _, ok, err := launchUnit(ctx, tx, attempt); err != nil {
				return err
			} else if ok {
				return fmt.Errorf("%w: %w", ErrBadTransition, ErrLaunchIntent)
			}
			return nil
		})
}

// Lookup reports how a launched unit ended, found by systemd InvocationID. found=false means systemd has no
// answer.
type Lookup func(ctx context.Context, invocationID string) (info EndInfo, found bool, err error)

// UnitState is what systemd says about an attempt's intended unit.
type UnitState struct {
	Found        bool     // systemd knows a unit of that name (running or ended)
	InvocationID string   // its InvocationID when Found
	Ended        *EndInfo // how it ended (with EndInfo.EndedAt, when known); nil while it is still running
}

// UnitLookup finds a unit by name. found=false with no error means systemd has no such unit.
type UnitLookup func(ctx context.Context, unit string) (UnitState, error)

// Recover is RecoverUnits without a unit lookup: an AUTHORIZED attempt that has a launch intent cannot be
// reconciled and is reported with ErrUnreconciled.
func (s *Store) Recover(ctx context.Context, lookup Lookup, now time.Time) ([]Record, error) {
	return s.RecoverUnits(ctx, lookup, nil, now)
}

// RecoverUnits settles attempts a crash left open. It must run at start-up, before any new attempt, and
// never retries anything. It only ever moves an attempt toward a known end; an unknown outcome keeps the
// card and the privilege's cap (UNCERTAIN holds both until an operator resolves it, ResolveUncertain).
//
//   - AUTHORIZED, no launch intent: the unit was never started, so ABORTED_BEFORE_START (no lookup).
//   - AUTHORIZED with a launch intent: byUnit is asked about the intended unit. Absent: UNCERTAIN, not
//     ABORTED_BEFORE_START, because the unit may have run and been collected, or been lost in a reboot; the
//     attempt keeps counting toward the interval and daily limits. Present: STARTED is recorded from its
//     InvocationID, then ENDED if it has ended; a unit still running stays STARTED and holds its card. No
//     answer: the attempt stays AUTHORIZED (ErrUnreconciled).
//     Residual: "absent" is only as good as byUnit; a launch call still in flight inside systemd cannot be
//     seen from here, which is why launching is a single call made after the intent is durable.
//   - STARTED: ENDED when byInv (by InvocationID) or else byUnit (by unit name, for the recorded
//     InvocationID) knows how the unit ended; still STARTED when byUnit reports that very invocation running;
//     otherwise UNCERTAIN ("outcome unknown, check by hand").
//
// An ENDED outcome carries the completion time the lookup reports. Recovery never substitutes its own
// clock: an unknown time leaves a check without a usable receipt, and a time that cannot be right (before
// the attempt, or after now) makes the outcome UNCERTAIN.
//
// A new run always needs a new envelope and nonce.
func (s *Store) RecoverUnits(ctx context.Context, byInv Lookup, byUnit UnitLookup, now time.Time) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT attempt, state, script_sha256 FROM attempts WHERE state IN('AUTHORIZED','STARTED') ORDER BY attempt`)
	if err != nil {
		return nil, err
	}
	type open struct{ id, state, sha string }
	var todo []open
	for rows.Next() {
		var o open
		if err := rows.Scan(&o.id, &o.state, &o.sha); err != nil {
			rows.Close()
			return nil, err
		}
		todo = append(todo, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	uncertain := func(id, note string) error {
		return s.transition(ctx, id, KindUncertain, now, nil, func(r *Record) { r.Note = note }, nil)
	}
	// ended records ENDED for a validated outcome, or UNCERTAIN when its completion time cannot be right.
	ended := func(id string, info EndInfo) error {
		err := s.transition(ctx, id, KindEnded, now, nil, func(r *Record) { applyEnd(r, info) }, endReceipt(id, info, now))
		if errors.Is(err, ErrEndTime) {
			return uncertain(id, "outcome unknown, check by hand: "+err.Error())
		}
		return err
	}
	var unreconciled []string
	for _, o := range todo {
		unit, intended, err := launchUnit(ctx, s.db, o.id)
		if err != nil {
			return nil, err
		}
		var st UnitState
		var lerr error = errors.New("no unit lookup")
		if byUnit != nil && intended {
			st, lerr = byUnit(ctx, unit)
		}
		if Kind(o.state) == KindAuthorized {
			if !intended {
				if err := s.RecordAborted(ctx, o.id, "verifier stopped before the unit was launched", now); err != nil {
					return nil, err
				}
				continue
			}
			switch {
			case lerr != nil || (st.Found && !reHex32.MatchString(st.InvocationID)):
				unreconciled = append(unreconciled, o.id)
			case !st.Found:
				err = uncertain(o.id, "launch intended; systemd has no such unit; it may have run, check by hand")
			default:
				err = s.RecordStarted(ctx, o.id, StartInfo{Unit: unit, InvocationID: st.InvocationID, CASSHA: o.sha}, now)
				if err == nil && st.Ended != nil {
					if st.Ended.validate() == nil {
						err = ended(o.id, *st.Ended)
					} else {
						err = uncertain(o.id, "outcome unknown, check by hand")
					}
				}
			}
			if err != nil {
				return nil, err
			}
			continue
		}
		// STARTED
		var inv string
		if err := s.db.QueryRowContext(ctx, `SELECT json_extract(body,'$.invocation_id') FROM audit WHERE attempt=? AND kind='STARTED'`, o.id).Scan(&inv); err != nil {
			return nil, err
		}
		var info EndInfo
		var found bool
		if byInv != nil {
			if info, found, err = byInv(ctx, inv); err != nil {
				found = false
			}
		}
		if found && info.validate() == nil {
			err = ended(o.id, info)
		} else if lerr == nil && st.Found && st.InvocationID == inv && st.Ended == nil {
			continue // the recorded invocation is still running: it keeps its card
		} else if lerr == nil && st.Found && st.InvocationID == inv && st.Ended.validate() == nil {
			err = ended(o.id, *st.Ended)
		} else {
			err = uncertain(o.id, "outcome unknown, check by hand")
		}
		if err != nil {
			return nil, err
		}
	}
	recs, err := s.Records(ctx)
	if err != nil {
		return nil, err
	}
	if len(unreconciled) > 0 {
		return recs, fmt.Errorf("%w: %s", ErrUnreconciled, strings.Join(unreconciled, ", "))
	}
	return recs, nil
}

// Resolution is an operator's ruling on an UNCERTAIN attempt.
type Resolution struct {
	Operator string // who looked: a short name, required, no control characters
	Outcome  string // ResolutionTerminated or ResolutionNeverRan
	Reason   string // what was checked, required
}

// Outcomes an operator may record. Both free the card and the privilege's cap; neither erases the attempt
// from the interval and daily limits, which count every attempt that may have executed.
const (
	ResolutionTerminated = "terminated" // the unit ran (or may have) and is certainly not running now
	ResolutionNeverRan   = "never_ran"  // the unit demonstrably never executed the script
)

func (r Resolution) validate() error {
	clean := func(name, v string, max int) error {
		if v == "" || len(v) > max {
			return fmt.Errorf("resolution needs %s (1-%d bytes)", name, max)
		}
		for _, c := range v {
			if c < 0x20 || c == 0x7f {
				return fmt.Errorf("resolution %s holds a control character", name)
			}
		}
		return nil
	}
	if err := clean("an operator", r.Operator, 64); err != nil {
		return err
	}
	if err := clean("a reason", r.Reason, 1024); err != nil {
		return err
	}
	if r.Outcome != ResolutionTerminated && r.Outcome != ResolutionNeverRan {
		return fmt.Errorf("resolution outcome %q is not %q or %q", r.Outcome, ResolutionTerminated, ResolutionNeverRan)
	}
	return nil
}

// ResolveUncertain records an operator's explicit decision that an UNCERTAIN attempt is over (RESOLVED),
// which frees its card and its privilege's cap. It is the only way out of UNCERTAIN: recovery never does
// it. The record names the operator, the outcome and the reason; the attempt must be UNCERTAIN.
func (s *Store) ResolveUncertain(ctx context.Context, attempt string, res Resolution, at time.Time) error {
	if err := res.validate(); err != nil {
		return err
	}
	return s.transition(ctx, attempt, KindResolved, at, nil, func(r *Record) {
		r.Operator, r.Resolution, r.Note = res.Operator, res.Outcome, res.Reason
	}, nil)
}

// Records returns the whole audit table in sequence order.
func (s *Store) Records(ctx context.Context) ([]Record, error) { return s.recordsAfter(ctx, 0) }

func (s *Store) recordsAfter(ctx context.Context, after int64) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT seq, body FROM audit WHERE seq>? ORDER BY seq`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var seq int64
		var body string
		if err := rows.Scan(&seq, &body); err != nil {
			return nil, err
		}
		var r Record
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			return nil, err
		}
		r.Seq = seq
		out = append(out, r)
	}
	return out, rows.Err()
}

// CheckAuditChain verifies the whole audit table: every attempt's records follow a legal sequence from
// AUTHORIZED, the attempt's state is its last record's kind, each record carries the attempt's identity,
// and a nonce is spent iff an attempt exists for it. With requireSettled it also refuses an attempt that is
// still AUTHORIZED or STARTED (the state after Recover has run). UNCERTAIN counts as settled here: it is a
// recorded state, though it still holds its card until resolved.
func (s *Store) CheckAuditChain(ctx context.Context, requireSettled bool) error {
	recs, err := s.Records(ctx)
	if err != nil {
		return err
	}
	by := map[string][]Record{}
	for _, r := range recs {
		by[r.Attempt] = append(by[r.Attempt], r)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT attempt, state, nonce, identity FROM attempts`)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, state, nonce, ident string
		if err := rows.Scan(&id, &state, &nonce, &ident); err != nil {
			return err
		}
		n++
		rs := by[id]
		if len(rs) == 0 || rs[0].Kind != KindAuthorized {
			return fmt.Errorf("attempt %s: audit does not start with AUTHORIZED", id)
		}
		for i := 1; i < len(rs); i++ {
			ok := false
			for _, p := range legalPrev[rs[i].Kind] {
				ok = ok || rs[i-1].Kind == p
			}
			if !ok {
				return fmt.Errorf("attempt %s: %s after %s", id, rs[i].Kind, rs[i-1].Kind)
			}
		}
		for _, r := range rs {
			if r.Kind != KindStarted {
				continue
			}
			if u, ok, err := launchUnit(ctx, s.db, id); err != nil || !ok || u != r.Unit {
				return fmt.Errorf("attempt %s: STARTED without a matching launch intent", id)
			}
		}
		if last := rs[len(rs)-1].Kind; string(last) != state {
			return fmt.Errorf("attempt %s: state %s but last audit record is %s", id, state, last)
		}
		if requireSettled && isActive(Kind(state)) {
			return fmt.Errorf("attempt %s: still %s", id, state)
		}
		for _, r := range rs {
			if r.Nonce != nonce || r.EnvelopeDigest == "" || r.Signer.Fingerprint == "" || r.At == "" {
				return fmt.Errorf("attempt %s: record %d lacks the attempt's identity", id, r.Seq)
			}
		}
		delete(by, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(by) != 0 {
		return errors.New("audit records for an unknown attempt")
	}
	return nil
}
