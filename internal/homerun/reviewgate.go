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
	ReviewMissing  = "receipt-missing"  // nothing in the receipt store under review_sha256
	ReviewRefused  = "receipt-refused"  // present but failed verification (parse, signature, outcome, binding, expiry)
	ReviewMismatch = "receipt-mismatch" // verified, but its digest is not the envelope's review_sha256
)

// ReceiptSource returns the canonical receipt bytes stored under a sha256. *CAS is one.
type ReceiptSource interface {
	Read(sha string) ([]byte, error)
}

// ReviewVerifyFunc verifies raw receipt bytes against what the operation requires and returns the
// receipt digest. review.Verifier supplies one; this package cannot import it.
type ReviewVerifyFunc func(raw []byte, scriptSHA, depsDigest string, flags []string, now time.Time) (string, error)

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

func (g *ReviewGate) check(e *Envelope, now time.Time) *ReviewError {
	if g == nil || g.Receipts == nil || g.Verify == nil {
		return &ReviewError{Reason: ReviewRefused, Detail: "review gate is not fully configured"}
	}
	raw, err := g.Receipts.Read(e.ReviewSHA256)
	if err != nil {
		return &ReviewError{Reason: ReviewMissing, Detail: err.Error()}
	}
	dg, err := g.Verify(raw, e.ScriptSHA256, e.DepsDigest, e.Flags, now)
	if err != nil {
		return &ReviewError{Reason: ReviewRefused, Detail: err.Error()}
	}
	if dg != e.ReviewSHA256 {
		return &ReviewError{Reason: ReviewMismatch, Detail: "receipt digest differs from review_sha256"}
	}
	return nil
}

const reviewSchema = `
CREATE TABLE IF NOT EXISTS review_refusals(
  seq             INTEGER PRIMARY KEY AUTOINCREMENT,
  at              INTEGER NOT NULL,
  reason          TEXT NOT NULL,
  detail          TEXT NOT NULL,
  envelope_digest TEXT NOT NULL,
  card_id         TEXT NOT NULL,
  op              TEXT NOT NULL,
  nonce           TEXT NOT NULL,
  review_sha256   TEXT NOT NULL
);
CREATE TRIGGER IF NOT EXISTS review_refusals_no_update BEFORE UPDATE ON review_refusals BEGIN SELECT RAISE(ABORT,'refusals are append-only'); END;
CREATE TRIGGER IF NOT EXISTS review_refusals_no_delete BEFORE DELETE ON review_refusals BEGIN SELECT RAISE(ABORT,'refusals are append-only'); END;
`

// ReviewRefusal is one recorded review refusal.
type ReviewRefusal struct {
	Seq            int64
	At             time.Time
	Reason         string
	Detail         string
	EnvelopeDigest string
	CardID         string
	Op             string
	Nonce          string
	ReviewSHA256   string
}

// SetReviewGate installs the gate. Consume refuses until one is installed.
func (s *Store) SetReviewGate(g *ReviewGate) { s.gate = g }

// ReviewRefusals lists recorded review refusals in order.
func (s *Store) ReviewRefusals(ctx context.Context) ([]ReviewRefusal, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT seq,at,reason,detail,envelope_digest,card_id,op,nonce,review_sha256 FROM review_refusals ORDER BY seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReviewRefusal
	for rows.Next() {
		var r ReviewRefusal
		var at int64
		if err := rows.Scan(&r.Seq, &at, &r.Reason, &r.Detail, &r.EnvelopeDigest, &r.CardID, &r.Op, &r.Nonce, &r.ReviewSHA256); err != nil {
			return nil, err
		}
		r.At = time.Unix(at, 0).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// gateReview runs the gate for one consume request and records a refusal.
func (s *Store) gateReview(ctx context.Context, req ConsumeRequest) error {
	if s.gate == nil {
		return ErrNoReviewGate
	}
	re := s.gate.check(req.Env, req.Now)
	if re == nil {
		return nil
	}
	detail := re.Detail
	if len(detail) > 256 {
		detail = detail[:256]
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO review_refusals(at,reason,detail,envelope_digest,card_id,op,nonce,review_sha256) VALUES(?,?,?,?,?,?,?,?)`,
		req.Now.Unix(), re.Reason, detail, req.EnvelopeDigest, req.Env.CardID, req.Env.Op, req.Env.Nonce, req.Env.ReviewSHA256)
	if err != nil {
		return fmt.Errorf("%w (and the refusal could not be recorded: %v)", re, err)
	}
	return re
}
