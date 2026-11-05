//go:build unix

package homerun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// T2.* limit, lockout, freshness and receipt cases (design 4.5, 4.10, D8). Every case passes its own
// clock; nothing here sleeps.

var bg = context.Background()

const outHex = "4444444444444444444444444444444444444444444444444444444444444444"

// submit issues a nonce and consumes an envelope for card/op at now. For apply and recover, cite names the
// check attempt to cite (empty cites a made-up one). mutate may alter the envelope before it is consumed.
func submit(t *testing.T, s *Store, now time.Time, card, op, cite string, mutate func(*Envelope)) (*Attempt, *Envelope, string, error) {
	t.Helper()
	n, err := s.IssueNonce(bg, testHost, 4*time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	e := testEnvelope(t, now)
	e.CardID, e.Nonce = card, n.Value
	if op != OpCheck {
		e.Op = op
		if cite == "" {
			cite = testAttemp
		}
		o := outHex
		e.CheckAttempt, e.CheckOutputSHA = &cite, &o
	}
	if mutate != nil {
		mutate(e)
	}
	if err := e.Validate(DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	dg, _ := e.Digest()
	a, err := s.Consume(bg, ConsumeRequest{Env: e, EnvelopeDigest: dg,
		Signer: Signer{Principal: principal, KeyType: "ED25519-CERT", Fingerprint: "SHA256:test"}, Host: testHost, Now: now})
	return a, e, n.Value, err
}

func finish(t *testing.T, s *Store, a *Attempt, at time.Time, info EndInfo) {
	t.Helper()
	if err := recStarted(s, bg, a.ID, StartInfo{Unit: "run-test.service", InvocationID: strings.Repeat("e", 32), CASSHA: a.ScriptSHA256}, at); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnded(bg, a.ID, info, at); err != nil {
		t.Fatal(err)
	}
}

func okEnd() EndInfo {
	return EndInfo{Result: ResultSuccess, ExitStatus: 0, OutputSHA256: outHex, OutputBytes: 7, Duration: time.Second}
}

// goodCheck runs a successful check for card at now and returns its attempt ID.
func goodCheck(t *testing.T, s *Store, now time.Time, card string) string {
	t.Helper()
	a, _, _, err := submit(t, s, now, card, OpCheck, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	finish(t, s, a, now, okEnd())
	return a.ID
}

func nonceSpent(t *testing.T, s *Store, nonce string) bool {
	t.Helper()
	var c *int64
	if err := s.db.QueryRow(`SELECT consumed_at FROM nonces WHERE nonce=?`, nonce).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c != nil
}

func TestApplyIntervalPerCard(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	chk := goodCheck(t, s, t0, "AUT-1")
	a, _, _, err := submit(t, s, t0, "AUT-1", OpApply, chk, nil)
	if err != nil {
		t.Fatal(err)
	}
	finish(t, s, a, t0, okEnd())

	// a second apply on the same card inside 10 minutes, with a fresh check: refused, nonce unspent
	t1 := t0.Add(9*time.Minute + 59*time.Second)
	chk2 := goodCheck(t, s, t1, "AUT-1")
	_, _, nonce, err := submit(t, s, t1, "AUT-1", OpApply, chk2, nil)
	var rl *RateLimitError
	if !errors.Is(err, ErrRateLimited) || !errors.As(err, &rl) || rl.Limit != "apply-interval" || rl.RetryAfter != time.Second {
		t.Fatalf("want apply-interval rate limit with 1s retry, got %v", err)
	}
	if nonceSpent(t, s, nonce) {
		t.Fatal("a rate-limited request must leave its nonce unspent")
	}
	// another card is not held back
	chkB := goodCheck(t, s, t1, "AUT-2")
	if _, _, _, err := submit(t, s, t1, "AUT-2", OpApply, chkB, nil); err != nil {
		t.Fatalf("other card: %v", err)
	}
	// a check on the same card is not held back by the apply interval
	if _, _, _, err := submit(t, s, t1, "AUT-1", OpCheck, "", nil); err != nil {
		t.Fatalf("check on the same card: %v", err)
	}
}

func TestApplyAllowedAfterInterval(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	chk := goodCheck(t, s, t0, "AUT-1")
	a, _, _, _ := submit(t, s, t0, "AUT-1", OpApply, chk, nil)
	finish(t, s, a, t0, okEnd())
	t1 := t0.Add(10 * time.Minute)
	chk2 := goodCheck(t, s, t1, "AUT-1")
	if _, _, _, err := submit(t, s, t1, "AUT-1", OpApply, chk2, nil); err != nil {
		t.Fatalf("apply exactly one interval later: %v", err)
	}
}

func TestAbortedAttemptDoesNotCount(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	chk := goodCheck(t, s, t0, "AUT-1")
	a, _, _, _ := submit(t, s, t0, "AUT-1", OpApply, chk, nil)
	if err := s.RecordAborted(bg, a.ID, "never launched", t0); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := submit(t, s, t0.Add(time.Minute), "AUT-1", OpApply, chk, nil); err != nil {
		t.Fatalf("an apply that never started must not block the next: %v", err)
	}
}

func TestRecoverNotHeldByApplyInterval(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	chk := goodCheck(t, s, t0, "AUT-1")
	a, _, _, _ := submit(t, s, t0, "AUT-1", OpApply, chk, nil)
	finish(t, s, a, t0, EndInfo{Result: ResultFailed, ExitStatus: 1, OutputSHA256: outHex, OutputBytes: 1, Duration: time.Second})
	chk2 := goodCheck(t, s, t0.Add(time.Minute), "AUT-1")
	if _, _, _, err := submit(t, s, t0.Add(time.Minute), "AUT-1", OpRecover, chk2, nil); err != nil {
		t.Fatalf("recover after a failed apply: %v", err)
	}
}

func TestDailyCapAndRollingWindow(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	max := DefaultLimits().MaxExecutionsPerDay
	for i := 0; i < max; i++ {
		at := t0.Add(time.Duration(i) * time.Minute)
		goodCheck(t, s, at, fmt.Sprintf("AUT-%d", i))
	}
	at := t0.Add(time.Hour)
	_, _, nonce, err := submit(t, s, at, "AUT-99", OpCheck, "", nil)
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.Limit != "daily" {
		t.Fatalf("want the daily limit, got %v", err)
	}
	if want := 23 * time.Hour; rl.RetryAfter != want {
		t.Fatalf("retry after %s, want %s (the oldest execution leaves the window)", rl.RetryAfter, want)
	}
	if nonceSpent(t, s, nonce) {
		t.Fatal("nonce spent despite the refusal")
	}
	// 24h after the first execution, one slot is free again
	if _, _, _, err := submit(t, s, t0.Add(24*time.Hour+time.Second), "AUT-99", OpCheck, "", nil); err != nil {
		t.Fatalf("after the window rolled: %v", err)
	}
}

func TestLimitsStayOnStoreLimits(t *testing.T) {
	// a stricter configured cap is the one enforced
	p := t.TempDir() + "/s.db"
	lim := DefaultLimits()
	lim.MaxExecutionsPerDay = 1
	s, err := Open(p, lim)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	t0 := time.Unix(1_800_000_000, 0)
	goodCheck(t, s, t0, "AUT-1")
	if _, _, _, err := submit(t, s, t0, "AUT-2", OpCheck, "", nil); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got %v", err)
	}
}

func TestApprovalTTLAndExpiry(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	if _, err := s.IssueNonce(bg, testHost, 5*time.Minute+time.Second, t0); err == nil {
		t.Fatal("a nonce longer than the 5-minute approval TTL was issued")
	}
	if _, err := s.IssueNonce(bg, testHost, 5*time.Minute, t0); err != nil {
		t.Fatal(err)
	}
	e := testEnvelope(t, t0)
	e.ExpiresAt = e.IssuedAt + 301
	if err := e.Validate(DefaultLimits()); err == nil {
		t.Fatal("an envelope valid for more than 5 minutes validated")
	}
	// single-use: consumed at the last valid second, refused at expiry (no sleeping: Now is injected)
	n, _ := s.IssueNonce(bg, testHost, 4*time.Minute, t0)
	e = testEnvelope(t, t0)
	e.Nonce = n.Value
	dg, _ := e.Digest()
	req := ConsumeRequest{Env: e, EnvelopeDigest: dg, Signer: Signer{Principal: principal, Fingerprint: "SHA256:t"}, Host: testHost, Now: t0.Add(4*time.Minute - time.Second)}
	if _, err := s.Consume(bg, req); err != nil {
		t.Fatalf("one second before expiry: %v", err)
	}
	n2, _ := s.IssueNonce(bg, testHost, 4*time.Minute, t0)
	e2 := testEnvelope(t, t0)
	e2.Nonce, e2.CardID = n2.Value, "AUT-2"
	dg2, _ := e2.Digest()
	req.Env, req.EnvelopeDigest, req.Now = e2, dg2, t0.Add(4*time.Minute)
	if _, err := s.Consume(bg, req); err == nil {
		t.Fatal("approval accepted at its expiry")
	}
}

// ---- T2.11 receipts and check freshness -------------------------------------------------------

func TestApplyWithoutReceiptRefused(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	// a made-up check attempt
	_, _, nonce, err := submit(t, s, t0, "AUT-1", OpApply, "", nil)
	if !errors.Is(err, ErrCheckReceipt) {
		t.Fatalf("forged check citation accepted: %v", err)
	}
	if nonceSpent(t, s, nonce) {
		t.Fatal("nonce spent despite the refusal")
	}
	// a check still running, and one that failed, leave no receipt
	a, _, _, _ := submit(t, s, t0, "AUT-1", OpCheck, "", nil)
	if _, _, _, err := submit(t, s, t0, "AUT-2", OpApply, a.ID, nil); !errors.Is(err, ErrCheckReceipt) {
		t.Fatalf("running check accepted: %v", err)
	}
	finish(t, s, a, t0, EndInfo{Result: ResultFailed, ExitStatus: 3, OutputSHA256: outHex, OutputBytes: 1, Duration: time.Second})
	if _, _, _, err := submit(t, s, t0, "AUT-1", OpApply, a.ID, nil); !errors.Is(err, ErrCheckReceipt) {
		t.Fatalf("failed check accepted: %v", err)
	}
	// a timed-out check, even with exit status 0 in the report, is not success
	b, _, _, _ := submit(t, s, t0, "AUT-3", OpCheck, "", nil)
	finish(t, s, b, t0, EndInfo{Result: ResultTimeout, ExitStatus: 0, OutputSHA256: outHex, OutputBytes: 1, Duration: time.Second})
	if _, _, _, err := submit(t, s, t0, "AUT-3", OpApply, b.ID, nil); !errors.Is(err, ErrCheckReceipt) {
		t.Fatalf("timed-out check accepted: %v", err)
	}
}

func TestReceiptBinding(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	chk := goodCheck(t, s, t0, "AUT-1")
	other := strings.Repeat("9", 64)
	oddDeps, _ := DepsDigest([]Dep{{Path: "/work/lib.sh", SHA256: sha2Hex}})
	cases := []struct {
		name string
		f    func(*Envelope)
	}{
		{"script bytes", func(e *Envelope) { e.ScriptSHA256 = other }},
		{"card revision", func(e *Envelope) { e.CardRev = "rev-2" }},
		{"dependencies", func(e *Envelope) { e.Deps = []Dep{{Path: "/work/lib.sh", SHA256: sha2Hex}}; e.DepsDigest = oddDeps }},
		{"output digest", func(e *Envelope) { o := other; e.CheckOutputSHA = &o }},
		{"card", func(e *Envelope) { e.CardID = "AUT-2" }},
	}
	for _, c := range cases {
		_, _, nonce, err := submit(t, s, t0, "AUT-1", OpApply, chk, c.f)
		if !errors.Is(err, ErrCheckReceipt) {
			t.Errorf("%s: want ErrCheckReceipt, got %v", c.name, err)
		}
		if nonceSpent(t, s, nonce) {
			t.Errorf("%s: nonce spent", c.name)
		}
	}
	// the matching envelope is accepted
	if _, _, _, err := submit(t, s, t0, "AUT-1", OpApply, chk, nil); err != nil {
		t.Fatalf("matching apply: %v", err)
	}
}

func TestReceiptCannotBeAnApplyAttempt(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	chk := goodCheck(t, s, t0, "AUT-1")
	a, _, _, _ := submit(t, s, t0, "AUT-1", OpApply, chk, nil)
	finish(t, s, a, t0, okEnd())
	// citing the successful apply as if it were a check
	if _, _, _, err := submit(t, s, t0.Add(11*time.Minute), "AUT-1", OpApply, a.ID, nil); !errors.Is(err, ErrCheckReceipt) {
		t.Fatalf("apply cited as a check: %v", err)
	}
}

func TestCheckFreshness(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	for i, c := range []struct {
		age   time.Duration
		fresh bool
	}{{29*time.Minute + 59*time.Second, true}, {30 * time.Minute, true}, {30*time.Minute + time.Second, false}, {3 * time.Hour, false}} {
		card := fmt.Sprintf("AUT-%d", i)
		chk := goodCheck(t, s, t0, card)
		a, _, nonce, err := submit(t, s, t0.Add(c.age), card, OpApply, chk, nil)
		if c.fresh {
			if err != nil {
				t.Fatalf("age %s: %v", c.age, err)
			}
			finish(t, s, a, t0.Add(c.age), okEnd())
			continue
		}
		if !errors.Is(err, ErrCheckReceipt) || nonceSpent(t, s, nonce) {
			t.Errorf("age %s: want a stale-check refusal with the nonce unspent, got %v", c.age, err)
		}
	}
}

func TestReceiptSurvivesRestart(t *testing.T) {
	p := t.TempDir() + "/s.db"
	s, err := Open(p, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Unix(1_800_000_000, 0)
	chk := goodCheck(t, s, t0, "AUT-1")
	s.Close()
	s, err = Open(p, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, _, err := submit(t, s, t0.Add(time.Minute), "AUT-1", OpApply, chk, nil); err != nil {
		t.Fatalf("receipt lost across restart: %v", err)
	}
	// the receipt table cannot be rewritten or emptied
	if _, err := s.db.Exec(`UPDATE check_receipts SET ended_at=0`); err == nil {
		t.Fatal("receipt updated")
	}
	if _, err := s.db.Exec(`DELETE FROM check_receipts`); err == nil {
		t.Fatal("receipt deleted")
	}
}

func TestRecoverEndedCheckGetsReceipt(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	a, _, _, _ := submit(t, s, t0, "AUT-1", OpCheck, "", nil)
	if err := recStarted(s, bg, a.ID, StartInfo{Unit: "run-test.service", InvocationID: strings.Repeat("e", 32), CASSHA: a.ScriptSHA256}, t0); err != nil {
		t.Fatal(err)
	}
	look := func(context.Context, string) (EndInfo, bool, error) { return okEnd(), true, nil }
	if _, err := s.Recover(bg, look, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := submit(t, s, t0.Add(2*time.Minute), "AUT-1", OpApply, a.ID, nil); err != nil {
		t.Fatalf("check settled by Recover has no receipt: %v", err)
	}
}

// ---- runtime caps -----------------------------------------------------------------------------

func TestRuntimeCapsAreValuesForTheLauncher(t *testing.T) {
	l := DefaultLimits()
	if l.RuntimeCap(OpCheck) != 10*time.Minute || l.RuntimeCap(OpApply) != 3*time.Hour || l.RuntimeCap(OpRecover) != 3*time.Hour {
		t.Fatalf("caps %v %v %v", l.RuntimeCap(OpCheck), l.RuntimeCap(OpApply), l.RuntimeCap(OpRecover))
	}
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	a, _, _, err := submit(t, s, t0, "AUT-1", OpCheck, "", func(e *Envelope) { e.RuntimeMaxS = 123 })
	if err != nil {
		t.Fatal(err)
	}
	if a.RuntimeMax != 123*time.Second {
		t.Fatalf("attempt runtime %s, want the signed 123s", a.RuntimeMax)
	}
	over := testEnvelope(t, t0)
	over.RuntimeMaxS = 601
	if err := over.Validate(DefaultLimits()); err == nil {
		t.Fatal("check above the 600s cap validated")
	}
	over.Op, over.RuntimeMaxS = OpApply, 10801
	c, o := testAttemp, sha3Hex
	over.CheckAttempt, over.CheckOutputSHA = &c, &o
	if err := over.Validate(DefaultLimits()); err == nil {
		t.Fatal("apply above the 10800s cap validated")
	}
}

// ---- lockout ----------------------------------------------------------------------------------

func TestLockoutAfterThreeBadSignaturesInWindow(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	for i, want := range []bool{false, false, true} {
		got, err := s.RecordSigFailure(bg, "bad", t0.Add(time.Duration(i)*4*time.Minute))
		if err != nil || got != want {
			t.Fatalf("failure %d: locked=%v err=%v", i+1, got, err)
		}
	}
	if _, _, _, err := submit(t, s, t0.Add(9*time.Minute), "AUT-1", OpCheck, "", nil); !errors.Is(err, ErrLocked) {
		t.Fatalf("locked verifier consumed an approval: %v", err)
	}
	if locked, _ := s.Locked(bg); !locked {
		t.Fatal("not reported locked")
	}
	// time does not unlock it
	if _, _, _, err := submit(t, s, t0.Add(48*time.Hour), "AUT-1", OpCheck, "", nil); !errors.Is(err, ErrLocked) {
		t.Fatalf("lock expired by itself: %v", err)
	}
	if err := s.Unlock(bg, t0.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := submit(t, s, t0.Add(48*time.Hour), "AUT-1", OpCheck, "", nil); err != nil {
		t.Fatalf("after unlock: %v", err)
	}
}

func TestSpacedFailuresDoNotLock(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	for i := 0; i < 6; i++ {
		locked, err := s.RecordSigFailure(bg, "bad", t0.Add(time.Duration(i)*6*time.Minute))
		if err != nil || locked {
			t.Fatalf("failure %d locked=%v err=%v: only 2 fit in any 10-minute window", i+1, locked, err)
		}
	}
}

func TestFailuresBeforeUnlockDoNotCountAgain(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	for i := 0; i < 3; i++ {
		s.RecordSigFailure(bg, "bad", t0.Add(time.Duration(i)*time.Second))
	}
	if err := s.Unlock(bg, t0.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if locked, _ := s.RecordSigFailure(bg, "bad", t0.Add(20*time.Second)); locked {
		t.Fatal("old failures re-locked the verifier after the unlock")
	}
	s.RecordSigFailure(bg, "bad", t0.Add(21*time.Second))
	if locked, _ := s.RecordSigFailure(bg, "bad", t0.Add(22*time.Second)); !locked {
		t.Fatal("three new failures did not lock")
	}
}

func TestLockSurvivesRestart(t *testing.T) {
	p := t.TempDir() + "/s.db"
	s, _ := Open(p, DefaultLimits())
	t0 := time.Unix(1_800_000_000, 0)
	for i := 0; i < 3; i++ {
		s.RecordSigFailure(bg, "bad", t0)
	}
	s.Close()
	s, err := Open(p, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if locked, _ := s.Locked(bg); !locked {
		t.Fatal("lock lost across restart")
	}
}

func TestStoreVerifyApprovalLocksOnBadSignatures(t *testing.T) {
	s, _ := newStore(t)
	k := newKit(t, "7", "-1h:+1h")
	now := time.Now()
	e := testEnvelope(t, now)
	b := canon(t, e)
	good := k.sign(t, b, DefaultNamespace, false)
	wrongNS := k.sign(t, b, "git", false)
	v := k.verifier()

	// an envelope-form failure is not a bad signature
	if _, _, _, err := s.VerifyApproval(bg, v, testHost, []byte("{}"), good, now); err == nil || errors.Is(err, ErrSignature) {
		t.Fatalf("malformed envelope: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, _, _, err := s.VerifyApproval(bg, v, testHost, b, wrongNS, now); !errors.Is(err, ErrSignature) || errors.Is(err, ErrLocked) {
			t.Fatalf("bad signature %d: %v", i+1, err)
		}
	}
	if _, _, _, err := s.VerifyApproval(bg, v, testHost, b, wrongNS, now); !errors.Is(err, ErrSignature) || !errors.Is(err, ErrLocked) {
		t.Fatalf("third bad signature should report the lock: %v", err)
	}
	// locked: even a good signature is refused without being verified
	if _, _, _, err := s.VerifyApproval(bg, v, testHost, b, good, now); !errors.Is(err, ErrLocked) {
		t.Fatalf("good signature while locked: %v", err)
	}
	if err := s.Unlock(bg, now); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.VerifyApproval(bg, v, testHost, b, good, now); err != nil {
		t.Fatalf("good signature after unlock: %v", err)
	}
}
