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
	// ErrBusy: another attempt is active on the card, or the global cap for the privilege is reached.
	// The request is not queued and the nonce is not consumed.
	ErrBusy = errors.New("busy")
	// ErrBadTransition: an audit step that the attempt's sequence does not allow.
	ErrBadTransition = errors.New("illegal audit transition")
)

// Store is the verifier's durable state: single-use nonces, attempts and the audit table, in one SQLite
// database (WAL, synchronous=FULL). The database file belongs to the verifier alone.
type Store struct {
	db  *sql.DB
	lim Limits
	mir *mirror
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
  state          TEXT NOT NULL CHECK(state IN('AUTHORIZED','STARTED','ENDED','UNCERTAIN','ABORTED_BEFORE_START')),
  created_at     INTEGER NOT NULL
) WITHOUT ROWID;
CREATE UNIQUE INDEX IF NOT EXISTS attempts_one_active_per_card ON attempts(card_id)
  WHERE state IN('AUTHORIZED','STARTED');
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
	if _, err := db.Exec(schema + schemaLimits); err != nil {
		db.Close()
		return nil, fmt.Errorf("homerun: schema: %w", err)
	}
	return &Store{db: db, lim: lim}, nil
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
	if err := s.requireUnlocked(ctx); err != nil {
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

	var onCard int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM attempts WHERE card_id=? AND state IN('AUTHORIZED','STARTED')`, e.CardID).Scan(&onCard); err != nil {
		return nil, err
	}
	var onPriv int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM attempts WHERE privilege=? AND state IN('AUTHORIZED','STARTED')`, e.Privilege).Scan(&onPriv); err != nil {
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

// RecordStarted records STARTED after the unit has been launched.
func (s *Store) RecordStarted(ctx context.Context, attempt string, info StartInfo, at time.Time) error {
	return s.transition(ctx, attempt, KindStarted, at,
		func(sha string) error { return info.validate(sha) },
		func(r *Record) { r.Unit, r.InvocationID, r.CASSHA = info.Unit, info.InvocationID, info.CASSHA }, nil)
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
}

// RecordAborted records ABORTED_BEFORE_START for an attempt whose unit never launched.
func (s *Store) RecordAborted(ctx context.Context, attempt, reason string, at time.Time) error {
	return s.transition(ctx, attempt, KindAborted, at, nil, func(r *Record) { r.Note = reason }, nil)
}

// Lookup reports how a launched unit ended, found by systemd InvocationID. found=false means systemd has no
// answer.
type Lookup func(ctx context.Context, invocationID string) (info EndInfo, found bool, err error)

// Recover settles attempts a crash left open. It must run at start-up, before any new attempt, and never
// retries anything: an AUTHORIZED attempt with no STARTED becomes ABORTED_BEFORE_START; a STARTED attempt
// becomes ENDED when lookup knows how the unit ended, else UNCERTAIN ("outcome unknown, check by hand").
// A new run always needs a new envelope and nonce.
func (s *Store) Recover(ctx context.Context, lookup Lookup, now time.Time) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT attempt, state FROM attempts WHERE state IN('AUTHORIZED','STARTED') ORDER BY attempt`)
	if err != nil {
		return nil, err
	}
	type open struct{ id, state string }
	var todo []open
	for rows.Next() {
		var o open
		if err := rows.Scan(&o.id, &o.state); err != nil {
			rows.Close()
			return nil, err
		}
		todo = append(todo, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, o := range todo {
		if Kind(o.state) == KindAuthorized {
			if err := s.RecordAborted(ctx, o.id, "verifier stopped before the unit was launched", now); err != nil {
				return nil, err
			}
			continue
		}
		var inv string
		if err := s.db.QueryRowContext(ctx, `SELECT json_extract(body,'$.invocation_id') FROM audit WHERE attempt=? AND kind='STARTED'`, o.id).Scan(&inv); err != nil {
			return nil, err
		}
		var info EndInfo
		var found bool
		if lookup != nil {
			if info, found, err = lookup(ctx, inv); err != nil {
				found = false
			}
		}
		if found && info.validate() == nil {
			err = s.transition(ctx, o.id, KindEnded, now, nil, func(r *Record) { applyEnd(r, info) }, endReceipt(o.id, info, now))
		} else {
			err = s.transition(ctx, o.id, KindUncertain, now, nil, func(r *Record) { r.Note = "outcome unknown, check by hand" }, nil)
		}
		if err != nil {
			return nil, err
		}
	}
	return s.Records(ctx)
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
// still active (the state after Recover has run).
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
