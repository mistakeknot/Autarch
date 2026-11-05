//go:build unix

package homerun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Review round 1 of the verifier core (mk-21nfo): one test per finding.

// Finding 1: Go's decoder folds case and Unicode look-alikes (U+017F folds to "s"), so an alias key can
// stand in for a schema key and override it. The parser must accept the exact schema key set only.
func TestParseEnvelopeRefusesAliasKey(t *testing.T) {
	now := time.Now()
	good := canon(t, testEnvelope(t, now))
	if !bytes.Contains(good, []byte(`"check_attempt":null`)) {
		t.Fatal("fixture drifted: no check_attempt member to replace")
	}
	for name, repl := range map[string]string{
		"long-s alias":     `"ſcript_sha256":"` + sha2Hex + `"`,
		"upper-case alias": `"SCRIPT_SHA256":"` + sha2Hex + `"`,
		"unknown name":     `"bogus":"x"`,
	} {
		forged := bytes.Replace(good, []byte(`"check_attempt":null`), []byte(repl), 1)
		c, err := Canonicalize(forged)
		if err != nil {
			t.Fatalf("%s: forged bytes do not canonicalize: %v", name, err)
		}
		if env, err := ParseEnvelope(c); err == nil {
			t.Fatalf("%s: parsed, script digest %s", name, env.ScriptSHA256)
		}
	}
}

// Finding 1: the digest is the hash of the bytes that were signed.
func TestVerifyApprovalDigestIsSignedBytes(t *testing.T) {
	now := time.Now()
	k := newKit(t, "1", "-5m:+1h")
	b := canon(t, testEnvelope(t, now))
	sig := k.sign(t, b, DefaultNamespace, false)
	_, dg, _, err := VerifyApproval(context.Background(), k.verifier(), DefaultLimits(), testHost, b, sig, now)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b)
	if dg != hex.EncodeToString(h[:]) {
		t.Fatalf("digest %s is not sha256 of the signed bytes", dg)
	}
}

// Finding 3: a complete record that lost only its final newline is kept and terminated.
func TestMirrorKeepsCompleteRecordMissingNewline(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	mp := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := s.SetMirror(ctx, mp); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	a, _, err := consumeFor(t, s, now, "AUT-1", OpCheck)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAborted(ctx, a.ID, "x", now); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(mp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mp, bytes.TrimSuffix(b, []byte("\n")), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := s.syncMirror(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := ReadMirror(mp)
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.Records(ctx)
	if len(got) != len(db) || len(db) != 2 {
		t.Fatalf("mirror has %d records, database %d", len(got), len(db))
	}
	if b2, _ := os.ReadFile(mp); !bytes.Equal(b2, b) {
		t.Fatalf("mirror not restored byte for byte:\n%q\n%q", b2, b)
	}
	// and a genuinely partial tail is still cut off
	if err := os.WriteFile(mp, append(append([]byte{}, b[:bytes.IndexByte(b, '\n')+1]...), []byte(`{"seq":2,"att`)...), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := s.syncMirror(ctx); err != nil {
		t.Fatal(err)
	}
	if b3, _ := os.ReadFile(mp); !bytes.Equal(b3, b) {
		t.Fatalf("partial tail not repaired:\n%q", b3)
	}
}

// Finding 4: seconds are compared as integers before any Duration is built.
func TestApprovalTTLOverflowRefused(t *testing.T) {
	now := time.Now()
	lim := DefaultLimits()
	for _, span := range []int64{18446744074, 9223372037, 9223372036854775807 - now.Unix()} {
		e := testEnvelope(t, now)
		e.ExpiresAt = e.IssuedAt + span
		if err := e.Validate(lim); err == nil {
			t.Fatalf("span %d s accepted", span)
		}
	}
	e := testEnvelope(t, now)
	e.ExpiresAt = e.IssuedAt + int64(lim.ApprovalTTL/time.Second)
	if err := e.Validate(lim); err != nil {
		t.Fatalf("span of exactly the TTL refused: %v", err)
	}
	e.ExpiresAt++
	if err := e.Validate(lim); err == nil {
		t.Fatal("span of TTL+1s accepted")
	}
	_ = strconv.Itoa
	_ = strings.TrimSpace
}

// Finding 2: the verifier dies after the unit is launched and before STARTED is recorded. Recovery must
// look for the unit it intended; it must not call the attempt "never started" while the unit runs.
func TestRecoverReconcilesLaunchIntent(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	inv := strings.Repeat("c", 32)
	type tc struct {
		name   string
		state  UnitState
		err    error
		noByU  bool
		want   Kind
		wantEr bool
	}
	endInfo := &EndInfo{Result: ResultSuccess, OutputSHA256: sha3Hex, OutputBytes: 3, Duration: time.Second, EndedAt: now}
	for _, c := range []tc{
		{name: "unit running", state: UnitState{Found: true, InvocationID: inv}, want: KindStarted},
		{name: "unit already ended", state: UnitState{Found: true, InvocationID: inv, Ended: endInfo}, want: KindEnded},
		{name: "systemd has no such unit", state: UnitState{}, want: KindUncertain},
		{name: "lookup fails", err: context.DeadlineExceeded, want: KindAuthorized, wantEr: true},
		{name: "no unit lookup supplied", noByU: true, want: KindAuthorized, wantEr: true},
		{name: "unit with a malformed invocation id", state: UnitState{Found: true, InvocationID: "x"}, want: KindAuthorized, wantEr: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newStore(t)
			a, _, err := consumeFor(t, s, now, "AUT-1", OpCheck)
			if err != nil {
				t.Fatal(err)
			}
			unit, err := s.RecordLaunchIntent(ctx, a.ID, now)
			if err != nil || unit != UnitName(a.ID) {
				t.Fatalf("intent: %q %v", unit, err)
			}
			if u2, err := s.RecordLaunchIntent(ctx, a.ID, now); err != nil || u2 != unit {
				t.Fatalf("intent is not idempotent: %q %v", u2, err)
			}
			// the launcher started the unit; the verifier dies here. A restart:
			var asked []string
			byU := func(_ context.Context, u string) (UnitState, error) { asked = append(asked, u); return c.state, c.err }
			if c.noByU {
				byU = nil
			}
			noInv := func(context.Context, string) (EndInfo, bool, error) { return EndInfo{}, false, nil }
			_, rerr := s.RecoverUnits(ctx, noInv, byU, now)
			if (rerr != nil) != c.wantEr {
				t.Fatalf("recover error %v, want error=%v", rerr, c.wantEr)
			}
			if !c.noByU && (len(asked) != 1 || asked[0] != unit) {
				t.Fatalf("recovery asked about %v, want [%s]", asked, unit)
			}
			recs, _ := s.Records(ctx)
			if got := recs[len(recs)-1].Kind; got != c.want {
				t.Fatalf("attempt is %s, want %s", got, c.want)
			}
			if c.want == KindAuthorized || c.want == KindStarted || c.want == KindUncertain {
				// still held: the card stays held and a second attempt is refused
				if _, _, err := consumeFor(t, s, now, "AUT-1", OpCheck); !errors.Is(err, ErrBusy) {
					t.Fatalf("card released while the unit may be running: %v", err)
				}
			}
			if err := s.CheckAuditChain(ctx, false); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Finding 2: once intent is recorded the attempt cannot be declared "never launched" by a caller, and
// STARTED must name the intended unit.
func TestLaunchIntentGuardsAbortAndStart(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	s, _ := newStore(t)
	a, _, err := consumeFor(t, s, now, "AUT-1", OpCheck)
	if err != nil {
		t.Fatal(err)
	}
	info := StartInfo{Unit: UnitName(a.ID), InvocationID: strings.Repeat("e", 32), CASSHA: a.ScriptSHA256}
	if err := s.RecordStarted(ctx, a.ID, info, now); err == nil {
		t.Fatal("STARTED recorded for a unit that was never announced")
	}
	if _, err := s.RecordLaunchIntent(ctx, a.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAborted(ctx, a.ID, "x", now); err == nil {
		t.Fatal("ABORTED_BEFORE_START recorded after a launch was intended")
	}
	bad := info
	bad.Unit = "some-other.service"
	if err := s.RecordStarted(ctx, a.ID, bad, now); err == nil {
		t.Fatal("STARTED accepted for a unit other than the intended one")
	}
	if err := s.RecordStarted(ctx, a.ID, info, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordLaunchIntent(ctx, a.ID, now); err == nil {
		t.Fatal("intent recorded for an attempt that already started")
	}
}
