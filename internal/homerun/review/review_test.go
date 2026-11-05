package review

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Unix(1_800_000_000, 0)

type testSigner struct{ priv ed25519.PrivateKey }

func newSigner(t *testing.T) *testSigner {
	t.Helper()
	_, p, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &testSigner{p}
}
func (s *testSigner) Sign(m []byte) ([]byte, error) { return ed25519.Sign(s.priv, m), nil }
func (s *testSigner) PublicKey() ed25519.PublicKey  { return s.priv.Public().(ed25519.PublicKey) }

type testProvider struct {
	res   ProviderResult
	err   error
	calls int
}

func (p *testProvider) Review(_ context.Context, _ Request) (ProviderResult, error) {
	p.calls++
	return p.res, p.err
}

func hx(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func okResult(verdict string) ProviderResult {
	return ProviderResult{Provider: "testlab", Model: "test-model-1", DispatchReceiptSHA256: hx("dispatch"),
		Text: "Looks fine.\nVERDICT: " + verdict + "\n"}
}

var script = []byte("#!/bin/bash\necho hi\n")
var deps = []DepInput{{Path: "/a/lib.sh", Content: []byte("x=1\n")}, {Path: "/b/lib.sh", Content: []byte("y=2\n")}}
var flags = []string{"--dry-run"}

type fixture struct {
	sg  *testSigner
	svc *Service
	v   *Verifier
	exp Expect
}

func newFixture(t *testing.T, res ProviderResult) *fixture {
	t.Helper()
	sg := newSigner(t)
	dd, err := DepsDigestOf(deps)
	if err != nil {
		t.Fatal(err)
	}
	ss := sha256.Sum256(script)
	return &fixture{
		sg:  sg,
		svc: &Service{Provider: &testProvider{res: res}, Signer: sg, TTL: 10 * time.Minute, Now: func() time.Time { return t0 }},
		v:   &Verifier{PublicKey: sg.PublicKey()},
		exp: Expect{ScriptSHA256: hex.EncodeToString(ss[:]), DepsDigest: dd, Flags: flags, Now: t0.Add(time.Minute)},
	}
}

func (f *fixture) issue(t *testing.T) []byte {
	t.Helper()
	raw, err := f.svc.Issue(context.Background(), Request{Script: script, Deps: deps, Flags: flags})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return raw
}

func wantRefused(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil {
		t.Fatalf("accepted, want refusal containing %q", sub)
	}
	if !errors.Is(err, ErrReceipt) {
		t.Fatalf("error %v is not ErrReceipt", err)
	}
	if !strings.Contains(err.Error(), sub) {
		t.Fatalf("error %q lacks %q", err, sub)
	}
}

func TestValidReceiptAccepted(t *testing.T) {
	f := newFixture(t, okResult("PASS"))
	raw := f.issue(t)
	digest, err := f.v.Verify(raw, f.exp)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := ParseReceipt(raw)
	if d, _ := r.Digest(); d != digest || len(digest) != 64 {
		t.Fatalf("digest %q", digest)
	}
}

func TestVerifyMatrix(t *testing.T) {
	f := newFixture(t, okResult("PASS"))
	raw := f.issue(t)
	other := newSigner(t)
	cases := []struct {
		name string
		v    *Verifier
		raw  []byte
		exp  func(Expect) Expect
		want string
	}{
		{"wrong key", &Verifier{PublicKey: other.PublicKey()}, raw, nil, "does not verify"},
		{"other script", f.v, raw, func(e Expect) Expect { e.ScriptSHA256 = hx("other"); return e }, "different script"},
		{"other deps", f.v, raw, func(e Expect) Expect { e.DepsDigest = hx("other"); return e }, "different dependencies"},
		{"other flags", f.v, raw, func(e Expect) Expect { e.Flags = []string{"--apply"}; return e }, "different flags"},
		{"no flags", f.v, raw, func(e Expect) Expect { e.Flags = nil; return e }, "different flags"},
		{"expired", f.v, raw, func(e Expect) Expect { e.Now = t0.Add(10 * time.Minute); return e }, "expired"},
		{"before issue", f.v, raw, func(e Expect) Expect { e.Now = t0.Add(-time.Second); return e }, "not yet valid"},
		{"empty", f.v, nil, nil, "no review receipt"},
		{"garbage", f.v, []byte("{}"), nil, "refused"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := f.exp
			if c.exp != nil {
				e = c.exp(e)
			}
			_, err := c.v.Verify(c.raw, e)
			wantRefused(t, err, c.want)
		})
	}
	// Last instant of validity is expires_at-1.
	e := f.exp
	e.Now = t0.Add(10*time.Minute - time.Second)
	if _, err := f.v.Verify(raw, e); err != nil {
		t.Fatalf("still valid one second before expiry: %v", err)
	}
}

func TestUnconfiguredVerifierRefuses(t *testing.T) {
	f := newFixture(t, okResult("PASS"))
	if _, err := (&Verifier{}).Verify(f.issue(t), f.exp); err == nil {
		t.Fatal("verifier with no key accepted a receipt")
	}
}

func TestFailVerdictYieldsFailReceiptVerifierRefuses(t *testing.T) {
	f := newFixture(t, okResult("FAIL"))
	raw := f.issue(t)
	r, err := ParseReceipt(raw)
	if err != nil || r.Outcome != OutcomeFail {
		t.Fatalf("want a FAIL receipt, got %+v %v", r, err)
	}
	_, err = f.v.Verify(raw, f.exp)
	wantRefused(t, err, "not PASS")
}

// resign re-signs a mutated body with the real key, to prove the field checks and not only the signature.
func resign(t *testing.T, f *fixture, mut func(*Body)) []byte {
	t.Helper()
	r, err := ParseReceipt(f.issue(t))
	if err != nil {
		t.Fatal(err)
	}
	mut(&r.Body)
	msg, _ := r.Body.SigningBytes()
	r.Signature = hex.EncodeToString(ed25519.Sign(f.sg.priv, msg))
	raw, err := r.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestTamperedFieldsRefused(t *testing.T) {
	f := newFixture(t, okResult("PASS"))
	raw := f.issue(t)
	r, _ := ParseReceipt(raw)
	muts := map[string]func(*Receipt){
		"outcome":  func(r *Receipt) { r.Outcome = OutcomeFail },
		"script":   func(r *Receipt) { r.ScriptSHA256 = hx("x") },
		"deps":     func(r *Receipt) { r.DepsDigest = hx("x") },
		"provider": func(r *Receipt) { r.Provider = "other" },
		"model":    func(r *Receipt) { r.Model = "other" },
		"dispatch": func(r *Receipt) { r.DispatchReceipt = hx("x") },
		"issued":   func(r *Receipt) { r.IssuedAt-- },
		"expires":  func(r *Receipt) { r.ExpiresAt += 3600 },
		"flags":    func(r *Receipt) { r.Flags = append(r.Flags, "--extra") },
	}
	for name, m := range muts {
		t.Run(name, func(t *testing.T) {
			cp := *r
			cp.Flags = append([]string(nil), r.Flags...)
			m(&cp)
			tampered, err := cp.Canonical()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.v.Verify(tampered, f.exp); err == nil {
				t.Fatalf("tampered %s accepted", name)
			}
		})
	}
	t.Run("signature bit flip", func(t *testing.T) {
		cp := *r
		b := []byte(cp.Signature)
		if b[0] == '0' {
			b[0] = '1'
		} else {
			b[0] = '0'
		}
		cp.Signature = string(b)
		tampered, _ := cp.Canonical()
		_, err := f.v.Verify(tampered, f.exp)
		wantRefused(t, err, "does not verify")
	})
	t.Run("stripped signature", func(t *testing.T) {
		cp := *r
		cp.Signature = ""
		tampered, _ := cp.Canonical()
		_, err := f.v.Verify(tampered, f.exp)
		wantRefused(t, err, "signature is malformed")
	})
	t.Run("non-canonical bytes", func(t *testing.T) {
		_, err := f.v.Verify(append([]byte(" "), raw...), f.exp)
		wantRefused(t, err, "")
	})
	t.Run("unknown field", func(t *testing.T) {
		bad := strings.Replace(string(raw), `"flags"`, `"extra":1,"flags"`, 1)
		_, err := f.v.Verify([]byte(bad), f.exp)
		wantRefused(t, err, "")
	})
}

func TestMissingOrBadDispatchReceiptRefused(t *testing.T) {
	f := newFixture(t, okResult("PASS"))
	for _, d := range []string{"", "abc", strings.ToUpper(hx("d"))} {
		raw := resign(t, f, func(b *Body) { b.DispatchReceipt = d })
		_, err := f.v.Verify(raw, f.exp)
		wantRefused(t, err, "dispatch_receipt_sha256")
	}
	// The service itself never issues one.
	res := okResult("PASS")
	res.DispatchReceiptSHA256 = ""
	g := newFixture(t, res)
	if raw, err := g.svc.Issue(context.Background(), Request{Script: script, Deps: deps, Flags: flags}); err == nil || raw != nil {
		t.Fatal("service issued a receipt without a dispatch receipt")
	}
}

func TestBodyShapeRefusals(t *testing.T) {
	f := newFixture(t, okResult("PASS"))
	muts := map[string]func(*Body){
		"version":   func(b *Body) { b.V = 2 },
		"namespace": func(b *Body) { b.NS = "other" },
		"provider":  func(b *Body) { b.Provider = "" },
		"model":     func(b *Body) { b.Model = "has space" },
		"outcome":   func(b *Body) { b.Outcome = "pass" },
		"order":     func(b *Body) { b.ExpiresAt = b.IssuedAt },
		"ttl":       func(b *Body) { b.ExpiresAt = b.IssuedAt + int64(MaxTTL/time.Second) + 1 },
		"flag":      func(b *Body) { b.Flags = []string{"-x"} },
	}
	for name, m := range muts {
		t.Run(name, func(t *testing.T) {
			raw := resign(t, f, m)
			if _, err := f.v.Verify(raw, f.exp); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestProviderErrorMeansNoReceipt(t *testing.T) {
	f := newFixture(t, ProviderResult{})
	f.svc.Provider = &testProvider{err: errors.New("model unavailable")}
	raw, err := f.svc.Issue(context.Background(), Request{Script: script, Deps: deps, Flags: flags})
	if err == nil || raw != nil {
		t.Fatalf("got %q, %v", raw, err)
	}
}

func TestMalformedVerdictMeansNoReceipt(t *testing.T) {
	for name, text := range map[string]string{
		"none":      "all good",
		"two":       "VERDICT: PASS\nVERDICT: PASS\n",
		"mixed":     "VERDICT: PASS\nVERDICT: FAIL\n",
		"inline":    "my VERDICT: PASS maybe",
		"lowercase": "verdict: pass",
		"empty":     "",
	} {
		t.Run(name, func(t *testing.T) {
			res := okResult("PASS")
			res.Text = text
			f := newFixture(t, res)
			raw, err := f.svc.Issue(context.Background(), Request{Script: script, Deps: deps, Flags: flags})
			if err == nil || raw != nil {
				t.Fatalf("issued %q", raw)
			}
		})
	}
}

func TestSignerFailureAndBadSignatureMeanNoReceipt(t *testing.T) {
	f := newFixture(t, okResult("PASS"))
	f.svc.Signer = badSigner{f.sg}
	if raw, err := f.svc.Issue(context.Background(), Request{Script: script, Deps: deps, Flags: flags}); err == nil || raw != nil {
		t.Fatal("issued with a signer whose output does not verify")
	}
	f.svc.Signer = failSigner{f.sg}
	if raw, err := f.svc.Issue(context.Background(), Request{Script: script, Deps: deps, Flags: flags}); err == nil || raw != nil {
		t.Fatal("issued although signing failed")
	}
}

type badSigner struct{ *testSigner }

func (badSigner) Sign([]byte) ([]byte, error) { return make([]byte, ed25519.SignatureSize), nil }

type failSigner struct{ *testSigner }

func (failSigner) Sign([]byte) ([]byte, error) { return nil, errors.New("secret detail") }

func TestRequestBounds(t *testing.T) {
	f := newFixture(t, okResult("PASS"))
	ctx := context.Background()
	if _, err := f.svc.Issue(ctx, Request{}); err == nil {
		t.Fatal("empty script accepted")
	}
	unsorted := []DepInput{{Path: "/b", Content: []byte("1")}, {Path: "/a", Content: []byte("2")}}
	if _, err := f.svc.Issue(ctx, Request{Script: script, Deps: unsorted}); err == nil {
		t.Fatal("unsorted deps accepted")
	}
	dup := []DepInput{{Path: "/a"}, {Path: "/a"}}
	if _, err := f.svc.Issue(ctx, Request{Script: script, Deps: dup}); err == nil {
		t.Fatal("duplicate deps accepted")
	}
	f.svc.MaxScriptBytes = 4
	if _, err := f.svc.Issue(ctx, Request{Script: script}); err == nil {
		t.Fatal("oversize script accepted")
	}
	if p := f.svc.Provider.(*testProvider); p.calls != 0 {
		t.Fatalf("provider called %d times for refused requests", p.calls)
	}
}

func TestReceiptBindsEachInput(t *testing.T) {
	f := newFixture(t, okResult("PASS"))
	raw := f.issue(t)
	// A receipt issued for other script bytes or other dep bytes does not satisfy this operation.
	for name, req := range map[string]Request{
		"script": {Script: append([]byte("#"), script...), Deps: deps, Flags: flags},
		"dep":    {Script: script, Deps: []DepInput{{Path: "/a/lib.sh", Content: []byte("x=2\n")}, deps[1]}, Flags: flags},
		"flags":  {Script: script, Deps: deps, Flags: []string{"--apply"}},
	} {
		other, err := f.svc.Issue(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.v.Verify(other, f.exp); err == nil {
			t.Fatalf("receipt for other %s accepted", name)
		}
	}
	if _, err := f.v.Verify(raw, f.exp); err != nil {
		t.Fatal(err)
	}
}

// ---- key handling ----

func fakeLstat(m map[string]FileMeta) func(string) (FileMeta, error) {
	return func(p string) (FileMeta, error) {
		if v, ok := m[p]; ok {
			return v, nil
		}
		return FileMeta{}, fs.ErrNotExist
	}
}

func TestCheckKeyPathMatrix(t *testing.T) {
	const kp = "/svc/keys/review.key"
	good := FileMeta{Mode: 0o600, UID: 500}
	dir := FileMeta{Mode: fs.ModeDir | 0o700, UID: 500}
	cases := []struct {
		name string
		file FileMeta
		dir  FileMeta
		path string
		ok   bool
	}{
		{"own uid 0600", good, dir, kp, true},
		{"own uid 0400", FileMeta{Mode: 0o400, UID: 500}, dir, kp, true},
		{"root owned", FileMeta{Mode: 0o600, UID: 0}, dir, kp, true},
		{"root dir 0755", good, FileMeta{Mode: fs.ModeDir | 0o755, UID: 0}, kp, true},
		{"group readable", FileMeta{Mode: 0o640, UID: 500}, dir, kp, false},
		{"other readable", FileMeta{Mode: 0o604, UID: 500}, dir, kp, false},
		{"group writable", FileMeta{Mode: 0o620, UID: 500}, dir, kp, false},
		{"other writable", FileMeta{Mode: 0o602, UID: 500}, dir, kp, false},
		{"foreign owner", FileMeta{Mode: 0o600, UID: 1000}, dir, kp, false},
		{"symlink", FileMeta{Mode: fs.ModeSymlink | 0o600, UID: 500}, dir, kp, false},
		{"directory", FileMeta{Mode: fs.ModeDir | 0o700, UID: 500}, dir, kp, false},
		{"dir group writable", good, FileMeta{Mode: fs.ModeDir | 0o770, UID: 500}, kp, false},
		{"dir other writable", good, FileMeta{Mode: fs.ModeDir | 0o707, UID: 500}, kp, false},
		{"dir foreign owner", good, FileMeta{Mode: fs.ModeDir | 0o700, UID: 1000}, kp, false},
		{"dir is symlink", good, FileMeta{Mode: fs.ModeSymlink | 0o700, UID: 500}, kp, false},
		{"relative", good, dir, "keys/review.key", false},
		{"unclean", good, dir, "/svc/keys/../keys/review.key", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opt := KeyOptions{SelfUID: 500, Lstat: fakeLstat(map[string]FileMeta{kp: c.file, "/svc/keys": c.dir})}
			err := CheckKeyPath(c.path, opt)
			if c.ok != (err == nil) {
				t.Fatalf("ok=%v err=%v", c.ok, err)
			}
		})
	}
	if err := CheckKeyPath(kp, KeyOptions{SelfUID: 500, Lstat: fakeLstat(nil)}); err == nil {
		t.Fatal("missing key accepted")
	}
}

func writeKey(t *testing.T, mode os.FileMode) (string, ed25519.PublicKey, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "review.key")
	seedHex := hex.EncodeToString(seed)
	if err := os.WriteFile(p, []byte(seedHex+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p, ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey), seedHex
}

func TestLoadKeyRealFiles(t *testing.T) {
	opt := KeyOptions{SelfUID: uint32(os.Getuid())}
	p, pub, seedHex := writeKey(t, 0o600)
	sg, err := LoadKey(p, opt)
	if err != nil {
		t.Fatal(err)
	}
	if !sg.PublicKey().Equal(pub) {
		t.Fatal("loaded a different key")
	}
	for _, s := range []string{fmt.Sprintf("%v", sg), fmt.Sprintf("%+v", sg), fmt.Sprintf("%#v", sg), fmt.Sprintf("%s", sg)} {
		if strings.Contains(s, seedHex) {
			t.Fatalf("formatting leaks key material: %s", s)
		}
	}
	// A signature from the loaded key is accepted by the verifier.
	f := newFixture(t, okResult("PASS"))
	f.svc.Signer, f.v = sg, &Verifier{PublicKey: pub}
	if _, err := f.v.Verify(f.issue(t), f.exp); err != nil {
		t.Fatal(err)
	}
}

func TestLoadKeyRefusals(t *testing.T) {
	opt := KeyOptions{SelfUID: uint32(os.Getuid())}
	for _, mode := range []os.FileMode{0o644, 0o640, 0o660, 0o666, 0o604} {
		p, _, seedHex := writeKey(t, mode)
		_, err := LoadKey(p, opt)
		if err == nil {
			t.Fatalf("mode %04o accepted", mode)
		}
		if strings.Contains(err.Error(), seedHex) {
			t.Fatal("error leaks key material")
		}
	}
	// Foreign owner by injected self uid.
	p, _, _ := writeKey(t, 0o600)
	if os.Getuid() != 0 {
		if _, err := LoadKey(p, KeyOptions{SelfUID: uint32(os.Getuid()) + 1}); err == nil {
			t.Fatal("file owned by another uid accepted")
		}
	}
	// Symlink to a good key.
	link := filepath.Join(filepath.Dir(p), "link.key")
	if err := os.Symlink(p, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(link, opt); err == nil {
		t.Fatal("symlink accepted")
	}
	// Group-writable directory.
	d := filepath.Dir(p)
	if err := os.Chmod(d, 0o770); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(p, opt); err == nil {
		t.Fatal("group-writable directory accepted")
	}
	if err := os.Chmod(d, 0o700); err != nil {
		t.Fatal(err)
	}
	// Bad content: error must not echo it.
	bad := "not-hex-but-secret-looking-material"
	if err := os.WriteFile(p, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadKey(p, opt)
	if err == nil || strings.Contains(err.Error(), bad) {
		t.Fatalf("err %v", err)
	}
	// Short seed.
	if err := os.WriteFile(p, []byte(hex.EncodeToString(make([]byte, 16))), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(p, opt); err == nil {
		t.Fatal("short seed accepted")
	}
}
