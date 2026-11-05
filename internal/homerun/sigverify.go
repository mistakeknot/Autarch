//go:build unix

package homerun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// ErrSignature is wrapped by every approval-signature failure. The reason after it is for the audit log;
// callers should show a requester only that the signature was refused.
var ErrSignature = errors.New("approval signature refused")

const (
	maxSignedBytes = 1 << 20
	maxSigBytes    = 16 << 10
)

// SigVerifier checks an approval signature by running "ssh-keygen -Y verify" against an allowed-signers
// file. Every path is a parameter; the deployment pins them to root-owned copies. Nothing is searched on
// PATH and the subprocess gets an empty environment.
type SigVerifier struct {
	// SSHKeygen is the absolute path of the ssh-keygen binary to run.
	SSHKeygen string
	// AllowedSigners is the allowed_signers file (a cert-authority entry limited to Principal and Namespace).
	AllowedSigners string
	// RevocationFile is an ssh-keygen revocation list (KRL or key list). Empty means none is configured.
	RevocationFile string
	// Namespace is the SSH-signature namespace to require (normally Limits.Namespace).
	Namespace string
	// Principal is the signer identity to require (ssh-keygen -I).
	Principal string
	// RequireCert refuses a signature made with a plain key: an approval must come from a certificate.
	RequireCert bool
	// Timeout bounds one ssh-keygen run; zero means 10 seconds.
	Timeout time.Duration
}

// Signer identifies who approved, for the audit trail.
type Signer struct {
	Principal   string `json:"principal"`
	KeyType     string `json:"key_type"`
	Fingerprint string `json:"fingerprint"`
	CertKeyID   string `json:"cert_key_id,omitempty"`
	CertSerial  uint64 `json:"cert_serial,omitempty"`
}

var reGood = regexp.MustCompile(`^Good "([^"]*)" signature for (\S+) with (\S+) key (\S+)$`)

func sigErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrSignature, fmt.Sprintf(format, a...))
}

// Verify checks that sig is a valid signature over msg, made in v.Namespace by v.Principal under the
// allowed signers, with a certificate (when required) that is valid at the given time and not revoked.
func (v *SigVerifier) Verify(ctx context.Context, msg, sig []byte, at time.Time) (*Signer, error) {
	if !filepath.IsAbs(v.SSHKeygen) || v.AllowedSigners == "" || v.Namespace == "" || v.Principal == "" {
		return nil, errors.New("signature verifier is not configured (ssh-keygen path, allowed signers, namespace and principal are required)")
	}
	if len(msg) == 0 || len(msg) > maxSignedBytes {
		return nil, sigErr("signed message size out of range")
	}
	if len(sig) == 0 || len(sig) > maxSigBytes {
		return nil, sigErr("signature size out of range")
	}
	if strings.ContainsAny(v.Principal+v.Namespace, " \t\r\n\x00") || strings.HasPrefix(v.Principal, "-") || strings.HasPrefix(v.Namespace, "-") {
		return nil, errors.New("signature verifier principal or namespace is malformed")
	}
	info, err := parseSSHSig(sig)
	if err != nil {
		return nil, sigErr("%v", err)
	}
	if info.Namespace != v.Namespace {
		return nil, sigErr("signature is in namespace %q, want %q", info.Namespace, v.Namespace)
	}
	if v.RequireCert && info.Cert == nil {
		return nil, sigErr("signing key is not a certificate")
	}
	if c := info.Cert; c != nil {
		if !slices.Contains(c.Principals, v.Principal) {
			return nil, sigErr("certificate is not issued to principal %q", v.Principal)
		}
		if !c.validAt(at) {
			return nil, sigErr("certificate is outside its validity window")
		}
	}

	dir, err := os.MkdirTemp("", "homerun-sig-")
	if err != nil {
		return nil, fmt.Errorf("signature verifier: %w", err)
	}
	defer os.RemoveAll(dir)
	sigPath := filepath.Join(dir, "msg.sig")
	if err := os.WriteFile(sigPath, sig, 0o600); err != nil {
		return nil, fmt.Errorf("signature verifier: %w", err)
	}

	timeout := v.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"-Y", "verify", "-f", v.AllowedSigners, "-I", v.Principal, "-n", v.Namespace, "-s", sigPath}
	if v.RevocationFile != "" {
		args = append(args, "-r", v.RevocationFile)
	}
	args = append(args, "-Overify-time="+at.UTC().Format("20060102150405")+"Z")
	cmd := exec.CommandContext(cctx, v.SSHKeygen, args...)
	cmd.Env = []string{"LC_ALL=C"}
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(msg)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if cctx.Err() != nil {
			return nil, sigErr("verification timed out")
		}
		return nil, sigErr("ssh-keygen: %s", firstLine(errb.String()+out.String()))
	}
	m := reGood.FindStringSubmatch(strings.TrimSpace(out.String()))
	if m == nil || m[1] != v.Namespace || m[2] != v.Principal {
		return nil, sigErr("unexpected ssh-keygen output")
	}
	s := &Signer{Principal: m[2], KeyType: m[3], Fingerprint: m[4]}
	if info.Cert != nil {
		s.CertKeyID, s.CertSerial = info.Cert.KeyID, info.Cert.Serial
	}
	return s, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// VerifyApproval is the whole approval check for one request, in order: the envelope bytes must parse as
// canonical, validate under lim, be fresh and for this host; then the signature must verify over exactly
// those bytes. It returns the parsed envelope, its digest and the signer. It does not consume the nonce.
func VerifyApproval(ctx context.Context, v *SigVerifier, lim Limits, machineID string, envBytes, sig []byte, now time.Time) (*Envelope, string, *Signer, error) {
	env, err := ParseEnvelope(envBytes)
	if err != nil {
		return nil, "", nil, err
	}
	if err := env.Validate(lim); err != nil {
		return nil, "", nil, err
	}
	if err := env.CheckFresh(now); err != nil {
		return nil, "", nil, err
	}
	if err := env.CheckHost(machineID); err != nil {
		return nil, "", nil, err
	}
	signer, err := v.Verify(ctx, envBytes, sig, now)
	if err != nil {
		return nil, "", nil, err
	}
	digest, err := env.Digest()
	if err != nil {
		return nil, "", nil, err
	}
	return env, digest, signer, nil
}
