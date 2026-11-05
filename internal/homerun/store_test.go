//go:build unix

package homerun

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// T2.3 — a nonce is spent once, across concurrency, handles and crashes.

func TestOpenCreatesPrivateDatabase(t *testing.T) {
	_, p := newStore(t)
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("state database mode %v, want 0600", fi.Mode().Perm())
	}
}

func TestConsumeOnceAndReplayRefused(t *testing.T) {
	s, _ := newStore(t)
	now := time.Now()
	a, e, err := consumeFor(t, s, now, "AUT-1", OpCheck)
	if err != nil {
		t.Fatal(err)
	}
	if a.State != KindAuthorized || len(a.ID) != 26 || a.CardID != "AUT-1" {
		t.Fatalf("unexpected attempt %+v", a)
	}
	// replay of the very same approval, while the attempt is still active and after it has settled
	dg, _ := e.Digest()
	req := ConsumeRequest{Env: e, EnvelopeDigest: dg, Signer: a.Signer, Host: testHost, Now: now}
	if _, err := s.Consume(context.Background(), req); !errors.Is(err, ErrNonceUnavailable) {
		t.Fatalf("replay while active: %v", err)
	}
	if err := s.RecordAborted(context.Background(), a.ID, "test", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Consume(context.Background(), req); !errors.Is(err, ErrNonceUnavailable) {
		t.Fatalf("replay after abort: %v", err)
	}
}

func TestConsumeConcurrentExactlyOneWinner(t *testing.T) {
	s, p := newStore(t)
	s2, err := openStub(p, DefaultLimits()) // a second handle on the same file, as a second process would hold
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	now := time.Now()
	n, err := s.IssueNonce(context.Background(), testHost, 4*time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	e := testEnvelope(t, now)
	e.Nonce = n.Value
	dg, _ := e.Digest()
	req := ConsumeRequest{Env: e, EnvelopeDigest: dg, Signer: Signer{Principal: principal, Fingerprint: "SHA256:x"}, Host: testHost, Now: now}

	var wins, unavailable, other atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 32; i++ {
		st := s
		if i%2 == 1 {
			st = s2
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := st.Consume(context.Background(), req)
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, ErrNonceUnavailable):
				unavailable.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
				other.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if wins.Load() != 1 || unavailable.Load() != 31 || other.Load() != 0 {
		t.Fatalf("wins=%d unavailable=%d other=%d", wins.Load(), unavailable.Load(), other.Load())
	}
	recs, _ := s.Records(context.Background())
	if len(recs) != 1 || recs[0].Kind != KindAuthorized {
		t.Fatalf("audit holds %d records after one win", len(recs))
	}
}

func TestNonceExpiryHostAndUnissued(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	now := time.Now()
	dg := func(e *Envelope) string { d, _ := e.Digest(); return d }
	sg := Signer{Principal: principal, Fingerprint: "SHA256:x"}

	// the nonce lives 30 s although the signed approval would last 4 min: the nonce row decides
	n, _ := s.IssueNonce(ctx, testHost, 30*time.Second, now)
	e := testEnvelope(t, now)
	e.Nonce = n.Value
	if _, err := s.Consume(ctx, ConsumeRequest{Env: e, EnvelopeDigest: dg(e), Signer: sg, Host: testHost, Now: now.Add(31 * time.Second)}); !errors.Is(err, ErrNonceUnavailable) {
		t.Fatalf("expired nonce: %v", err)
	}
	// and it does not revive: no later consume works, and the row cannot be edited back
	if _, err := s.Consume(ctx, ConsumeRequest{Env: e, EnvelopeDigest: dg(e), Signer: sg, Host: testHost, Now: now}); err != nil {
		// within its lifetime it is still unspent, so this one is allowed to win
		t.Fatalf("unspent nonce within its lifetime refused: %v", err)
	}

	// a nonce issued for another host
	n2, _ := s.IssueNonce(ctx, otherHost, 4*time.Minute, now)
	e2 := testEnvelope(t, now)
	e2.CardID, e2.Nonce = "AUT-2", n2.Value
	if _, err := s.Consume(ctx, ConsumeRequest{Env: e2, EnvelopeDigest: dg(e2), Signer: sg, Host: testHost, Now: now}); !errors.Is(err, ErrNonceUnavailable) {
		t.Fatalf("nonce of another host: %v", err)
	}
	// an envelope signed for another host
	e3 := testEnvelope(t, now)
	e3.Host = otherHost
	if _, err := s.Consume(ctx, ConsumeRequest{Env: e3, EnvelopeDigest: dg(e3), Signer: sg, Host: testHost, Now: now}); !errors.Is(err, ErrEnvelope) {
		t.Fatalf("envelope of another host: %v", err)
	}
	// a nonce nobody issued
	e4 := testEnvelope(t, now)
	e4.CardID, e4.Nonce = "AUT-4", strings.Repeat("d", 32)
	if _, err := s.Consume(ctx, ConsumeRequest{Env: e4, EnvelopeDigest: dg(e4), Signer: sg, Host: testHost, Now: now}); !errors.Is(err, ErrNonceUnavailable) {
		t.Fatalf("unissued nonce: %v", err)
	}
	// an expired approval is refused before any state is touched
	e5 := testEnvelope(t, now)
	if _, err := s.Consume(ctx, ConsumeRequest{Env: e5, EnvelopeDigest: dg(e5), Signer: sg, Host: testHost, Now: now.Add(10 * time.Minute)}); !errors.Is(err, ErrEnvelope) {
		t.Fatalf("expired approval: %v", err)
	}
}

func TestConsumedNonceCannotBeRevivedOrRemoved(t *testing.T) {
	s, p := newStore(t)
	a, e, err := consumeFor(t, s, time.Now(), "AUT-1", OpCheck)
	if err != nil {
		t.Fatal(err)
	}
	_ = a
	raw, err := sql.Open("sqlite", "file:"+p+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`UPDATE nonces SET consumed_at=NULL WHERE nonce=?`, e.Nonce); err == nil {
		t.Error("consumed nonce was revived")
	}
	if _, err := raw.Exec(`DELETE FROM nonces WHERE nonce=?`, e.Nonce); err == nil {
		t.Error("nonce row was deleted")
	}
	if _, err := raw.Exec(`UPDATE audit SET body='{}' WHERE seq=1`); err == nil {
		t.Error("audit row was rewritten")
	}
	if _, err := raw.Exec(`DELETE FROM audit WHERE seq=1`); err == nil {
		t.Error("audit row was deleted")
	}
}

func TestBusyOneActivePerCardAndCaps(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	now := time.Now()
	a1, _, err := consumeFor(t, s, now, "AUT-1", OpCheck)
	if err != nil {
		t.Fatal(err)
	}
	// same card again: busy, and the nonce stays unspent
	n, _ := s.IssueNonce(ctx, testHost, 4*time.Minute, now)
	e := testEnvelope(t, now)
	e.Nonce = n.Value
	dg, _ := e.Digest()
	req := ConsumeRequest{Env: e, EnvelopeDigest: dg, Signer: Signer{Principal: principal, Fingerprint: "SHA256:x"}, Host: testHost, Now: now}
	if _, err := s.Consume(ctx, req); !errors.Is(err, ErrBusy) {
		t.Fatalf("second attempt on a card: %v", err)
	}
	// second card fits (cap 2 as-mk), third is over the cap
	if _, _, err := consumeFor(t, s, now, "AUT-2", OpCheck); err != nil {
		t.Fatalf("second card: %v", err)
	}
	if _, _, err := consumeFor(t, s, now, "AUT-3", OpCheck); !errors.Is(err, ErrBusy) {
		t.Fatalf("third card over the cap: %v", err)
	}
	// settle the first; the card is free again and the earlier busy nonce is still good
	if err := s.RecordAborted(ctx, a1.ID, "test", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Consume(ctx, req); err != nil {
		t.Fatalf("after the first attempt settled: %v", err)
	}
}

// ---- crash tests: a helper process does part of a run and is killed with SIGKILL ----

const helperEnv = "HOMERUN_TEST_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		runHelper(mode)
		return
	}
	os.Exit(m.Run())
}

// runHelper opens the state database, consumes the envelope in the file named by HOMERUN_ENV, optionally
// records STARTED, prints the attempt ID, then kills itself without any cleanup.
func runHelper(mode string) {
	ctx := context.Background()
	s, err := openStub(os.Getenv("HOMERUN_DB"), DefaultLimits())
	if err != nil {
		os.Stderr.WriteString("open: " + err.Error())
		os.Exit(2)
	}
	b, err := os.ReadFile(os.Getenv("HOMERUN_ENV"))
	if err != nil {
		os.Exit(2)
	}
	e, err := ParseEnvelope(b)
	if err != nil {
		os.Stderr.WriteString("parse: " + err.Error())
		os.Exit(2)
	}
	dg, _ := e.Digest()
	now := time.Now()
	a, err := s.Consume(ctx, ConsumeRequest{Env: e, EnvelopeDigest: dg, Signer: Signer{Principal: principal, Fingerprint: "SHA256:x"}, Host: testHost, Now: now})
	if err != nil {
		os.Stderr.WriteString("consume: " + err.Error())
		os.Exit(3)
	}
	if mode == "started" {
		if err := recStarted(s, ctx, a.ID, StartInfo{Unit: "run-test.service", InvocationID: strings.Repeat("e", 32), CASSHA: e.ScriptSHA256}, now); err != nil {
			os.Stderr.WriteString("started: " + err.Error())
			os.Exit(4)
		}
	}
	os.Stdout.WriteString(a.ID + "\n")
	os.Stdout.Sync()
	syscall.Kill(os.Getpid(), syscall.SIGKILL)
	select {}
}

// crashRun issues a nonce in dbPath, has a helper consume it (and maybe record STARTED), and returns the
// attempt ID and envelope after the helper died of SIGKILL.
func crashRun(t *testing.T, dbPath, mode string, now time.Time) (string, *Envelope) {
	t.Helper()
	s, err := openStub(dbPath, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.IssueNonce(context.Background(), testHost, 4*time.Minute, now)
	s.Close()
	if err != nil {
		t.Fatal(err)
	}
	e := testEnvelope(t, now)
	e.Nonce = n.Value
	envPath := filepath.Join(t.TempDir(), "env.json")
	if err := os.WriteFile(envPath, canon(t, e), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = []string{helperEnv + "=" + mode, "HOMERUN_DB=" + dbPath, "HOMERUN_ENV=" + envPath}
	out, err := cmd.Output()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("helper did not die: %v", err)
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); !ok || !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
		t.Fatalf("helper exit %v, stderr %q", err, ee.Stderr)
	}
	return strings.TrimSpace(string(out)), e
}

func TestCrashAfterConsumeRecoversAsAbortedAndNeverRetries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	now := time.Now()
	id, e := crashRun(t, dbPath, "consumed", now)

	s, err := openStub(dbPath, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	recs, _ := s.Records(ctx)
	if len(recs) != 1 || recs[0].Attempt != id || recs[0].Kind != KindAuthorized {
		t.Fatalf("after the crash the audit holds %+v", recs)
	}
	lookups := 0
	recs, err = s.Recover(ctx, func(context.Context, string) (EndInfo, bool, error) { lookups++; return EndInfo{}, false, nil }, now)
	if err != nil {
		t.Fatal(err)
	}
	if lookups != 0 {
		t.Fatal("recovery asked systemd about a unit that was never launched")
	}
	if len(recs) != 2 || recs[1].Kind != KindAborted || recs[1].Attempt != id {
		t.Fatalf("recovery wrote %+v", recs)
	}
	if err := s.CheckAuditChain(ctx, true); err != nil {
		t.Fatal(err)
	}
	// the nonce is spent: the same approval does not run again
	dg, _ := e.Digest()
	if _, err := s.Consume(ctx, ConsumeRequest{Env: e, EnvelopeDigest: dg, Signer: Signer{Principal: principal, Fingerprint: "SHA256:x"}, Host: testHost, Now: now}); !errors.Is(err, ErrNonceUnavailable) {
		t.Fatalf("approval reusable after a crash: %v", err)
	}
	// recovery is idempotent
	recs2, err := s.Recover(ctx, nil, now)
	if err != nil || len(recs2) != 2 {
		t.Fatalf("second recovery changed the audit: %v %d", err, len(recs2))
	}
}

func TestCrashAfterStartedRecoversAsUncertainOrEnded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lookup Lookup
		want   Kind
	}{
		{"no answer from systemd", func(context.Context, string) (EndInfo, bool, error) { return EndInfo{}, false, nil }, KindUncertain},
		{"lookup error", func(context.Context, string) (EndInfo, bool, error) { return EndInfo{}, false, errors.New("dbus down") }, KindUncertain},
		{"nil lookup", nil, KindUncertain},
		{"unit finished", func(_ context.Context, inv string) (EndInfo, bool, error) {
			if inv != strings.Repeat("e", 32) {
				t.Errorf("lookup by %q, want the recorded InvocationID", inv)
			}
			return EndInfo{Result: ResultSuccess, OutputSHA256: sha2Hex, OutputBytes: 12, Duration: time.Second}, true, nil
		}, KindEnded},
		{"garbled end info", func(context.Context, string) (EndInfo, bool, error) {
			return EndInfo{Result: ResultSuccess, ExitStatus: 3}, true, nil
		}, KindUncertain},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "state.db")
			now := time.Now()
			id, e := crashRun(t, dbPath, "started", now)
			s, err := openStub(dbPath, DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ctx := context.Background()
			recs, err := s.Recover(ctx, tc.lookup, now)
			if err != nil {
				t.Fatal(err)
			}
			if len(recs) != 3 || recs[1].Kind != KindStarted || recs[2].Kind != tc.want || recs[2].Attempt != id {
				t.Fatalf("recovery wrote %+v", recs)
			}
			if tc.want == KindUncertain && !strings.Contains(recs[2].Note, "check by hand") {
				t.Errorf("UNCERTAIN lacks the by-hand note: %q", recs[2].Note)
			}
			if err := s.CheckAuditChain(ctx, true); err != nil {
				t.Fatal(err)
			}
			dg, _ := e.Digest()
			if _, err := s.Consume(ctx, ConsumeRequest{Env: e, EnvelopeDigest: dg, Signer: Signer{Principal: principal, Fingerprint: "SHA256:x"}, Host: testHost, Now: now}); !errors.Is(err, ErrNonceUnavailable) {
				t.Fatalf("approval reusable after a crash: %v", err)
			}
		})
	}
}

func TestExpiredNonceStaysDeadAcrossRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	now := time.Now()
	s, err := openStub(dbPath, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	n, _ := s.IssueNonce(ctx, testHost, time.Minute, now)
	s.Close()
	s, err = openStub(dbPath, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e := testEnvelope(t, now)
	e.Nonce = n.Value
	dg, _ := e.Digest()
	req := ConsumeRequest{Env: e, EnvelopeDigest: dg, Signer: Signer{Principal: principal, Fingerprint: "SHA256:x"}, Host: testHost, Now: now.Add(2 * time.Minute)}
	if _, err := s.Consume(ctx, req); !errors.Is(err, ErrNonceUnavailable) {
		t.Fatalf("expired nonce after restart: %v", err)
	}
}

func TestNonceIssueBounds(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	if _, err := s.IssueNonce(ctx, "not-hex", time.Minute, time.Now()); err == nil {
		t.Error("bad host accepted")
	}
	if _, err := s.IssueNonce(ctx, testHost, 6*time.Minute, time.Now()); err == nil {
		t.Error("nonce lifetime over the approval TTL accepted")
	}
	if _, err := s.IssueNonce(ctx, testHost, 0, time.Now()); err == nil {
		t.Error("zero lifetime accepted")
	}
	a, _ := s.IssueNonce(ctx, testHost, time.Minute, time.Now())
	b, _ := s.IssueNonce(ctx, testHost, time.Minute, time.Now())
	if a.Value == b.Value || len(a.Value) != 32 {
		t.Error("nonces are not distinct 128-bit values")
	}
}

// keep the encoding/json import honest for table dumps in failures
var _ = json.Marshal

func TestConsumeRefusesRuntimeOutsideCapBeforeSpendingNonce(t *testing.T) {
	now := time.Now()
	for i, v := range []int64{9223372037, 1 << 62, 601, 0, -1} {
		s, _ := newStore(t) // fresh store: an earlier success would hold the privilege cap
		n, err := s.IssueNonce(context.Background(), testHost, 4*time.Minute, now)
		if err != nil {
			t.Fatal(err)
		}
		e := testEnvelope(t, now)
		e.Nonce, e.RuntimeMaxS = n.Value, v
		e.CardID = fmt.Sprintf("card-%d", i) // one active attempt per card
		dg, _ := e.Digest()
		req := ConsumeRequest{Env: e, EnvelopeDigest: dg, Host: testHost, Now: now,
			Signer: Signer{Principal: principal, KeyType: "ED25519-CERT", Fingerprint: "SHA256:test"}}
		if a, err := s.Consume(context.Background(), req); err == nil {
			t.Fatalf("runtime_max_s %d accepted (RuntimeMax %v)", v, a.RuntimeMax)
		}
		e.RuntimeMaxS = 600
		req.Env = e
		req.EnvelopeDigest, _ = e.Digest()
		if a, err := s.Consume(context.Background(), req); err != nil || a.RuntimeMax != 600*time.Second {
			t.Fatalf("value %d: nonce spent by the refusal: %v", v, err)
		}
	}
}
