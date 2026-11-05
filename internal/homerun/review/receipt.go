// Package review is the review-receipt layer of the Citadel script-run verifier (bead mk-21nfo, slice 5).
//
// A review receipt says that a review service, holding its own signing key, reviewed one exact script
// and dependency set and reached a verdict. The verifier accepts a run only with a receipt that:
// verifies under the configured review public key, has outcome PASS, names the same script digest,
// deps digest and flags as the operation, carries a dispatch receipt digest, and has not expired.
// There is no fallback: with no valid receipt the run is refused.
//
// What a receipt proves: the holder of the review key signed this verdict over these bytes. It does
// not prove the verdict is right, and it is no stronger than the isolation of the key (see key.go).
//
// This slice has no real model provider and no real key. Providers and signers are injected.
package review

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/mistakeknot/autarch/internal/homerun"
)

// Outcomes.
const (
	OutcomePass = "PASS"
	OutcomeFail = "FAIL"
)

const (
	// Version is the receipt format version.
	Version = 1
	// Namespace domain-separates the signed bytes from every other signature in the system.
	Namespace = "home-review"
	// MaxTTL bounds expires_at - issued_at on both issue and verify.
	MaxTTL = 24 * time.Hour
	// MaxTTLSeconds is MaxTTL in whole seconds, compared as integers.
	MaxTTLSeconds = int64(24 * 60 * 60)
	// maxReceiptBytes bounds what ParseReceipt reads.
	maxReceiptBytes = 64 << 10
	// maxUnix bounds timestamps (year 2255), so integer arithmetic on them cannot overflow.
	maxUnix = int64(9_000_000_000)
)

// ErrReceipt is wrapped by every receipt refusal. Show a requester only that the review was refused.
var ErrReceipt = errors.New("review receipt refused")

func refuse(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrReceipt, fmt.Sprintf(format, a...))
}

var (
	reHex64  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	reHex128 = regexp.MustCompile(`^[0-9a-f]{128}$`)
	reToken  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/+-]{0,127}$`)
	reFlag   = regexp.MustCompile(`^--[a-z][a-z0-9-]{0,31}$`)
	maxFlags = 32
)

// Body is the signed part of a receipt.
type Body struct {
	V               int      `json:"v"`
	NS              string   `json:"ns"`
	ScriptSHA256    string   `json:"script_sha256"`
	DepsDigest      string   `json:"deps_digest"`
	Flags           []string `json:"flags"`
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	DispatchReceipt string   `json:"dispatch_receipt_sha256"`
	Outcome         string   `json:"outcome"`
	IssuedAt        int64    `json:"issued_at"`
	ExpiresAt       int64    `json:"expires_at"`
}

// Receipt is a body and the review key's signature over the body's signing bytes.
type Receipt struct {
	Body
	Signature string `json:"signature"` // lower-case hex ed25519 signature
}

var receiptKeys = []string{"v", "ns", "script_sha256", "deps_digest", "flags", "provider", "model",
	"dispatch_receipt_sha256", "outcome", "issued_at", "expires_at", "signature"}

// validate checks field shape only (no key, no time).
func (b *Body) validate() error {
	switch {
	case b.V != Version:
		return refuse("unsupported version")
	case b.NS != Namespace:
		return refuse("wrong namespace")
	case !reHex64.MatchString(b.ScriptSHA256):
		return refuse("script_sha256 must be 64 lower-case hex characters")
	case !reHex64.MatchString(b.DepsDigest):
		return refuse("deps_digest must be 64 lower-case hex characters")
	case !reHex64.MatchString(b.DispatchReceipt):
		return refuse("dispatch_receipt_sha256 is missing or malformed")
	case !reToken.MatchString(b.Provider):
		return refuse("provider is missing or malformed")
	case !reToken.MatchString(b.Model):
		return refuse("model is missing or malformed")
	case b.Outcome != OutcomePass && b.Outcome != OutcomeFail:
		return refuse("outcome must be PASS or FAIL")
	case b.IssuedAt <= 0 || b.IssuedAt > maxUnix || b.ExpiresAt <= b.IssuedAt || b.ExpiresAt > maxUnix:
		return refuse("issued_at/expires_at are not a bounded, positive, ordered pair")
	case b.ExpiresAt-b.IssuedAt > MaxTTLSeconds: // both are below maxUnix, so this cannot overflow
		return refuse("lifetime exceeds the maximum")
	case len(b.Flags) > maxFlags:
		return refuse("too many flags")
	}
	for _, f := range b.Flags {
		if !reFlag.MatchString(f) {
			return refuse("malformed flag")
		}
	}
	return nil
}

// SigningBytes returns the exact bytes that are signed: the namespace, a NUL, and the RFC 8785 body.
func (b *Body) SigningBytes() ([]byte, error) {
	cp := *b
	if cp.Flags == nil {
		cp.Flags = []string{}
	}
	raw, err := json.Marshal(&cp)
	if err != nil {
		return nil, err
	}
	canon, err := homerun.Canonicalize(raw)
	if err != nil {
		return nil, err
	}
	return append([]byte(Namespace+"\x00"), canon...), nil
}

// Canonical returns the RFC 8785 bytes of the whole receipt.
func (r *Receipt) Canonical() ([]byte, error) {
	cp := *r
	if cp.Flags == nil {
		cp.Flags = []string{}
	}
	raw, err := json.Marshal(&cp)
	if err != nil {
		return nil, err
	}
	return homerun.Canonicalize(raw)
}

// Digest is the lower-case hex sha256 of the canonical receipt: the value an envelope carries as
// review_sha256.
func (r *Receipt) Digest() (string, error) {
	c, err := r.Canonical()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(c)
	return hex.EncodeToString(sum[:]), nil
}

// ParseReceipt decodes canonical receipt bytes. It refuses unknown fields, trailing data and any bytes
// that are not already the canonical encoding.
func ParseReceipt(raw []byte) (*Receipt, error) {
	if len(raw) == 0 || len(raw) > maxReceiptBytes {
		return nil, refuse("size out of range")
	}
	canon, err := homerun.Canonicalize(raw)
	if err != nil {
		return nil, refuse("not canonical JSON")
	}
	if !bytes.Equal(canon, raw) {
		return nil, refuse("not the canonical encoding")
	}
	// Go's decoder matches keys case-insensitively, so check the exact key set first.
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, refuse("malformed receipt")
	}
	if len(keys) != len(receiptKeys) {
		return nil, refuse("receipt keys are not the exact schema")
	}
	for _, k := range receiptKeys {
		if _, ok := keys[k]; !ok {
			return nil, refuse("receipt keys are not the exact schema")
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var r Receipt
	if err := dec.Decode(&r); err != nil {
		return nil, refuse("malformed receipt")
	}
	if err := r.Body.validate(); err != nil {
		return nil, err
	}
	if !reHex128.MatchString(r.Signature) {
		return nil, refuse("signature is malformed (need 128 lower-case hex characters)")
	}
	// The parsed value must re-encode to exactly the bytes that were read.
	again, err := r.Canonical()
	if err != nil || !bytes.Equal(again, raw) {
		return nil, refuse("receipt does not round-trip to its bytes")
	}
	return &r, nil
}

// Expect is what the verifier requires a receipt to match.
type Expect struct {
	ScriptSHA256 string
	DepsDigest   string
	Flags        []string
	Now          time.Time
}

// Verifier checks receipts against one configured review public key.
type Verifier struct {
	PublicKey ed25519.PublicKey
}

// Verify accepts a receipt only if every condition holds; the first failure refuses. rawReceipt must be
// the canonical bytes. It returns the receipt digest (the envelope's review_sha256) on success.
func (v *Verifier) Verify(rawReceipt []byte, want Expect) (string, error) {
	dg, _, err := v.verify(rawReceipt, want)
	return dg, err
}

// verify is Verify that also returns the receipt's expiry (unix seconds).
func (v *Verifier) verify(rawReceipt []byte, want Expect) (string, int64, error) {
	if len(v.PublicKey) != ed25519.PublicKeySize {
		return "", 0, errors.New("review verifier is not configured (public key)")
	}
	if len(rawReceipt) == 0 {
		return "", 0, refuse("no review receipt")
	}
	r, err := ParseReceipt(rawReceipt)
	if err != nil {
		return "", 0, err
	}
	sig, err := hex.DecodeString(r.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize || !reHex128.MatchString(r.Signature) {
		return "", 0, refuse("signature is malformed")
	}
	msg, err := r.Body.SigningBytes()
	if err != nil {
		return "", 0, refuse("cannot encode receipt")
	}
	if !ed25519.Verify(v.PublicKey, msg, sig) {
		return "", 0, refuse("signature does not verify under the review key")
	}
	if r.Outcome != OutcomePass {
		return "", 0, refuse("review outcome is not PASS")
	}
	if !reHex64.MatchString(want.ScriptSHA256) || !reHex64.MatchString(want.DepsDigest) {
		return "", 0, errors.New("expected script or deps digest is malformed")
	}
	if r.ScriptSHA256 != want.ScriptSHA256 {
		return "", 0, refuse("receipt is for a different script")
	}
	if r.DepsDigest != want.DepsDigest {
		return "", 0, refuse("receipt is for different dependencies")
	}
	wf := want.Flags
	if wf == nil {
		wf = []string{}
	}
	gf := r.Flags
	if gf == nil {
		gf = []string{}
	}
	if !slices.Equal(gf, wf) {
		return "", 0, refuse("receipt is for different flags")
	}
	now := want.Now.Unix()
	if now < r.IssuedAt {
		return "", 0, refuse("receipt is not yet valid")
	}
	if now >= r.ExpiresAt {
		return "", 0, refuse("receipt has expired")
	}
	dg, err := r.Digest()
	if err != nil {
		return "", 0, err
	}
	return dg, r.ExpiresAt, nil
}

// Gate adapts the verifier for the run verifier's review gate: receipts are read from src (a
// content-addressed store keyed by the receipt digest) and verified under v's public key.
func (v *Verifier) Gate(src homerun.ReceiptSource) *homerun.ReviewGate {
	return &homerun.ReviewGate{
		Receipts: src,
		Verify: func(raw []byte, scriptSHA, depsDigest string, flags []string, now time.Time) (string, int64, error) {
			return v.verify(raw, Expect{ScriptSHA256: scriptSHA, DepsDigest: depsDigest, Flags: flags, Now: now})
		},
	}
}
