package review

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/homerun"
)

// DepInput is one dependency as the service sees it: the path is informational, the bytes are reviewed.
type DepInput struct {
	Path    string
	Content []byte
}

// Request is what the service reviews.
type Request struct {
	Script []byte
	Deps   []DepInput // sorted by path, no duplicates
	Flags  []string
}

// ProviderResult is a reviewer's answer. DispatchReceiptSHA256 identifies the dispatch that produced
// it; a result without one is refused.
type ProviderResult struct {
	Provider              string
	Model                 string
	DispatchReceiptSHA256 string
	Text                  string
}

// ReviewProvider performs the model review. It is an interface so that no model call is wired in.
// The real provider is a later slice.
type ReviewProvider interface {
	Review(ctx context.Context, req Request) (ProviderResult, error)
}

// Signer signs receipt bytes. Implementations hold the key; nothing here ever reads or logs it.
type Signer interface {
	Sign(msg []byte) ([]byte, error)
	PublicKey() ed25519.PublicKey
}

// Service turns a request into a signed receipt. A provider or signer error, or a malformed
// verdict, yields no receipt at all.
type Service struct {
	Provider ReviewProvider
	Signer   Signer
	TTL      time.Duration    // zero means 15 minutes; capped at MaxTTL
	Now      func() time.Time // nil means time.Now
	// MaxScriptBytes and MaxDepBytes bound input; zero means 1 MiB and 4 MiB in total.
	MaxScriptBytes int
	MaxDepBytes    int
}

var reVerdict = regexp.MustCompile(`(?m)^VERDICT: (PASS|FAIL)[ \t]*\r?$`)

// ParseVerdict extracts the verdict from reviewer text. Exactly one "VERDICT: PASS" or
// "VERDICT: FAIL" line must be present; none, two, or a mix of the two is an error, so a script that
// echoes a verdict line into the reviewer's output cannot choose the outcome by ambiguity.
func ParseVerdict(text string) (string, error) {
	m := reVerdict.FindAllStringSubmatch(text, -1)
	if len(m) != 1 {
		return "", fmt.Errorf("reviewer output holds %d verdict lines, want exactly 1", len(m))
	}
	return m[0][1], nil
}

// DepsDigestOf computes the deps digest for request deps, refusing unsorted or duplicate paths.
func DepsDigestOf(deps []DepInput) (string, error) {
	hd := make([]homerun.Dep, len(deps))
	for i, d := range deps {
		sum := sha256.Sum256(d.Content)
		hd[i] = homerun.Dep{Path: d.Path, SHA256: hex.EncodeToString(sum[:])}
		if i > 0 && !(deps[i-1].Path < d.Path) {
			return "", errors.New("deps must be sorted by path with no duplicates")
		}
	}
	return homerun.DepsDigest(hd)
}

// Issue reviews the request and returns canonical signed receipt bytes. Any failure returns no bytes.
// A FAIL verdict yields a FAIL receipt (the verifier refuses it); it is never silently dropped.
func (s *Service) Issue(ctx context.Context, req Request) ([]byte, error) {
	if s.Provider == nil || s.Signer == nil {
		return nil, errors.New("review service is not configured")
	}
	maxS, maxD := s.MaxScriptBytes, s.MaxDepBytes
	if maxS <= 0 {
		maxS = 1 << 20
	}
	if maxD <= 0 {
		maxD = 4 << 20
	}
	if len(req.Script) == 0 || len(req.Script) > maxS {
		return nil, errors.New("script size out of range")
	}
	total := 0
	for _, d := range req.Deps {
		total += len(d.Content)
	}
	if total > maxD {
		return nil, errors.New("dependency size out of range")
	}
	depsDigest, err := DepsDigestOf(req.Deps)
	if err != nil {
		return nil, err
	}
	flags := slices.Clone(req.Flags)
	if flags == nil {
		flags = []string{}
	}
	res, err := s.Provider.Review(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("review provider failed: %w", err)
	}
	outcome, err := ParseVerdict(res.Text)
	if err != nil {
		return nil, err
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	ttl := s.TTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if ttl > MaxTTL {
		ttl = MaxTTL
	}
	sum := sha256.Sum256(req.Script)
	issued := now().Unix()
	body := Body{
		V: Version, NS: Namespace,
		ScriptSHA256: hex.EncodeToString(sum[:]), DepsDigest: depsDigest, Flags: flags,
		Provider: strings.TrimSpace(res.Provider), Model: strings.TrimSpace(res.Model),
		DispatchReceipt: res.DispatchReceiptSHA256,
		Outcome:         outcome, IssuedAt: issued, ExpiresAt: issued + int64(ttl/time.Second),
	}
	if err := body.validate(); err != nil {
		return nil, err
	}
	msg, err := body.SigningBytes()
	if err != nil {
		return nil, err
	}
	sig, err := s.Signer.Sign(msg)
	if err != nil {
		return nil, errors.New("signing failed")
	}
	if len(sig) != ed25519.SignatureSize || !ed25519.Verify(s.Signer.PublicKey(), msg, sig) {
		return nil, errors.New("signer returned a signature that does not verify under its own public key")
	}
	r := &Receipt{Body: body, Signature: hex.EncodeToString(sig)}
	return r.Canonical()
}
