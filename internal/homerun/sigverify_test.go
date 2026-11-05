//go:build unix

package homerun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// T2.2 — signature, namespace, certificate and freshness checks.

func TestVerifyApprovalGood(t *testing.T) {
	now := time.Now()
	k := newKit(t, "7", "-1h:+1h")
	e := testEnvelope(t, now)
	b := canon(t, e)
	sig := k.sign(t, b, DefaultNamespace, false)
	env, dg, s, err := VerifyApproval(context.Background(), k.verifier(), DefaultLimits(), testHost, b, sig, now)
	if err != nil {
		t.Fatal(err)
	}
	if env.CardID != e.CardID || s.Principal != principal || s.CertSerial != 7 || s.CertKeyID != "test-cert-id" || !strings.HasPrefix(s.Fingerprint, "SHA256:") {
		t.Fatalf("unexpected result %+v %+v", env, s)
	}
	if want, _ := e.Digest(); dg != want {
		t.Fatal("digest mismatch")
	}
}

func TestSignatureCoversEveryField(t *testing.T) {
	now := time.Now()
	k := newKit(t, "7", "-1h:+1h")
	base := testEnvelope(t, now)
	oldSig := k.sign(t, canon(t, base), DefaultNamespace, false)
	ctx := context.Background()
	other := "c" + strings.Repeat("0", 63)
	str := func(s string) *string { return &s }

	type mut struct {
		name string
		f    func(*Envelope)
		// wantEnvelopeErr: the mutated envelope is itself refused before the signature is looked at.
		wantEnvelopeErr bool
	}
	muts := []mut{
		{"v", func(e *Envelope) { e.V = 2 }, true},
		{"ns", func(e *Envelope) { e.NS = "git" }, true},
		{"op", func(e *Envelope) { e.Op = OpApply; e.CheckAttempt = str(testAttemp); e.CheckOutputSHA = str(sha3Hex) }, false},
		{"host", func(e *Envelope) { e.Host = otherHost }, true},
		{"project", func(e *Envelope) { e.Project = "other" }, false},
		{"card_id", func(e *Envelope) { e.CardID = "AUT-2" }, false},
		{"card_rev", func(e *Envelope) { e.CardRev = "rev-2" }, false},
		{"script_sha256", func(e *Envelope) { e.ScriptSHA256 = other }, false},
		{"script_path", func(e *Envelope) { e.ScriptPath = "/work/other.sh" }, false},
		{"deps", func(e *Envelope) {
			e.Deps = []Dep{{"/work/lib.sh", other}}
			e.DepsDigest, _ = DepsDigest(e.Deps)
		}, false},
		{"deps_digest", func(e *Envelope) { e.DepsDigest = other }, true},
		{"flags", func(e *Envelope) { e.Flags = []string{"--dry-run"} }, false},
		{"privilege", func(e *Envelope) { e.Privilege = PrivRoot }, true}, // root disabled
		{"runtime_max_s", func(e *Envelope) { e.RuntimeMaxS = 599 }, false},
		{"ruling_id", func(e *Envelope) { e.RulingID = "q2" }, false},
		{"lint_sha256", func(e *Envelope) { e.LintSHA256 = other }, false},
		{"review_sha256", func(e *Envelope) { e.ReviewSHA256 = other }, false},
		{"check_attempt", func(e *Envelope) { e.CheckAttempt = str(testAttemp) }, true},
		{"check_output_sha256", func(e *Envelope) { e.CheckOutputSHA = str(sha3Hex) }, true},
		{"nonce", func(e *Envelope) { e.Nonce = strings.Repeat("b", 32) }, false},
		{"issued_at", func(e *Envelope) { e.IssuedAt-- }, false},
		{"expires_at", func(e *Envelope) { e.ExpiresAt++ }, false},
	}
	if got := len(envelopeKeys()); got != len(muts) {
		// every envelope field has a mutation (op also covers check_*; the table has one row per field)
		t.Fatalf("envelope has %d fields, mutation table has %d", got, len(muts))
	}
	for _, m := range muts {
		t.Run(m.name, func(t *testing.T) {
			e := *base
			e.Deps, e.Flags = append([]Dep{}, base.Deps...), append([]string{}, base.Flags...)
			m.f(&e)
			b := canon(t, &e)
			if bytes := canon(t, base); string(bytes) == string(b) {
				t.Fatal("mutation changed nothing")
			}
			// the old signature never verifies over changed bytes
			if _, err := k.verifier().Verify(ctx, b, oldSig, now); !errors.Is(err, ErrSignature) {
				t.Fatalf("old signature accepted a changed %s: %v", m.name, err)
			}
			_, _, _, err := VerifyApproval(ctx, k.verifier(), DefaultLimits(), testHost, b, oldSig, now)
			if err == nil {
				t.Fatalf("changed %s accepted", m.name)
			}
			if m.wantEnvelopeErr != errors.Is(err, ErrEnvelope) || (!m.wantEnvelopeErr && !errors.Is(err, ErrSignature)) {
				t.Fatalf("changed %s: wrong refusal %v", m.name, err)
			}
		})
	}
}

func TestSignatureNamespaceAndKeyKind(t *testing.T) {
	now := time.Now()
	k := newKit(t, "7", "-1h:+1h")
	b := canon(t, testEnvelope(t, now))
	ctx := context.Background()
	for _, ns := range []string{"git", "file", "home-run@other", "home-run@local2", "Home-Run@local"} {
		if _, err := k.verifier().Verify(ctx, b, k.sign(t, b, ns, false), now); !errors.Is(err, ErrSignature) {
			t.Errorf("signature in namespace %q accepted: %v", ns, err)
		}
	}
	// a plain (non-certificate) key signing in the right namespace is refused when a cert is required
	if _, err := k.verifier().Verify(ctx, b, k.sign(t, b, DefaultNamespace, true), now); !errors.Is(err, ErrSignature) {
		t.Errorf("plain key accepted: %v", err)
	}
	// tampered signature bytes
	sig := k.sign(t, b, DefaultNamespace, false)
	lines := strings.Split(string(sig), "\n")
	// flip one base64 character in the middle of the last line (the signature itself)
	last := lines[len(lines)-3]
	flip := byte('A')
	if last[len(last)/2] == 'A' {
		flip = 'B'
	}
	lines[len(lines)-3] = last[:len(last)/2] + string(flip) + last[len(last)/2+1:]
	if _, err := k.verifier().Verify(ctx, b, []byte(strings.Join(lines, "\n")), now); !errors.Is(err, ErrSignature) {
		t.Errorf("tampered signature accepted: %v", err)
	}
	if _, err := k.verifier().Verify(ctx, b, []byte("not a signature"), now); !errors.Is(err, ErrSignature) {
		t.Errorf("garbage accepted: %v", err)
	}
}

func TestSignatureCertificateWindow(t *testing.T) {
	now := time.Now()
	k := newKit(t, "7", "-1h:+1h")
	b := canon(t, testEnvelope(t, now))
	sig := k.sign(t, b, DefaultNamespace, false)
	ctx := context.Background()
	if _, err := k.verifier().Verify(ctx, b, sig, now); err != nil {
		t.Fatal(err)
	}
	if _, err := k.verifier().Verify(ctx, b, sig, now.Add(2*time.Hour)); !errors.Is(err, ErrSignature) {
		t.Errorf("expired certificate accepted: %v", err)
	}
	if _, err := k.verifier().Verify(ctx, b, sig, now.Add(-2*time.Hour)); !errors.Is(err, ErrSignature) {
		t.Errorf("not-yet-valid certificate accepted: %v", err)
	}
}

func TestSignatureRevokedCertificate(t *testing.T) {
	now := time.Now()
	k := newKit(t, "7", "-1h:+1h")
	b := canon(t, testEnvelope(t, now))
	sig := k.sign(t, b, DefaultNamespace, false)
	d := t.TempDir()
	spec := filepath.Join(d, "spec")
	if err := os.WriteFile(spec, []byte("serial: 7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	krl := filepath.Join(d, "revoked.krl")
	keygen(t, d, "-k", "-f", krl, "-s", k.caPub, spec)
	v := k.verifier()
	v.RevocationFile = krl
	if _, err := v.Verify(context.Background(), b, sig, now); !errors.Is(err, ErrSignature) {
		t.Fatalf("revoked certificate accepted: %v", err)
	}
	// a KRL that revokes another serial leaves this one valid
	if err := os.WriteFile(spec, []byte("serial: 8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	krl2 := filepath.Join(d, "other.krl")
	keygen(t, d, "-k", "-f", krl2, "-s", k.caPub, spec)
	v.RevocationFile = krl2
	if _, err := v.Verify(context.Background(), b, sig, now); err != nil {
		t.Fatalf("unrelated revocation refused a good signature: %v", err)
	}
}

func TestSignatureWrongSignerAndUnknownCA(t *testing.T) {
	now := time.Now()
	k := newKit(t, "7", "-1h:+1h")
	b := canon(t, testEnvelope(t, now))
	sig := k.sign(t, b, DefaultNamespace, false)
	ctx := context.Background()

	v := k.verifier()
	v.Principal = "someone-else"
	if _, err := v.Verify(ctx, b, sig, now); !errors.Is(err, ErrSignature) {
		t.Errorf("certificate for another principal accepted: %v", err)
	}
	// a certificate from a CA that the allowed signers do not list
	rogue := newKit(t, "9", "-1h:+1h")
	rsig := rogue.sign(t, b, DefaultNamespace, false)
	if _, err := k.verifier().Verify(ctx, b, rsig, now); !errors.Is(err, ErrSignature) {
		t.Errorf("unknown CA accepted: %v", err)
	}
	// the right CA's certificate but the allowed-signers entry limited to another namespace
	d := t.TempDir()
	lim := filepath.Join(d, "allowed")
	cb, _ := os.ReadFile(k.caPub)
	f := strings.Fields(string(cb))
	os.WriteFile(lim, []byte(principal+" cert-authority,namespaces=\"git\" "+f[0]+" "+f[1]+"\n"), 0o600)
	v = k.verifier()
	v.AllowedSigners = lim
	if _, err := v.Verify(ctx, b, sig, now); !errors.Is(err, ErrSignature) {
		t.Errorf("namespace-limited allowed signers accepted: %v", err)
	}
}

func TestApprovalFreshnessAndHost(t *testing.T) {
	now := time.Now()
	k := newKit(t, "7", "-1h:+1h")
	e := testEnvelope(t, now)
	b := canon(t, e)
	sig := k.sign(t, b, DefaultNamespace, false)
	ctx := context.Background()
	lim := DefaultLimits()
	// expired approval (TTL 4 minutes)
	if _, _, _, err := VerifyApproval(ctx, k.verifier(), lim, testHost, b, sig, now.Add(5*time.Minute)); !errors.Is(err, ErrEnvelope) {
		t.Errorf("expired approval accepted: %v", err)
	}
	// approval from the future
	if _, _, _, err := VerifyApproval(ctx, k.verifier(), lim, testHost, b, sig, now.Add(-time.Minute)); !errors.Is(err, ErrEnvelope) {
		t.Errorf("future approval accepted: %v", err)
	}
	// envelope signed for another host, presented on this one
	if _, _, _, err := VerifyApproval(ctx, k.verifier(), lim, otherHost, b, sig, now); !errors.Is(err, ErrEnvelope) {
		t.Errorf("envelope for another host accepted: %v", err)
	}
	// a lifetime over the TTL is refused even though correctly signed
	long := testEnvelope(t, now)
	long.ExpiresAt = now.Add(6 * time.Minute).Unix()
	lb := canon(t, long)
	if _, _, _, err := VerifyApproval(ctx, k.verifier(), lim, testHost, lb, k.sign(t, lb, DefaultNamespace, false), now); !errors.Is(err, ErrEnvelope) {
		t.Errorf("over-long approval accepted: %v", err)
	}
	// non-canonical bytes are refused even with a signature over exactly those bytes
	pretty := []byte(strings.Replace(string(b), ",", ", ", 1))
	if _, _, _, err := VerifyApproval(ctx, k.verifier(), lim, testHost, pretty, k.sign(t, pretty, DefaultNamespace, false), now); !errors.Is(err, ErrEnvelope) {
		t.Errorf("non-canonical envelope accepted: %v", err)
	}
}

func TestVerifierMisconfiguredFailsClosed(t *testing.T) {
	now := time.Now()
	k := newKit(t, "7", "-1h:+1h")
	b := canon(t, testEnvelope(t, now))
	sig := k.sign(t, b, DefaultNamespace, false)
	v := k.verifier()
	v.SSHKeygen = "ssh-keygen" // not absolute: never searched on PATH
	if _, err := v.Verify(context.Background(), b, sig, now); err == nil {
		t.Error("relative ssh-keygen path accepted")
	}
	v = k.verifier()
	v.AllowedSigners = filepath.Join(t.TempDir(), "missing")
	if _, err := v.Verify(context.Background(), b, sig, now); !errors.Is(err, ErrSignature) {
		t.Errorf("missing allowed signers accepted: %v", err)
	}
	v = k.verifier()
	v.Principal = ""
	if _, err := v.Verify(context.Background(), b, sig, now); err == nil {
		t.Error("empty principal accepted")
	}
}
