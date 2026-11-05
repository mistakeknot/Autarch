//go:build unix

package homerun

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	testHost   = "0123456789abcdef0123456789abcdef"
	otherHost  = "fedcba9876543210fedcba9876543210"
	principal  = "approver"
	sha1Hex    = "1111111111111111111111111111111111111111111111111111111111111111"
	sha2Hex    = "2222222222222222222222222222222222222222222222222222222222222222"
	sha3Hex    = "3333333333333333333333333333333333333333333333333333333333333333"
	testAttemp = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
)

// keygen runs ssh-keygen with no stdin and a deadline, in dir.
func keygen(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/ssh-keygen", args...)
	cmd.Dir = dir
	cmd.Stdin = nil
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("ssh-keygen %v: %v\n%s", args, err, out.String())
	}
	return out.Bytes()
}

// signer holds a throwaway CA, a user key and (optionally) a user certificate, all generated in a test
// temp directory. No real key is read or written.
type signerKit struct {
	dir     string
	caPub   string
	key     string // private key path
	cert    string // certificate path (empty for a plain key)
	allowed string
}

func newKit(t *testing.T, serial string, validity string) *signerKit {
	t.Helper()
	d := t.TempDir()
	k := &signerKit{dir: d, caPub: filepath.Join(d, "ca.pub"), key: filepath.Join(d, "user")}
	keygen(t, d, "-q", "-t", "ed25519", "-N", "", "-C", "test-ca", "-f", filepath.Join(d, "ca"))
	keygen(t, d, "-q", "-t", "ed25519", "-N", "", "-C", "test-user", "-f", k.key)
	keygen(t, d, "-q", "-s", filepath.Join(d, "ca"), "-I", "test-cert-id", "-n", principal, "-V", validity, "-z", serial, k.key+".pub")
	k.cert = k.key + "-cert.pub"
	k.allowed = filepath.Join(d, "allowed_signers")
	writeAllowed(t, k.allowed, k.caPub)
	return k
}

func writeAllowed(t *testing.T, path, caPub string) {
	t.Helper()
	b, err := os.ReadFile(caPub)
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(string(b))
	line := principal + " cert-authority,namespaces=\"" + DefaultNamespace + "\" " + f[0] + " " + f[1] + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
}

// sign signs msg in the namespace ns with the kit's certificate (or the plain key when plain is set).
func (k *signerKit) sign(t *testing.T, msg []byte, ns string, plain bool) []byte {
	t.Helper()
	d := t.TempDir()
	mp := filepath.Join(d, "msg")
	if err := os.WriteFile(mp, msg, 0o600); err != nil {
		t.Fatal(err)
	}
	f := k.cert
	if plain {
		f = k.key
	}
	keygen(t, d, "-Y", "sign", "-f", f, "-n", ns, mp)
	b, err := os.ReadFile(mp + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (k *signerKit) verifier() *SigVerifier {
	return &SigVerifier{SSHKeygen: "/usr/bin/ssh-keygen", AllowedSigners: k.allowed, Namespace: DefaultNamespace,
		Principal: principal, RequireCert: true, Timeout: 20 * time.Second}
}

// testEnvelope is a valid check envelope issued at now for testHost.
func testEnvelope(t *testing.T, now time.Time) *Envelope {
	t.Helper()
	dd, err := DepsDigest(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &Envelope{
		V: EnvelopeVersion, NS: DefaultNamespace, Op: OpCheck, Host: testHost, Project: "autarch", CardID: "AUT-1",
		CardRev: "rev-1", ScriptSHA256: sha1Hex, ScriptPath: "/work/run.sh", Deps: []Dep{}, DepsDigest: dd,
		Flags: []string{}, Privilege: PrivMk, RuntimeMaxS: 600, RulingID: "q1", LintSHA256: sha2Hex,
		ReviewSHA256: sha3Hex, Nonce: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", IssuedAt: now.Unix(), ExpiresAt: now.Add(4 * time.Minute).Unix(),
	}
}

func canon(t *testing.T, e *Envelope) []byte {
	t.Helper()
	b, err := e.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(p, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}

// consumeFor issues a nonce, builds a card envelope bound to it and consumes it.
func consumeFor(t *testing.T, s *Store, now time.Time, card, op string) (*Attempt, *Envelope, error) {
	t.Helper()
	n, err := s.IssueNonce(context.Background(), testHost, 4*time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	e := testEnvelope(t, now)
	e.CardID, e.Nonce = card, n.Value
	if op != OpCheck {
		e.Op = op
		a, o := testAttemp, sha3Hex
		e.CheckAttempt, e.CheckOutputSHA = &a, &o
	}
	if err := e.Validate(DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	dg, _ := e.Digest()
	a, err := s.Consume(context.Background(), ConsumeRequest{Env: e, EnvelopeDigest: dg,
		Signer: Signer{Principal: principal, KeyType: "ED25519-CERT", Fingerprint: "SHA256:test"}, Host: testHost, Now: now})
	return a, e, err
}

// recStarted is what a launcher does: record the launch intent, start the unit under the intended name,
// record STARTED.
func recStarted(s *Store, ctx context.Context, attempt string, info StartInfo, at time.Time) error {
	unit, err := s.RecordLaunchIntent(ctx, attempt, at)
	if err != nil {
		return err
	}
	info.Unit = unit
	return s.RecordStarted(ctx, attempt, info, at)
}
