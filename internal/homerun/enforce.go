//go:build unix

package homerun

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Enforcement of the verifier's limits (design 4.5, 4.10 and D8). Every function takes the time as a
// parameter, never reading the clock, so tests drive it with a fake one; the verifier passes time.Now().

var (
	// ErrRateLimited: a per-card or per-day limit is reached. The request is not queued and the nonce is
	// left unspent. The concrete error is a *RateLimitError.
	ErrRateLimited = errors.New("rate limited")
	// ErrCheckReceipt: an apply or recover cites a check that has no usable verifier-recorded receipt.
	// The concrete error says which test failed; the nonce is left unspent.
	ErrCheckReceipt = errors.New("no usable check receipt")
	// ErrLocked: too many bad signatures; the verifier refuses everything until Unlock runs from the
	// root shell.
	ErrLocked = errors.New("verifier locked after repeated bad signatures")
)

// RateLimitError names the limit that was hit and when the request could next succeed.
type RateLimitError struct {
	Limit      string // "apply-interval" or "daily"
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited (%s): retry in %s", e.Limit, e.RetryAfter.Round(time.Second))
}
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

const schemaLimits = `
CREATE TABLE IF NOT EXISTS attempt_bind(
  attempt     TEXT PRIMARY KEY REFERENCES attempts(attempt),
  card_rev    TEXT NOT NULL,
  deps_digest TEXT NOT NULL
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS check_receipts(
  attempt       TEXT PRIMARY KEY REFERENCES attempts(attempt),
  card_id       TEXT NOT NULL,
  card_rev      TEXT NOT NULL,
  script_sha256 TEXT NOT NULL,
  deps_digest   TEXT NOT NULL,
  privilege     TEXT NOT NULL,
  output_sha256 TEXT NOT NULL,
  ended_at      INTEGER NOT NULL
) WITHOUT ROWID;
CREATE TRIGGER IF NOT EXISTS receipts_no_update BEFORE UPDATE ON check_receipts BEGIN SELECT RAISE(ABORT,'receipts are immutable'); END;
CREATE TRIGGER IF NOT EXISTS receipts_no_delete BEFORE DELETE ON check_receipts BEGIN SELECT RAISE(ABORT,'receipts are kept'); END;
CREATE TABLE IF NOT EXISTS sig_failures(
  id     INTEGER PRIMARY KEY AUTOINCREMENT,
  at     INTEGER NOT NULL,
  reason TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS lockouts(
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  locked_at   INTEGER NOT NULL,
  unlocked_at INTEGER
);
CREATE UNIQUE INDEX IF NOT EXISTS lockouts_one_active ON lockouts((1)) WHERE unlocked_at IS NULL;
`

// ---- rate limits ---------------------------------------------------------------------------------

// enforceRateLimits applies the per-card apply interval and the rolling daily cap. It runs inside the
// Consume transaction, before the new attempt is inserted. Attempts that ended ABORTED_BEFORE_START never
// executed and are not counted; every other attempt is, whatever its outcome.
//
// The interval covers op "apply" only: a recover on the same card is an execution (so it counts toward
// the daily cap) but is not held back by a recent apply, since recovering from a failed apply is the
// case it exists for. A check is likewise subject to the daily cap alone.
func enforceRateLimits(ctx context.Context, tx *sql.Tx, lim Limits, e *Envelope, now time.Time) error {
	t := now.Unix()
	if e.Op == OpApply {
		var last sql.NullInt64
		if err := tx.QueryRowContext(ctx,
			`SELECT max(created_at) FROM attempts WHERE card_id=? AND op=? AND state<>? AND created_at>?`,
			e.CardID, OpApply, string(KindAborted), t-int64(lim.ApplyInterval/time.Second)).Scan(&last); err != nil {
			return err
		}
		if last.Valid {
			return &RateLimitError{Limit: "apply-interval",
				RetryAfter: time.Duration(last.Int64+int64(lim.ApplyInterval/time.Second)-t) * time.Second}
		}
	}
	const day = int64(24 * time.Hour / time.Second)
	rows, err := tx.QueryContext(ctx,
		`SELECT created_at FROM attempts WHERE state<>? AND created_at>? ORDER BY created_at`,
		string(KindAborted), t-day)
	if err != nil {
		return err
	}
	var ats []int64
	for rows.Next() {
		var a int64
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			return err
		}
		ats = append(ats, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if n := len(ats) - lim.MaxExecutionsPerDay; n >= 0 {
		// the request can succeed once enough of the oldest executions have left the window
		return &RateLimitError{Limit: "daily", RetryAfter: time.Duration(ats[n]+day-t) * time.Second}
	}
	return nil
}

// ---- check receipts ------------------------------------------------------------------------------

// endReceipt returns the in-transaction step that records a check receipt when a check attempt ends with
// result success and exit status 0. Any other attempt, or any other outcome, records nothing. The receipt
// is taken from what the verifier itself stored when it consumed the nonce (card, card revision, script
// and deps digest), never from the caller; the output digest comes from the unit's end report.
func endReceipt(attempt string, info EndInfo, at time.Time) func(context.Context, *sql.Tx) error {
	return func(ctx context.Context, tx *sql.Tx) error {
		if info.Result != ResultSuccess || info.ExitStatus != 0 {
			return nil
		}
		var op, card, sha, priv, rev, deps string
		err := tx.QueryRowContext(ctx,
			`SELECT a.op, a.card_id, a.script_sha256, a.privilege, b.card_rev, b.deps_digest
			   FROM attempts a JOIN attempt_bind b ON b.attempt=a.attempt WHERE a.attempt=?`, attempt).
			Scan(&op, &card, &sha, &priv, &rev, &deps)
		if err != nil {
			return err
		}
		if op != OpCheck {
			return nil
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO check_receipts(attempt,card_id,card_rev,script_sha256,deps_digest,privilege,output_sha256,ended_at) VALUES(?,?,?,?,?,?,?,?)`,
			attempt, card, rev, sha, deps, priv, info.OutputSHA256, at.Unix())
		return err
	}
}

func receiptErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrCheckReceipt, fmt.Sprintf(format, a...))
}

// checkCitedReceipt is the T2.11 gate: an apply or recover is allowed only when the check attempt it
// cites has a receipt the verifier recorded, bound to the same card, card revision, script and deps
// digest, carrying the output digest the envelope cites, and at most CheckFreshness old (counted from the
// moment the check ended). A failed or still-running check never has a receipt.
func checkCitedReceipt(ctx context.Context, tx *sql.Tx, lim Limits, e *Envelope, now time.Time) error {
	if e.CheckAttempt == nil || e.CheckOutputSHA == nil {
		return receiptErr("envelope cites no check")
	}
	var card, rev, sha, deps, priv, out string
	var ended int64
	err := tx.QueryRowContext(ctx,
		`SELECT card_id, card_rev, script_sha256, deps_digest, privilege, output_sha256, ended_at FROM check_receipts WHERE attempt=?`,
		*e.CheckAttempt).Scan(&card, &rev, &sha, &deps, &priv, &out, &ended)
	if errors.Is(err, sql.ErrNoRows) {
		return receiptErr("no verifier-recorded successful check %s", *e.CheckAttempt)
	}
	if err != nil {
		return err
	}
	switch {
	case card != e.CardID:
		return receiptErr("check was for another card")
	case rev != e.CardRev:
		return receiptErr("check was for another card revision")
	case sha != e.ScriptSHA256:
		return receiptErr("check was for other script bytes")
	case deps != e.DepsDigest:
		return receiptErr("check was for other dependencies")
	case priv != e.Privilege:
		return receiptErr("check ran under another privilege")
	case out != *e.CheckOutputSHA:
		return receiptErr("check output digest differs from the one cited")
	}
	age := now.Unix() - ended
	if age < 0 {
		return receiptErr("check ended in the future")
	}
	if age > int64(lim.CheckFreshness/time.Second) {
		return receiptErr("check is %s old, more than %s", (time.Duration(age) * time.Second).Round(time.Second), lim.CheckFreshness)
	}
	return nil
}

// ---- signature-failure lockout -------------------------------------------------------------------

// Locked reports whether the verifier is locked.
func (s *Store) Locked(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM lockouts WHERE unlocked_at IS NULL`).Scan(&n)
	return n > 0, err
}

func (s *Store) requireUnlocked(ctx context.Context) error {
	locked, err := s.Locked(ctx)
	if err != nil {
		return err
	}
	if locked {
		return ErrLocked
	}
	return nil
}

// RecordSigFailure records one failed signature verification at now. When the failures since the last
// unlock reach SigFailureThreshold within SigFailureWindow it locks the verifier and returns true. The
// lock survives restarts and ends only with Unlock.
func (s *Store) RecordSigFailure(ctx context.Context, reason string, now time.Time) (locked bool, err error) {
	if len(reason) > 200 {
		reason = reason[:200]
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck
	t := now.Unix()
	if _, err := tx.ExecContext(ctx, `INSERT INTO sig_failures(at,reason) VALUES(?,?)`, t, reason); err != nil {
		return false, err
	}
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM lockouts WHERE unlocked_at IS NULL`).Scan(&active); err != nil {
		return false, err
	}
	if active == 0 {
		var since sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT max(unlocked_at) FROM lockouts`).Scan(&since); err != nil {
			return false, err
		}
		floor := t - int64(s.lim.SigFailureWindow/time.Second)
		if since.Valid && since.Int64 > floor {
			floor = since.Int64 // failures from before the last unlock do not count again
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM sig_failures WHERE at>? AND at<=?`, floor, t).Scan(&n); err != nil {
			return false, err
		}
		if n >= s.lim.SigFailureThreshold {
			if _, err := tx.ExecContext(ctx, `INSERT INTO lockouts(locked_at) VALUES(?)`, t); err != nil {
				return false, err
			}
			active = 1
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return active > 0, nil
}

// Unlock clears a lockout. It is meant to be called only from the root shell's maintenance command; the
// request socket must not expose it.
func (s *Store) Unlock(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE lockouts SET unlocked_at=? WHERE unlocked_at IS NULL`, now.Unix())
	return err
}

// VerifyApproval is the package-level VerifyApproval with the lockout wired in: it refuses at once while
// locked, and counts a refused signature (ErrSignature only; a malformed or expired envelope is not a bad
// signature) toward the lockout. Use this, not the bare function, in the verifier's request path.
func (s *Store) VerifyApproval(ctx context.Context, v *SigVerifier, machineID string, envBytes, sig []byte, now time.Time) (*Envelope, string, *Signer, error) {
	if err := s.requireUnlocked(ctx); err != nil {
		return nil, "", nil, err
	}
	env, dg, signer, err := VerifyApproval(ctx, v, s.lim, machineID, envBytes, sig, now)
	if err != nil && errors.Is(err, ErrSignature) {
		if locked, ferr := s.RecordSigFailure(ctx, err.Error(), now); ferr != nil {
			return nil, "", nil, fmt.Errorf("%w (and recording the failure failed: %v)", err, ferr)
		} else if locked {
			return nil, "", nil, fmt.Errorf("%w; %w", err, ErrLocked)
		}
	}
	return env, dg, signer, err
}

// RuntimeCap is the policy cap on runtime_max_s for op, as a duration. The launcher reads the signed value
// from Attempt.RuntimeMax; this is the ceiling it was validated against.
func (l Limits) RuntimeCap(op string) time.Duration {
	return time.Duration(l.runtimeCap(op)) * time.Second
}
