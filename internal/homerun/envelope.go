//go:build unix

package homerun

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// NamespacePrefix starts the SSH-signature namespace an approval must be made in. The deployment appends
// its host label (NamespaceFor) and carries the result in Limits.Namespace and SigVerifier.Namespace, so
// no host name lives in this source. A signature in any other namespace (a git signature, say) is never
// an approval.
const NamespacePrefix = "home-run@"

// DefaultNamespace is the namespace of DefaultLimits. A deployment sets its own; a signer using another
// namespace is then refused, so forgetting to set it fails closed.
const DefaultNamespace = NamespacePrefix + "local"

var reNamespace = regexp.MustCompile(`^home-run@[a-z0-9][a-z0-9-]{0,30}$`)

// NamespaceFor returns the signature namespace for a host label such as "dev1".
func NamespaceFor(label string) (string, error) {
	ns := NamespacePrefix + label
	if !reNamespace.MatchString(ns) {
		return "", fmt.Errorf("host label %q is not 1-31 lower-case letters, digits or hyphens", label)
	}
	return ns, nil
}

// Envelope versions, operations and privileges.
const (
	EnvelopeVersion = 1

	OpCheck   = "check"
	OpApply   = "apply"
	OpRecover = "recover"

	PrivMk   = "mk"
	PrivRoot = "root"
)

var (
	reHex64    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	reHex32    = regexp.MustCompile(`^[0-9a-f]{32}$`)
	reFlag     = regexp.MustCompile(`^--[a-z][a-z0-9-]{0,31}$`)
	reRuling   = regexp.MustCompile(`^q[0-9]{1,8}$`)
	reName     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	reRev      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)
	reAttempt  = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
	maxFlags   = 32
	maxDeps    = 64
	maxPathLen = 4096
)

// ErrEnvelope is wrapped by every envelope validation failure.
var ErrEnvelope = errors.New("invalid operation envelope")

// FieldError names the envelope field that failed and why.
type FieldError struct {
	Field  string
	Reason string
}

func (e *FieldError) Error() string { return fmt.Sprintf("envelope field %s: %s", e.Field, e.Reason) }
func (e *FieldError) Unwrap() error { return ErrEnvelope }

func bad(field, format string, a ...any) error {
	return &FieldError{Field: field, Reason: fmt.Sprintf(format, a...)}
}

// Dep pins one file the script sources or executes: its path (informational, where the signer saw it)
// and the sha256 of its bytes. On the wire it is a two-element array [path, sha256].
type Dep struct {
	Path   string
	SHA256 string
}

func (d Dep) MarshalJSON() ([]byte, error) { return json.Marshal([2]string{d.Path, d.SHA256}) }

func (d *Dep) UnmarshalJSON(b []byte) error {
	var a []string
	if err := json.Unmarshal(b, &a); err != nil || len(a) != 2 {
		return errors.New("a dep must be a two-element array [path, sha256]")
	}
	d.Path, d.SHA256 = a[0], a[1]
	return nil
}

// Envelope is the operation a signature approves. Every field is signed; none is taken from card text
// at execution time. Pointer fields serialise as null when unset.
type Envelope struct {
	V              int      `json:"v"`
	NS             string   `json:"ns"`
	Op             string   `json:"op"`
	Host           string   `json:"host"`
	Project        string   `json:"project"`
	CardID         string   `json:"card_id"`
	CardRev        string   `json:"card_rev"`
	ScriptSHA256   string   `json:"script_sha256"`
	ScriptPath     string   `json:"script_path"`
	Deps           []Dep    `json:"deps"`
	DepsDigest     string   `json:"deps_digest"`
	Flags          []string `json:"flags"`
	Privilege      string   `json:"privilege"`
	RuntimeMaxS    int64    `json:"runtime_max_s"`
	RulingID       string   `json:"ruling_id"`
	LintSHA256     string   `json:"lint_sha256"`
	ReviewSHA256   string   `json:"review_sha256"`
	CheckAttempt   *string  `json:"check_attempt"`
	CheckOutputSHA *string  `json:"check_output_sha256"`
	Nonce          string   `json:"nonce"`
	IssuedAt       int64    `json:"issued_at"`
	ExpiresAt      int64    `json:"expires_at"`
}

// DepsDigest returns the digest the envelope's deps_digest must carry: sha256 over the RFC 8785 encoding of
// the list of [path, sha256] pairs, in the order given (the envelope requires that order to be sorted).
func DepsDigest(deps []Dep) (string, error) {
	if deps == nil {
		deps = []Dep{}
	}
	raw, err := json.Marshal(deps)
	if err != nil {
		return "", err
	}
	canon, err := Canonicalize(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

// Canonical returns the RFC 8785 bytes of the envelope. These exact bytes are what is signed, stored
// and hashed. Canonical does not validate; call Validate first.
func (e *Envelope) Canonical() ([]byte, error) {
	cp := *e
	if cp.Deps == nil {
		cp.Deps = []Dep{}
	}
	if cp.Flags == nil {
		cp.Flags = []string{}
	}
	raw, err := json.Marshal(&cp)
	if err != nil {
		return nil, err
	}
	return Canonicalize(raw)
}

// Digest is the lower-case hex sha256 of the canonical bytes.
func (e *Envelope) Digest() (string, error) {
	c, err := e.Canonical()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(c)
	return hex.EncodeToString(sum[:]), nil
}

// ParseEnvelope decodes canonical envelope bytes. It refuses unknown or missing fields, duplicate keys and
// any byte sequence that is not already the canonical encoding, so the bytes that were signed are the
// bytes that were parsed. It does not apply policy; call Validate.
func ParseEnvelope(b []byte) (*Envelope, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var e Envelope
	if err := dec.Decode(&e); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEnvelope, err)
	}
	// The key set must be exactly the schema's. encoding/json matches names case-insensitively and folds
	// look-alike Unicode (U+017F is "s"), so a count of keys is not enough: an alias member would be taken
	// for a schema member.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEnvelope, err)
	}
	want := envelopeKeys()
	if len(m) != len(want) {
		return nil, bad("*", "has %d fields, want exactly %d", len(m), len(want))
	}
	for k := range m {
		if !want[k] {
			return nil, bad("*", "field %q is not part of the schema", k)
		}
	}
	canon, err := Canonicalize(b)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEnvelope, err)
	}
	if !bytes.Equal(canon, b) {
		return nil, bad("*", "not in canonical (RFC 8785) form")
	}
	if e.Deps == nil || e.Flags == nil {
		return nil, bad("*", "deps and flags must be arrays")
	}
	// What was parsed must encode back to exactly the input, so the struct the policy runs on is the
	// document that was signed.
	if re, err := e.Canonical(); err != nil || !bytes.Equal(re, b) {
		return nil, bad("*", "parsed envelope does not re-encode to the signed bytes")
	}
	return &e, nil
}

// envelopeKeys is the exact set of member names, taken from the struct's json tags.
func envelopeKeys() map[string]bool {
	raw, _ := json.Marshal(Envelope{})
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

// Validate applies the field rules and the policy in lim. It checks form only; time and host binding are
// CheckFresh and CheckHost.
func (e *Envelope) Validate(lim Limits) error {
	if e.V != EnvelopeVersion {
		return bad("v", "must be %d", EnvelopeVersion)
	}
	if lim.Namespace == "" || e.NS != lim.Namespace {
		return bad("ns", "must be %q", lim.Namespace)
	}
	switch e.Op {
	case OpCheck, OpApply, OpRecover:
	default:
		return bad("op", "must be check, apply or recover")
	}
	if !reHex32.MatchString(e.Host) {
		return bad("host", "must be a 32-character lower-case hex machine id")
	}
	if !reName.MatchString(e.Project) {
		return bad("project", "must match %s", reName)
	}
	if !reName.MatchString(e.CardID) {
		return bad("card_id", "must match %s", reName)
	}
	if !reRev.MatchString(e.CardRev) {
		return bad("card_rev", "must match %s", reRev)
	}
	if !reHex64.MatchString(e.ScriptSHA256) {
		return bad("script_sha256", "must be 64 lower-case hex characters")
	}
	if err := checkAbsPath(e.ScriptPath); err != nil {
		return bad("script_path", "%v", err)
	}
	if len(e.Deps) > maxDeps {
		return bad("deps", "more than %d entries", maxDeps)
	}
	for i, d := range e.Deps {
		if err := checkAbsPath(d.Path); err != nil {
			return bad("deps", "entry %d path: %v", i, err)
		}
		if !reHex64.MatchString(d.SHA256) {
			return bad("deps", "entry %d sha256 must be 64 lower-case hex characters", i)
		}
		if i > 0 {
			p := e.Deps[i-1]
			if p.Path > d.Path || (p.Path == d.Path && p.SHA256 >= d.SHA256) {
				return bad("deps", "must be sorted by path then sha256, without duplicates")
			}
		}
	}
	dd, err := DepsDigest(e.Deps)
	if err != nil || dd != e.DepsDigest {
		return bad("deps_digest", "does not match the digest of deps")
	}
	if len(e.Flags) > maxFlags {
		return bad("flags", "more than %d entries", maxFlags)
	}
	seen := map[string]bool{}
	for _, f := range e.Flags {
		if !reFlag.MatchString(f) {
			return bad("flags", "%q must match %s", f, reFlag)
		}
		if seen[f] {
			return bad("flags", "%q appears twice", f)
		}
		seen[f] = true
	}
	switch e.Privilege {
	case PrivMk:
	case PrivRoot:
		if !lim.RootEnabled {
			return bad("privilege", "root launches are disabled")
		}
	default:
		return bad("privilege", "must be mk or root")
	}
	if e.RuntimeMaxS <= 0 {
		return bad("runtime_max_s", "must be positive")
	}
	if c := lim.runtimeCap(e.Op); e.RuntimeMaxS > c {
		return bad("runtime_max_s", "%d exceeds the policy cap %d for %s", e.RuntimeMaxS, c, e.Op)
	}
	if !reRuling.MatchString(e.RulingID) {
		return bad("ruling_id", "must match %s", reRuling)
	}
	if !reHex64.MatchString(e.LintSHA256) {
		return bad("lint_sha256", "must be 64 lower-case hex characters")
	}
	if !reHex64.MatchString(e.ReviewSHA256) {
		return bad("review_sha256", "must be 64 lower-case hex characters")
	}
	switch e.Op {
	case OpCheck:
		if e.CheckAttempt != nil || e.CheckOutputSHA != nil {
			return bad("check_attempt", "must be null (with check_output_sha256) for a check")
		}
	default:
		if e.CheckAttempt == nil || !reAttempt.MatchString(*e.CheckAttempt) {
			return bad("check_attempt", "an %s must cite a check attempt (26-character ULID)", e.Op)
		}
		if e.CheckOutputSHA == nil || !reHex64.MatchString(*e.CheckOutputSHA) {
			return bad("check_output_sha256", "an %s must cite the check output digest", e.Op)
		}
	}
	if !reHex32.MatchString(e.Nonce) {
		return bad("nonce", "must be 32 lower-case hex characters (128 bits)")
	}
	if e.IssuedAt <= 0 || e.ExpiresAt <= e.IssuedAt {
		return bad("expires_at", "must be after issued_at, both positive Unix seconds")
	}
	// Compare whole seconds: converting an attacker-chosen span to a Duration first can overflow.
	if span, max := e.ExpiresAt-e.IssuedAt, int64(lim.ApprovalTTL/time.Second); span > max {
		return bad("expires_at", "approval lifetime %d s exceeds %d s", span, max)
	}
	return nil
}

func checkAbsPath(p string) error {
	if p == "" || len(p) > maxPathLen {
		return errors.New("must be a non-empty path of at most 4096 bytes")
	}
	if !path.IsAbs(p) || path.Clean(p) != p {
		return errors.New("must be an absolute, clean path")
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return errors.New("must not contain control characters")
		}
	}
	return nil
}

// CheckFresh reports an envelope that is not yet valid or has expired at now.
func (e *Envelope) CheckFresh(now time.Time) error {
	t := now.Unix()
	if t < e.IssuedAt {
		return bad("issued_at", "is in the future")
	}
	if t >= e.ExpiresAt {
		return bad("expires_at", "approval has expired")
	}
	return nil
}

// CheckHost reports an envelope signed for another machine.
func (e *Envelope) CheckHost(machineID string) error {
	if e.Host != machineID {
		return bad("host", "envelope is for another host")
	}
	return nil
}

// ParseFlagsHeader reads the "# flags:" line from the head of a script (its first 64 lines) and returns
// the flags it declares. A script may declare the header at most once; no header means no flags.
func ParseFlagsHeader(script []byte) ([]string, error) {
	var flags []string
	found := false
	for i, line := range strings.Split(string(script), "\n") {
		if i >= 64 {
			break
		}
		rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "# flags:")
		if !ok {
			continue
		}
		if found {
			return nil, errors.New("script declares a # flags: header twice")
		}
		found = true
		for _, f := range strings.Fields(rest) {
			if !reFlag.MatchString(f) {
				return nil, fmt.Errorf("script header flag %q must match %s", f, reFlag)
			}
			flags = append(flags, f)
		}
	}
	sort.Strings(flags)
	return flags, nil
}

// CheckFlagsAgainstHeader refuses a flag that the script's own header does not declare. This is what
// keeps card text out of argv: only a flag the signed script itself lists can be passed to it.
func (e *Envelope) CheckFlagsAgainstHeader(script []byte) error {
	declared, err := ParseFlagsHeader(script)
	if err != nil {
		return bad("flags", "%v", err)
	}
	ok := map[string]bool{}
	for _, f := range declared {
		ok[f] = true
	}
	for _, f := range e.Flags {
		if !ok[f] {
			return bad("flags", "%q is not declared in the script's # flags: header", f)
		}
	}
	return nil
}

// LaunchArgv is the whole command line of a run: the interpreter, the CAS copy, then the validated flags.
// Nothing else reaches it.
func LaunchArgv(bash, casPath string, e *Envelope) []string {
	argv := make([]string, 0, 2+len(e.Flags))
	argv = append(argv, bash, casPath)
	return append(argv, e.Flags...)
}
