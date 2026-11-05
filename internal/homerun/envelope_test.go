//go:build unix

package homerun

import (
	"errors"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestCanonicalizeRFC8785(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"b":1,"a":2}`, `{"a":2,"b":1}`},
		{`{ "a" : [ 1 , 2 ] , "b" : null }`, `{"a":[1,2],"b":null}`},
		{`{"é":1,"z":2,"😀":3}`, "{\"z\":2,\"é\":1,\"\U0001F600\":3}"},
		{`{"￮":2,"😀":1}`, "{\"\U0001F600\":1,\"￮\":2}"}, // UTF-16 order: the pair (D83D) sorts before U+FFEE
		{`"a\u000ab\u007f\/"`, "\"a\\nb\u007f/\""},
		{`[0, 9007199254740991, -9007199254740991]`, `[0,9007199254740991,-9007199254740991]`},
		{`"\u0001"`, `"\u0001"`},
	}
	for _, c := range cases {
		got, err := Canonicalize([]byte(c.in))
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		// JCS orders the surrogate pair (D83D) below U+FFEE (UTF-16 order), not above (code-point order)
		if string(got) != c.want {
			t.Errorf("%s: got %q want %q", c.in, got, c.want)
		}
	}
	for _, bad := range []string{`1.5`, `1e3`, `9007199254740992`, `-9007199254740992`, `{"a":1} x`, `{"a":1,"a":2}`, `{"x":{"a":1,"a":2}}`, "\"\xff\"", `"\ud800"`, `"\udc00"`, `"\ud800\u0041"`, `-0`, ``, `{`, `[1,]`} {
		if _, err := Canonicalize([]byte(bad)); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	now := time.Now()
	e := testEnvelope(t, now)
	b := canon(t, e)
	got, err := ParseEnvelope(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Validate(DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	if string(canon(t, got)) != string(b) {
		t.Fatal("round trip changed the bytes")
	}
	// the wire form is JCS: keys sorted, no whitespace
	if !strings.HasPrefix(string(b), `{"card_id":"AUT-1","card_rev":"rev-1","check_attempt":null,`) || strings.ContainsAny(string(b), " \n\t") {
		t.Fatalf("not canonical: %s", b)
	}
}

func TestParseEnvelopeStrict(t *testing.T) {
	b := string(canon(t, testEnvelope(t, time.Now())))
	for name, in := range map[string]string{
		"whitespace":    strings.Replace(b, `{"card_id"`, `{ "card_id"`, 1),
		"unknown field": strings.Replace(b, `{"card_id"`, `{"extra":1,"card_id"`, 1),
		"missing field": strings.Replace(b, `"card_rev":"rev-1",`, ``, 1),
		"duplicate key": strings.Replace(b, `{"card_id":"AUT-1"`, `{"card_id":"AUT-1","card_id":"AUT-1"`, 1),
		"trailing":      b + "\n",
		"unsorted":      `{"v":1,` + strings.TrimPrefix(b, `{`),
		"float":         strings.Replace(b, `"runtime_max_s":600`, `"runtime_max_s":600.0`, 1),
		"wrong type":    strings.Replace(b, `"v":1`, `"v":"1"`, 1),
	} {
		if _, err := ParseEnvelope([]byte(in)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestEnvelopeFieldValidation(t *testing.T) {
	now := time.Now()
	lim := DefaultLimits()
	str := func(s string) *string { return &s }
	muts := map[string]func(*Envelope){
		"host short":       func(e *Envelope) { e.Host = "abc" },
		"project":          func(e *Envelope) { e.Project = "../x" },
		"card_id space":    func(e *Envelope) { e.CardID = "A B" },
		"card_id empty":    func(e *Envelope) { e.CardID = "" },
		"card_rev":         func(e *Envelope) { e.CardRev = "a/b" },
		"script sha upper": func(e *Envelope) { e.ScriptSHA256 = strings.ToUpper(sha1Hex[:60]) + "ABCD" },
		"script path rel":  func(e *Envelope) { e.ScriptPath = "run.sh" },
		"script path dots": func(e *Envelope) { e.ScriptPath = "/a/../b" },
		"script path ctl":  func(e *Envelope) { e.ScriptPath = "/a/\n" },
		"deps unsorted": func(e *Envelope) {
			e.Deps = []Dep{{"/b", sha1Hex}, {"/a", sha1Hex}}
			e.DepsDigest, _ = DepsDigest(e.Deps)
		},
		"deps duplicate": func(e *Envelope) {
			e.Deps = []Dep{{"/a", sha1Hex}, {"/a", sha1Hex}}
			e.DepsDigest, _ = DepsDigest(e.Deps)
		},
		"deps digest wrong":  func(e *Envelope) { e.Deps = []Dep{{"/a", sha1Hex}} },
		"flag no dashes":     func(e *Envelope) { e.Flags = []string{"dry-run"} },
		"flag equals":        func(e *Envelope) { e.Flags = []string{"--out=/etc/passwd"} },
		"flag short":         func(e *Envelope) { e.Flags = []string{"-f"} },
		"flag double":        func(e *Envelope) { e.Flags = []string{"--a", "--a"} },
		"flag shell":         func(e *Envelope) { e.Flags = []string{"--a;rm"} },
		"flag bare --":       func(e *Envelope) { e.Flags = []string{"--"} },
		"privilege":          func(e *Envelope) { e.Privilege = "admin" },
		"root disabled":      func(e *Envelope) { e.Privilege = PrivRoot },
		"runtime zero":       func(e *Envelope) { e.RuntimeMaxS = 0 },
		"check over cap":     func(e *Envelope) { e.RuntimeMaxS = 601 },
		"ruling":             func(e *Envelope) { e.RulingID = "ruling-1" },
		"lint":               func(e *Envelope) { e.LintSHA256 = "x" },
		"review":             func(e *Envelope) { e.ReviewSHA256 = "" },
		"check with attempt": func(e *Envelope) { e.CheckAttempt = str(testAttemp) },
		"nonce short":        func(e *Envelope) { e.Nonce = "abc" },
		"expires before":     func(e *Envelope) { e.ExpiresAt = e.IssuedAt },
		"ttl too long":       func(e *Envelope) { e.ExpiresAt = e.IssuedAt + 301 },
		"op unknown":         func(e *Envelope) { e.Op = "delete" },
		"version":            func(e *Envelope) { e.V = 0 },
		"namespace":          func(e *Envelope) { e.NS = "home-run@local " },
	}
	for name, m := range muts {
		e := testEnvelope(t, now)
		m(e)
		if err := e.Validate(lim); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := testEnvelope(t, now).Validate(lim); err != nil {
		t.Fatalf("base envelope refused: %v", err)
	}
}

func TestOpSpecificFieldsAndRuntimeCaps(t *testing.T) {
	now := time.Now()
	lim := DefaultLimits()
	e := testEnvelope(t, now)
	e.Op = OpApply // no check cited
	if err := e.Validate(lim); err == nil {
		t.Error("apply without a cited check accepted")
	}
	a, o := testAttemp, sha3Hex
	e.CheckAttempt, e.CheckOutputSHA = &a, &o
	e.RuntimeMaxS = 3 * 3600
	if err := e.Validate(lim); err != nil {
		t.Errorf("apply at the 3 h cap refused: %v", err)
	}
	e.RuntimeMaxS++
	if err := e.Validate(lim); err == nil {
		t.Error("apply over the 3 h cap accepted")
	}
	c := testEnvelope(t, now)
	c.RuntimeMaxS = 600
	if err := c.Validate(lim); err != nil {
		t.Errorf("check at the 10 min cap refused: %v", err)
	}
	// the cap comes from the policy struct, not a constant
	tight := lim
	tight.CheckRuntimeMax = 5 * time.Minute
	if err := c.Validate(tight); err == nil {
		t.Error("check over a tightened cap accepted")
	}
}

func TestRootPathBuiltButDisabled(t *testing.T) {
	now := time.Now()
	e := testEnvelope(t, now)
	e.Privilege = PrivRoot
	if err := e.Validate(DefaultLimits()); err == nil {
		t.Fatal("root accepted by default")
	}
	on := DefaultLimits()
	on.RootEnabled = true
	if err := e.Validate(on); err != nil {
		t.Fatalf("root path unusable when enabled: %v", err)
	}
	s, _ := newStore(t)
	_ = s
	if DefaultLimits().RootEnabled {
		t.Fatal("RootEnabled must default to false")
	}
}

func TestLimitsDefaultsAndValidate(t *testing.T) {
	l := DefaultLimits()
	if l.ApprovalTTL != 5*time.Minute || l.CheckFreshness != 30*time.Minute || l.CheckRuntimeMax != 10*time.Minute ||
		l.ApplyRuntimeMax != 3*time.Hour || l.ApplyInterval != 10*time.Minute || l.MaxExecutionsPerDay != 10 ||
		l.SigFailureThreshold != 3 || l.SigFailureWindow != 10*time.Minute || l.MaxActiveRoot != 1 || l.MaxActiveAsMk != 2 {
		t.Fatalf("unexpected defaults %+v", l)
	}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	l.ApprovalTTL = 0
	if err := l.Validate(); err == nil {
		t.Fatal("zero TTL accepted")
	}
}

// T2.4 — flags reach the command line only through the signed envelope and the script's own header.

func TestFlagsOutsideHeaderRefused(t *testing.T) {
	now := time.Now()
	script := []byte("#!/bin/bash\n# flags: --dry-run --verbose\necho hi\n")
	e := testEnvelope(t, now)
	e.Flags = []string{"--dry-run"}
	if err := e.CheckFlagsAgainstHeader(script); err != nil {
		t.Fatal(err)
	}
	e.Flags = []string{"--dry-run", "--delete-everything"}
	if err := e.CheckFlagsAgainstHeader(script); err == nil {
		t.Fatal("flag missing from the header accepted")
	}
	// no header: no flags allowed
	e.Flags = []string{"--dry-run"}
	if err := e.CheckFlagsAgainstHeader([]byte("echo hi\n")); err == nil {
		t.Fatal("flag accepted from a script with no header")
	}
	// header deeper than line 64 does not count
	late := strings.Repeat("# filler\n", 70) + "# flags: --dry-run\n"
	if err := e.CheckFlagsAgainstHeader([]byte(late)); err == nil {
		t.Fatal("header after line 64 accepted")
	}
	// a second header, or a header with '=' or other junk, is refused outright
	for _, s := range []string{
		"# flags: --a\n# flags: --b\n",
		"# flags: --out=x\n",
		"# flags: -x\n",
		"# flags: --a;id\n",
		"# flags: --a $(id)\n",
	} {
		if _, err := ParseFlagsHeader([]byte(s)); err == nil {
			t.Errorf("header %q accepted", s)
		}
	}
	// a header-looking line elsewhere in the file (indented, or inside text) is not a header
	if f, err := ParseFlagsHeader([]byte("echo '# flags: --x'\n  # flags: --y\n")); err != nil || len(f) != 0 {
		t.Errorf("non-header lines parsed as header: %v %v", f, err)
	}
}

func TestOnlyBashCASAndHeaderFlagsReachArgv(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	now := time.Now()
	lim := DefaultLimits()
	alphabet := []rune("abcXYZ019-_=;$()`'\" \t\n\\/.:*?<>|&~{}[]#!%^@+,\x00é\U0001F600")
	rnd := func(max int) string {
		n := r.Intn(max)
		b := make([]rune, n)
		for i := range b {
			b[i] = alphabet[r.Intn(len(alphabet))]
		}
		return string(b)
	}
	pool := []string{"--dry-run", "--verbose", "--limit", "--a-b-c"}
	accepted := 0
	for i := 0; i < 3000; i++ {
		e := testEnvelope(t, now)
		// card text reaches the envelope fields it is allowed to: names, revision, paths, flags
		if r.Intn(3) == 0 {
			e.Project, e.CardID, e.CardRev = rnd(12), rnd(12), rnd(12)
			e.ScriptPath = "/" + rnd(20)
		}
		nf := r.Intn(4)
		e.Flags = nil
		for j := 0; j < nf; j++ {
			if r.Intn(4) != 0 {
				e.Flags = append(e.Flags, pool[r.Intn(len(pool))])
			} else {
				e.Flags = append(e.Flags, "--"+rnd(8))
			}
		}
		header := "#!/bin/bash\n# flags: " + strings.Join(pool, " ") + " " + rnd(10) + "\n" + rnd(40)
		if e.Validate(lim) != nil {
			continue
		}
		script := []byte(header)
		if e.CheckFlagsAgainstHeader(script) != nil {
			continue
		}
		accepted++
		cas := "/var/cas/" + sha1Hex
		argv := LaunchArgv("/usr/bin/bash", cas, e)
		if argv[0] != "/usr/bin/bash" || argv[1] != cas || len(argv) != 2+len(e.Flags) {
			t.Fatalf("argv shape wrong: %q", argv)
		}
		declared, _ := ParseFlagsHeader(script)
		for _, a := range argv[2:] {
			if !reFlag.MatchString(a) {
				t.Fatalf("argv carries %q, not a plain flag", a)
			}
			found := false
			for _, d := range declared {
				found = found || d == a
			}
			if !found {
				t.Fatalf("argv carries %q that the header never declared", a)
			}
		}
		for _, a := range argv {
			if strings.ContainsAny(a, "=;$`'\" \t\n\\|&<>*?()") {
				t.Fatalf("argv element %q has shell-significant characters", a)
			}
		}
	}
	if accepted == 0 {
		t.Fatal("property test never produced an accepted envelope; it tests nothing")
	}
}

func TestCheckFlagsAgainstDeclaredSubset(t *testing.T) {
	script := []byte("# flags: --a --b\n")
	e := testEnvelope(t, time.Now())
	e.Flags = []string{"--a", "--b"}
	if err := e.CheckFlagsAgainstHeader(script); err != nil {
		t.Fatal(err)
	}
	e.Flags = nil
	if err := e.CheckFlagsAgainstHeader(script); err != nil {
		t.Fatal(err)
	}
}

func TestNamespaceIsDeploymentParameter(t *testing.T) {
	ns, err := NamespaceFor("dev1")
	if err != nil || ns != "home-run@dev1" {
		t.Fatalf("NamespaceFor: %q %v", ns, err)
	}
	for _, bad := range []string{"", "Dev", "a b", "-x", "a@b", strings.Repeat("a", 32)} {
		if _, err := NamespaceFor(bad); err == nil {
			t.Errorf("host label %q accepted", bad)
		}
	}
	lim := DefaultLimits()
	e := testEnvelope(t, time.Now())
	if err := e.Validate(lim); err != nil {
		t.Fatal(err)
	}
	lim.Namespace = "home-run@dev1" // a verifier configured for another host refuses this envelope
	if err := e.Validate(lim); !errors.Is(err, ErrEnvelope) {
		t.Errorf("envelope in another namespace accepted: %v", err)
	}
	lim.Namespace = "git"
	if lim.Validate() == nil {
		t.Error("a non-home-run namespace passed Limits.Validate")
	}
}
