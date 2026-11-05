//go:build unix

package homerun

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// The review gate (design 4.2, "Review receipt provenance"). Before a nonce is spent, the verifier must
// fetch the review receipt whose sha256 equals the envelope's review_sha256, verify it under the
// configured review public key, and require PASS, the envelope's script digest, deps digest and flags,
// and an unexpired receipt. There is no fallback and no switch that turns the gate off: a Store with no
// gate refuses every Consume.
//
// Ordering. The gate runs after the host, freshness, range and lock checks and before the nonce
// transaction, so a review refusal never spends the nonce, the same as the busy, check-receipt and rate
// limit refusals. A refusal is recorded in the review_refusals table (append-only, with a reason code);
// it has no attempt, so it is not part of the per-attempt audit sequence or its mirror.

var (
	// ErrReview wraps every review-gate refusal. Show a requester only that the review was refused.
	ErrReview = errors.New("review evidence refused")
	// ErrNoReviewGate: the store has no review gate, so nothing can be authorised.
	ErrNoReviewGate = errors.New("homerun: no review gate configured")
)

// Reasons recorded for a review refusal.
const (
	ReviewMissing  = "receipt-missing" // nothing in the receipt store under review_sha256
	ReviewRefused  = "receipt-refused" // present but failed verification (parse, signature, outcome, binding, expiry)
	ReviewNoGate   = "no-review-gate"
	ReviewMismatch = "receipt-mismatch" // verified, but its digest is not the envelope's review_sha256
)

// ReceiptSource returns the canonical receipt bytes stored under a sha256. *CAS is one.
type ReceiptSource interface {
	Read(sha string) ([]byte, error)
}

// ReviewVerifyFunc verifies raw receipt bytes against what the operation requires and returns the
// receipt digest and the receipt's expiry (unix seconds). review.Verifier supplies one; this package
// cannot import it.
type ReviewVerifyFunc func(raw []byte, scriptSHA, depsDigest string, flags []string, now time.Time) (digest string, expiresAt int64, err error)

// ReviewGate is the configured review evidence check.
type ReviewGate struct {
	Receipts ReceiptSource
	Verify   ReviewVerifyFunc
}

// ReviewError carries the recorded reason; it matches ErrReview.
type ReviewError struct {
	Reason string
	Detail string
}

func (e *ReviewError) Error() string { return "review evidence refused: " + e.Reason }
func (e *ReviewError) Unwrap() error { return ErrReview }

// Is makes the no-gate refusal match ErrNoReviewGate as well as ErrReview.
func (e *ReviewError) Is(target error) bool {
	return target == ErrNoReviewGate && e.Reason == ReviewNoGate
}

func (g *ReviewGate) check(e *Envelope, now time.Time) (int64, *ReviewError) {
	if g == nil || g.Receipts == nil || g.Verify == nil {
		return 0, &ReviewError{Reason: ReviewRefused, Detail: "review gate is not fully configured"}
	}
	raw, err := g.Receipts.Read(e.ReviewSHA256)
	if err != nil {
		return 0, &ReviewError{Reason: ReviewMissing, Detail: err.Error()}
	}
	dg, exp, err := g.Verify(raw, e.ScriptSHA256, e.DepsDigest, e.Flags, now)
	if err != nil {
		return 0, &ReviewError{Reason: ReviewRefused, Detail: err.Error()}
	}
	if dg != e.ReviewSHA256 {
		return 0, &ReviewError{Reason: ReviewMismatch, Detail: "receipt digest differs from review_sha256"}
	}
	return exp, nil
}

// One row per (envelope digest, reason): a replay of the same refused envelope increments count and moves
// last_at instead of adding a row, so the table grows with distinct signed envelopes, not with attempts.
// Rows are never deleted, and an update may only count one more refusal of the same row.
const reviewSchema = `
CREATE TABLE IF NOT EXISTS review_refusals(
  seq             INTEGER PRIMARY KEY AUTOINCREMENT,
  first_at        INTEGER NOT NULL,
  last_at         INTEGER NOT NULL,
  count           INTEGER NOT NULL,
  reason          TEXT NOT NULL,
  detail          TEXT NOT NULL,
  envelope_digest TEXT NOT NULL,
  card_id         TEXT NOT NULL,
  op              TEXT NOT NULL,
  nonce           TEXT NOT NULL,
  review_sha256   TEXT NOT NULL,
  UNIQUE(envelope_digest, reason)
);
CREATE TRIGGER IF NOT EXISTS review_refusals_count_only BEFORE UPDATE ON review_refusals
WHEN NEW.seq<>OLD.seq OR NEW.first_at<>OLD.first_at OR NEW.reason<>OLD.reason OR NEW.envelope_digest<>OLD.envelope_digest
  OR NEW.card_id<>OLD.card_id OR NEW.op<>OLD.op OR NEW.nonce<>OLD.nonce OR NEW.review_sha256<>OLD.review_sha256
  OR NEW.count<>OLD.count+1 OR NEW.last_at<OLD.last_at
BEGIN SELECT RAISE(ABORT,'refusals are append-only'); END;
CREATE TRIGGER IF NOT EXISTS review_refusals_no_delete BEFORE DELETE ON review_refusals BEGIN SELECT RAISE(ABORT,'refusals are append-only'); END;
`

// ReviewRefusal is one recorded review refusal.
type ReviewRefusal struct {
	Seq             int64
	At              time.Time
	Reason          string
	Detail          string
	EnvelopeDigest  string
	CardID          string
	Op              string
	Nonce           string
	ReviewSHA256    string
	Count           int64
	FirstAt, LastAt time.Time
}

// SetClock sets the clock the store trusts for the recheck made after the write lock is taken (the default
// is the system clock). Tests control it; production leaves it alone.
func (s *Store) SetClock(now func() time.Time) { s.now = now }

// SetReviewGate installs the gate. Consume refuses until one is installed.
func (s *Store) SetReviewGate(g *ReviewGate) { s.gate = g }

// maxRefusalPage bounds one page of ReviewRefusals.
const maxRefusalPage = 1000

// ReviewRefusals returns up to limit refusals with Seq greater than after, in order (limit is clamped to
// 1..1000, default 100 when not positive). To page, pass the last Seq returned as after.
func (s *Store) ReviewRefusals(ctx context.Context, after int64, limit int) ([]ReviewRefusal, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > maxRefusalPage {
		limit = maxRefusalPage
	}
	rows, err := s.db.QueryContext(ctx, `SELECT seq,first_at,last_at,count,reason,detail,envelope_digest,card_id,op,nonce,review_sha256 FROM review_refusals WHERE seq>? ORDER BY seq LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReviewRefusal
	for rows.Next() {
		var r ReviewRefusal
		var first, last int64
		if err := rows.Scan(&r.Seq, &first, &last, &r.Count, &r.Reason, &r.Detail, &r.EnvelopeDigest, &r.CardID, &r.Op, &r.Nonce, &r.ReviewSHA256); err != nil {
			return nil, err
		}
		r.FirstAt, r.LastAt = time.Unix(first, 0).UTC(), time.Unix(last, 0).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// recordRefusal counts a refusal against its (envelope, reason) row.
func (s *Store) recordRefusal(ctx context.Context, req ConsumeRequest, re *ReviewError) error {
	detail := re.Detail
	if len(detail) > 256 {
		detail = detail[:256]
	}
	at := req.Now.Unix()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO review_refusals(first_at,last_at,count,reason,detail,envelope_digest,card_id,op,nonce,review_sha256) VALUES(?,?,1,?,?,?,?,?,?,?)
		 ON CONFLICT(envelope_digest,reason) DO UPDATE SET count=count+1, last_at=MAX(last_at,excluded.last_at), detail=excluded.detail`,
		at, at, re.Reason, detail, req.EnvelopeDigest, req.Env.CardID, req.Env.Op, req.Env.Nonce, req.Env.ReviewSHA256)
	if err != nil {
		return fmt.Errorf("%w (and the refusal could not be recorded: %v)", re, err)
	}
	return re
}

// gateReview runs the gate for one consume request, records a refusal, and returns the verified receipt's
// expiry so Consume can recheck it once it holds the write lock.
func (s *Store) gateReview(ctx context.Context, req ConsumeRequest) (int64, error) {
	if s.gate == nil {
		return 0, s.recordRefusal(ctx, req, &ReviewError{Reason: ReviewNoGate, Detail: ErrNoReviewGate.Error()})
	}
	exp, re := s.gate.check(req.Env, req.Now)
	if re != nil {
		return 0, s.recordRefusal(ctx, req, re)
	}
	return exp, nil
}
