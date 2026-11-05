//go:build unix

package review

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/homerun"
)

const gateHost = "0123456789abcdef0123456789abcdef"

type gateRig struct {
	t     *testing.T
	f     *fixture
	store *homerun.Store
	cas   *homerun.CAS
	now   time.Time
}

func newGateRig(t *testing.T, res ProviderResult) *gateRig {
	t.Helper()
	f := newFixture(t, res)
	dir := t.TempDir()
	root := filepath.Join(dir, "receipts")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cas := &homerun.CAS{Root: root, OwnerUID: -1, MaxBytes: 64 << 10}
	s, err := homerun.Open(filepath.Join(dir, "state.db"), homerun.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetReviewGate(f.v.Gate(cas))
	return &gateRig{t: t, f: f, store: s, cas: cas, now: f.exp.Now}
}

// put stores raw receipt bytes under their own digest and returns it.
func (g *gateRig) put(raw []byte) string {
	g.t.Helper()
	sha := hx2(raw)
	if _, err := g.cas.Put(raw, sha); err != nil {
		g.t.Fatal(err)
	}
	return sha
}

func hx2(b []byte) string { return hx(string(b)) }

// envelope builds a valid check envelope for the fixture's script, deps and flags naming review.
func (g *gateRig) envelope(review string) (*homerun.Envelope, string) {
	g.t.Helper()
	n, err := g.store.IssueNonce(context.Background(), gateHost, 4*time.Minute, g.now)
	if err != nil {
		g.t.Fatal(err)
	}
	hd := make([]homerun.Dep, len(deps))
	for i, d := range deps {
		hd[i] = homerun.Dep{Path: d.Path, SHA256: hx(string(d.Content))}
	}
	e := &homerun.Envelope{V: homerun.EnvelopeVersion, NS: homerun.DefaultNamespace, Op: homerun.OpCheck, Host: gateHost,
		Project: "autarch", CardID: "AUT-1", CardRev: "rev-1", ScriptSHA256: g.f.exp.ScriptSHA256, ScriptPath: "/work/run.sh",
		Deps: hd, DepsDigest: g.f.exp.DepsDigest, Flags: flags, Privilege: homerun.PrivMk, RuntimeMaxS: 600, RulingID: "q1",
		LintSHA256: strings.Repeat("2", 64), ReviewSHA256: review, Nonce: n.Value, IssuedAt: g.now.Unix(), ExpiresAt: g.now.Add(4 * time.Minute).Unix()}
	if err := e.Validate(homerun.DefaultLimits()); err != nil {
		g.t.Fatal(err)
	}
	dg, err := e.Digest()
	if err != nil {
		g.t.Fatal(err)
	}
	return e, dg
}

func (g *gateRig) consume(e *homerun.Envelope, dg string) error {
	_, err := g.store.Consume(context.Background(), homerun.ConsumeRequest{Env: e, EnvelopeDigest: dg,
		Signer: homerun.Signer{Principal: "approver", KeyType: "ED25519-CERT", Fingerprint: "SHA256:test"}, Host: gateHost, Now: g.now})
	return err
}

// refuseKeepsNonce asserts a review refusal with the reason, one recorded refusal, and an unspent nonce
// (the same nonce then succeeds with a good receipt).
func (g *gateRig) refuseKeepsNonce(e *homerun.Envelope, dg, reason string) {
	g.t.Helper()
	err := g.consume(e, dg)
	var re *homerun.ReviewError
	if !errors.Is(err, homerun.ErrReview) || !errors.As(err, &re) || re.Reason != reason {
		g.t.Fatalf("got %v, want review refusal %s", err, reason)
	}
	rs, err := g.store.ReviewRefusals(context.Background(), 0, 100)
	if err != nil || len(rs) == 0 || rs[len(rs)-1].Reason != reason || rs[len(rs)-1].Nonce != e.Nonce || rs[len(rs)-1].CardID != "AUT-1" {
		g.t.Fatalf("refusal not recorded: %v %+v", err, rs)
	}
	recs, err := g.store.Records(context.Background())
	if err != nil || len(recs) != 0 {
		g.t.Fatalf("a refusal must start no attempt: %v %d", err, len(recs))
	}
	good := g.put(g.f.issue(g.t))
	e2 := *e
	e2.ReviewSHA256 = good
	dg2, _ := e2.Digest()
	if _, err := g.store.Consume(context.Background(), homerun.ConsumeRequest{Env: &e2, EnvelopeDigest: dg2,
		Signer: homerun.Signer{Principal: "approver", KeyType: "ED25519-CERT", Fingerprint: "SHA256:test"}, Host: gateHost, Now: g.now}); err != nil {
		g.t.Fatalf("nonce was spent by the refusal: %v", err)
	}
}

func TestGateHappyPath(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	e, dg := g.envelope(g.put(g.f.issue(t)))
	if err := g.consume(e, dg); err != nil {
		t.Fatal(err)
	}
	if rs, _ := g.store.ReviewRefusals(context.Background(), 0, 100); len(rs) != 0 {
		t.Fatalf("unexpected refusals %+v", rs)
	}
}

func TestGateMissingReceipt(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	e, dg := g.envelope(strings.Repeat("a", 64))
	g.refuseKeepsNonce(e, dg, homerun.ReviewMissing)
}

func TestGateNoGateRefusesEverything(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	g.store.SetReviewGate(nil)
	e, dg := g.envelope(g.put(g.f.issue(t)))
	if err := g.consume(e, dg); !errors.Is(err, homerun.ErrNoReviewGate) {
		t.Fatalf("got %v", err)
	}
	rs, err := g.store.ReviewRefusals(context.Background(), 0, 100)
	if err != nil || len(rs) != 1 || rs[0].Reason != homerun.ReviewNoGate || rs[0].Nonce != e.Nonce || rs[0].Count != 1 {
		t.Fatalf("no-gate refusal not recorded: %v %+v", err, rs)
	}
}

func TestGateUnparsableReceipt(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	e, dg := g.envelope(g.put([]byte("not a receipt")))
	g.refuseKeepsNonce(e, dg, homerun.ReviewRefused)
}

func TestGateFailOutcome(t *testing.T) {
	g := newGateRig(t, okResult("FAIL"))
	e, dg := g.envelope(g.put(g.f.issue(t)))
	g.f = newFixtureWithSigner(t, g.f.sg, okResult("PASS")) // so refuseKeepsNonce can issue a good one
	g.store.SetReviewGate(g.f.v.Gate(g.cas))
	g.refuseKeepsNonce(e, dg, homerun.ReviewRefused)
}

func newFixtureWithSigner(t *testing.T, sg *testSigner, res ProviderResult) *fixture {
	f := newFixture(t, res)
	f.sg = sg
	f.svc.Signer = sg
	f.v = &Verifier{PublicKey: sg.PublicKey()}
	return f
}

// issueFor issues a PASS receipt for different material than the envelope names.
func (g *gateRig) issueFor(req Request) string {
	g.t.Helper()
	raw, err := g.f.svc.Issue(context.Background(), req)
	if err != nil {
		g.t.Fatal(err)
	}
	return g.put(raw)
}

func TestGateWrongScriptDepsFlags(t *testing.T) {
	for name, req := range map[string]Request{
		"script": {Script: []byte("#!/bin/bash\necho other\n"), Deps: deps, Flags: flags},
		"deps":   {Script: script, Deps: deps[:1], Flags: flags},
		"flags":  {Script: script, Deps: deps, Flags: []string{"--other"}},
	} {
		t.Run(name, func(t *testing.T) {
			g := newGateRig(t, okResult("PASS"))
			e, dg := g.envelope(g.issueFor(req))
			g.refuseKeepsNonce(e, dg, homerun.ReviewRefused)
		})
	}
}

func TestGateTamperedBody(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	raw := g.f.issue(t)
	// Change the model name inside the signed body, keeping the receipt well formed and canonical.
	bad := bytes.Replace(raw, []byte("test-model-1"), []byte("test-model-2"), 1)
	if bytes.Equal(bad, raw) {
		t.Fatal("tamper did not change the bytes")
	}
	e, dg := g.envelope(g.put(bad))
	g.refuseKeepsNonce(e, dg, homerun.ReviewRefused)
}

func TestGateWrongKey(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	other := newSigner(t)
	raw, err := (&Service{Provider: &testProvider{res: okResult("PASS")}, Signer: other, TTL: 10 * time.Minute,
		Now: func() time.Time { return t0 }}).Issue(context.Background(), Request{Script: script, Deps: deps, Flags: flags})
	if err != nil {
		t.Fatal(err)
	}
	e, dg := g.envelope(g.put(raw))
	g.refuseKeepsNonce(e, dg, homerun.ReviewRefused)
}

func TestGateExpiredReceipt(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	raw := g.f.issue(t) // valid t0 .. t0+10min
	g.now = t0.Add(11 * time.Minute)
	g.f.svc.Now = func() time.Time { return g.now } // the retry's good receipt is fresh at the new time
	e, dg := g.envelope(g.put(raw))
	g.refuseKeepsNonce(e, dg, homerun.ReviewRefused)
}

// A receipt stored under another receipt's name is not served by the content-addressed store.
func TestGateMisfiledReceipt(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	raw := g.f.issue(t)
	name := strings.Repeat("b", 64)
	if err := os.WriteFile(filepath.Join(g.cas.Root, name), raw, 0o444); err != nil {
		t.Fatal(err)
	}
	e, dg := g.envelope(name)
	g.refuseKeepsNonce(e, dg, homerun.ReviewMissing)
}

// Applies to every op, not only check.
func TestGateCoversApply(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	e, dg := g.envelope(strings.Repeat("c", 64))
	a, o := "01ARZ3NDEKTSV4RRFFQ69G5FAV", strings.Repeat("3", 64)
	e.Op, e.CheckAttempt, e.CheckOutputSHA = homerun.OpApply, &a, &o
	dg, _ = e.Digest()
	err := g.consume(e, dg)
	if !errors.Is(err, homerun.ErrReview) {
		t.Fatalf("got %v", err)
	}
}

// The receipt can expire while the write lock is awaited: it is rechecked against the store's clock once
// the lock is held, and a late receipt leaves the nonce unspent and is recorded.
func TestGateReceiptExpiresWhileWaitingForLock(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	var off atomic.Int64
	g.store.SetClock(func() time.Time { return g.now.Add(time.Duration(off.Load())) })
	raw := g.f.issue(t) // valid t0 .. t0+10min
	e, dg := g.envelope(g.put(raw))
	dbPath := filepath.Join(filepath.Dir(g.cas.Root), "state.db")
	other, err := sql.Open("sqlite", "file:"+dbPath+"?_txlock=immediate&_pragma=busy_timeout(10000)")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	tx, err := other.BeginTx(context.Background(), nil) // immediate: holds the write lock
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- g.consume(e, dg) }()
	select {
	case err := <-done:
		t.Fatalf("consume did not wait for the lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	off.Store(int64(11 * time.Minute)) // the receipt has now expired
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	err = <-done
	var re *homerun.ReviewError
	if !errors.Is(err, homerun.ErrReview) || !errors.As(err, &re) || re.Reason != homerun.ReviewRefused {
		t.Fatalf("got %v, want a review refusal", err)
	}
	rs, _ := g.store.ReviewRefusals(context.Background(), 0, 100)
	if len(rs) != 1 || rs[0].Nonce != e.Nonce {
		t.Fatalf("refusal not recorded: %+v", rs)
	}
	// the nonce is unspent: with the clock back, the same envelope succeeds
	off.Store(0)
	if err := g.consume(e, dg); err != nil {
		t.Fatalf("nonce was spent: %v", err)
	}
}

// Replaying one refused envelope keeps one row and counts it.
func TestGateReplayAggregates(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	e, dg := g.envelope(strings.Repeat("a", 64))
	for i := 0; i < 1000; i++ {
		g.now = g.now.Add(time.Millisecond * 0) // same time; count only
		if err := g.consume(e, dg); !errors.Is(err, homerun.ErrReview) {
			t.Fatal(err)
		}
	}
	rs, err := g.store.ReviewRefusals(context.Background(), 0, 100)
	if err != nil || len(rs) != 1 || rs[0].Count != 1000 {
		t.Fatalf("want one row with count 1000: %v %+v", err, rs)
	}
	if rs[0].FirstAt.IsZero() || rs[0].LastAt.Before(rs[0].FirstAt) {
		t.Fatalf("bad timestamps %+v", rs[0])
	}
}

// Pages come back in order and the cursor resumes after the last row seen.
func TestGateRefusalsPagination(t *testing.T) {
	g := newGateRig(t, okResult("PASS"))
	for i := 0; i < 5; i++ {
		e, dg := g.envelope(fmt.Sprintf("%064x", i+1))
		if err := g.consume(e, dg); !errors.Is(err, homerun.ErrReview) {
			t.Fatal(err)
		}
	}
	var got []int64
	var after int64
	for {
		page, err := g.store.ReviewRefusals(context.Background(), after, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) > 2 {
			t.Fatalf("page too large: %d", len(page))
		}
		if len(page) == 0 {
			break
		}
		for _, r := range page {
			got = append(got, r.Seq)
		}
		after = page[len(page)-1].Seq
	}
	if len(got) != 5 {
		t.Fatalf("got %v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("not in order: %v", got)
		}
	}
}
