package escalcheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	head1 = strings.Repeat("a", 40)
	head2 = strings.Repeat("b", 40)
)

type fakeRunner struct {
	fn    func(argv []string, env []string) (Result, error)
	calls [][]string
	envs  [][]string
}

func (f *fakeRunner) Run(_ context.Context, argv []string, env []string) (Result, error) {
	f.calls = append(f.calls, argv)
	f.envs = append(f.envs, env)
	return f.fn(argv, env)
}

type fixture struct {
	t      *testing.T
	dir    string
	script string
	digest string
	state  string
	env    Env
	run    *fakeRunner
	gh     []string // successive gh outputs
	ghN    int
}

func newFixture(t *testing.T) *fixture {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	f := &fixture{t: t, dir: dir, state: filepath.Join(dir, "state")}
	f.script = filepath.Join(dir, "h.sh")
	body := []byte("#!/bin/bash\necho hi\n")
	if err := os.WriteFile(f.script, body, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	f.digest = hex.EncodeToString(sum[:])
	f.writeReceipt(map[string]any{"passed": true, "check_rc": 0, "lint_rc": 0, "uid": 1000, "problems": []any{}})
	f.run = &fakeRunner{}
	f.run.fn = f.dispatch
	f.env = Env{Runner: f.run, Now: func() time.Time { return time.Unix(0, 0) }, Environ: func() []string {
		return []string{"HOME=" + dir, "PATH=/usr/bin", "GH_HOST=evil.example", "GH_REPO=x/y", "XDG_STATE_HOME=" + filepath.Join(dir, "xdg")}
	}, TempDir: dir}
	return f
}

func (f *fixture) writeReceipt(over map[string]any) {
	rd := filepath.Join(f.state, "r1")
	_ = os.MkdirAll(rd, 0o755)
	rec := map[string]any{"sha256": f.digest, "script": f.script}
	for k, v := range over {
		rec[k] = v
	}
	b, _ := json.Marshal(rec)
	_ = os.WriteFile(filepath.Join(rd, "receipt.json"), b, 0o644)
	_ = os.WriteFile(filepath.Join(rd, "check.log"), nil, 0o644)
}

func (f *fixture) dispatch(argv []string, env []string) (Result, error) {
	switch argv[0] {
	case "handoff-lint":
		return Result{}, nil
	case "handoff-rehearse":
		return Result{Stdout: fmt.Sprintf("REHEARSED sha256=%s uid=1000 check_rc=0 log=%s\n", f.digest, filepath.Join(f.state, "r1", "check.log"))}, nil
	case "gh":
		if f.ghN >= len(f.gh) {
			return Result{Code: 1, Stderr: "no more"}, nil
		}
		o := f.gh[f.ghN]
		f.ghN++
		return Result{Stdout: o}, nil
	}
	return Result{Code: 127}, nil
}

func ghJSON(state string, draft bool, head string, roll ...string) string {
	return fmt.Sprintf(`{"state":%q,"isDraft":%v,"headRefOid":%q,"baseRefName":"main","statusCheckRollup":[%s]}`, state, draft, head, strings.Join(roll, ","))
}

const okRun = `{"__typename":"CheckRun","name":"ci","status":"COMPLETED","conclusion":"SUCCESS"}`

func (f *fixture) review(lines ...string) string {
	p := filepath.Join(f.dir, "review.md")
	_ = os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	return p
}

func status(t *testing.T, r Report, id string) Status {
	t.Helper()
	for _, it := range r.Items {
		if it.ID == id {
			return it.Status
		}
	}
	t.Fatalf("no item %s in %+v", id, r.Items)
	return ""
}

const prURL = "https://github.com/o/r/pull/7"

func (f *fixture) opts() Options {
	return Options{Script: f.script, SHA: f.digest, PR: prURL, RehearseState: f.state}
}

func TestAllPass(t *testing.T) {
	f := newFixture(t)
	f.gh = []string{ghJSON("OPEN", false, head1, okRun), ghJSON("OPEN", false, head1, okRun)}
	o := f.opts()
	o.Review = f.review("review text", "REVIEWED-PR: o/r#7", "REVIEWED-HEAD: "+head1, "REVIEWED-SCRIPT-SHA256: "+f.digest, "VERDICT: PASS")
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	r := Run(context.Background(), o, f.env)
	if r.Overall != Pass {
		b, _ := json.MarshalIndent(r, "", " ")
		t.Fatalf("overall %s\n%s", r.Overall, b)
	}
	// gh must be pinned and not inherit GH_HOST/GH_REPO.
	for i, c := range f.run.calls {
		if c[0] == "gh" {
			if strings.Join(c[3:7], " ") != "7 --repo github.com/o/r --json" {
				t.Errorf("gh argv %v", c)
			}
			for _, kv := range f.run.envs[i] {
				if strings.HasPrefix(kv, "GH_HOST=") || strings.HasPrefix(kv, "GH_REPO=") {
					t.Errorf("leaked %s", kv)
				}
			}
		}
	}
}

func TestScriptFailures(t *testing.T) {
	f := newFixture(t)
	o := f.opts()
	o.PR = ""
	o.SHA = strings.Repeat("0", 64)
	if r := Run(context.Background(), o, f.env); status(t, r, "script.sha256") != Fail || r.Overall != Fail {
		t.Error("wrong sha must fail")
	}
	// lint findings
	f = newFixture(t)
	f.run.fn = func(a []string, e []string) (Result, error) {
		if a[0] == "handoff-lint" {
			return Result{Code: 1, Stdout: "bad"}, nil
		}
		return f.dispatch(a, e)
	}
	o = f.opts()
	o.PR = ""
	if r := Run(context.Background(), o, f.env); status(t, r, "script.lint") != Fail {
		t.Error("lint exit 1 must fail")
	}
	// lint tool missing -> UNKNOWN
	f.run.fn = func(a []string, e []string) (Result, error) {
		if a[0] == "handoff-lint" {
			return Result{}, fmt.Errorf("not found")
		}
		return f.dispatch(a, e)
	}
	if r := Run(context.Background(), o, f.env); status(t, r, "script.lint") != Unknown {
		t.Error("missing lint must be unknown")
	}
	// no exec bit
	f = newFixture(t)
	_ = os.Chmod(f.script, 0o644)
	o = f.opts()
	o.PR = ""
	if r := Run(context.Background(), o, f.env); status(t, r, "script.exec_bit") != Fail {
		t.Error("exec bit")
	}
}

func TestReceiptForgery(t *testing.T) {
	cases := map[string]map[string]any{
		"not passed":  {"passed": false},
		"bad check":   {"check_rc": 1},
		"bad lint":    {"lint_rc": 2},
		"problems":    {"problems": []any{"x"}},
		"other sha":   {"sha256": strings.Repeat("c", 64)},
		"other uid":   {"uid": 7},
		"other path":  {"script": "/elsewhere"},
		"string true": {"passed": "true"},
		"lint string": {"lint_rc": "1"},
		"lint null":   {"lint_rc": nil},
		"lint bool":   {"lint_rc": false},
		"lint object": {"lint_rc": map[string]any{}},
	}
	for name, over := range cases {
		f := newFixture(t)
		f.writeReceipt(over)
		o := f.opts()
		o.PR = ""
		if r := Run(context.Background(), o, f.env); status(t, r, "script.rehearsed") != Fail {
			t.Errorf("%s: want FAIL", name)
		}
	}
}

func TestReceiptOutsideState(t *testing.T) {
	f := newFixture(t)
	other := filepath.Join(f.dir, "other")
	_ = os.MkdirAll(other, 0o755)
	_ = os.WriteFile(filepath.Join(other, "receipt.json"), []byte(`{}`), 0o644)
	f.run.fn = func(a []string, e []string) (Result, error) {
		if a[0] == "handoff-rehearse" {
			return Result{Stdout: fmt.Sprintf("REHEARSED sha256=%s uid=1000 check_rc=0 log=%s\n", f.digest, filepath.Join(other, "check.log"))}, nil
		}
		return f.dispatch(a, e)
	}
	o := f.opts()
	o.PR = ""
	if r := Run(context.Background(), o, f.env); status(t, r, "script.rehearsed") != Fail {
		t.Error("receipt outside the state dir must fail")
	}
}

func TestRehearsedLineForOtherSha(t *testing.T) {
	f := newFixture(t)
	f.run.fn = func(a []string, e []string) (Result, error) {
		if a[0] == "handoff-rehearse" {
			return Result{Stdout: "REHEARSED sha256=" + strings.Repeat("d", 64) + " uid=1000 check_rc=0 log=/x/check.log\n"}, nil
		}
		return f.dispatch(a, e)
	}
	o := f.opts()
	o.PR = ""
	if r := Run(context.Background(), o, f.env); status(t, r, "script.rehearsed") != Fail {
		t.Error("line for another sha must fail")
	}
}

func TestStateDirIsPinnedAndAbsolute(t *testing.T) {
	f := newFixture(t)
	wantVar := func(name, want string) {
		t.Helper()
		found := false
		for i, c := range f.run.calls {
			if c[0] != "handoff-rehearse" {
				continue
			}
			for _, kv := range f.run.envs[i] {
				if strings.HasPrefix(kv, "HANDOFF_REHEARSE_STATE=") {
					if found || kv != "HANDOFF_REHEARSE_STATE="+want {
						t.Errorf("%s: got %s, want %s", name, kv, want)
					}
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s: no state variable passed", name)
		}
		f.run.calls, f.run.envs = nil, nil
	}
	reported := func(name string, r Report) {
		t.Helper()
		for _, it := range r.Items {
			if it.ID == "script.rehearsed" && it.Data["state_dir"] != f.state {
				t.Errorf("%s: reported state_dir %v, want %s", name, it.Data["state_dir"], f.state)
			}
		}
	}
	o := f.opts()
	o.PR = ""
	// inherited override
	f.env.Environ = func() []string { return []string{"HOME=" + f.dir, "HANDOFF_REHEARSE_STATE=" + f.state} }
	o.RehearseState = ""
	r := Run(context.Background(), o, f.env)
	wantVar("inherited", f.state)
	reported("inherited", r)
	if status(t, r, "script.rehearsed") != Pass {
		t.Error("inherited state should pass")
	}
	// explicit flag wins over the inherited value
	f.env.Environ = func() []string { return []string{"HOME=" + f.dir, "HANDOFF_REHEARSE_STATE=/attacker"} }
	o.RehearseState = f.state
	r = Run(context.Background(), o, f.env)
	wantVar("flag", f.state)
	reported("flag", r)
	if status(t, r, "script.rehearsed") != Pass {
		t.Error("flag state should pass")
	}
	// relative flag is made absolute
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	_ = os.Chdir(f.dir)
	o.RehearseState = "state"
	r = Run(context.Background(), o, f.env)
	wantVar("relative", f.state)
	reported("relative", r)
	// relative inherited override
	o.RehearseState = ""
	f.env.Environ = func() []string { return []string{"HOME=" + f.dir, "HANDOFF_REHEARSE_STATE=state"} }
	r = Run(context.Background(), o, f.env)
	wantVar("relative inherited", f.state)
	reported("relative inherited", r)
	if status(t, r, "script.rehearsed") != Pass {
		t.Errorf("relative inherited state should pass: %v", r.Items)
	}
	if status(t, r, "script.rehearsed") != Pass {
		t.Errorf("relative state should pass: %v", r.Items)
	}
}

func TestSymlinkAndSwap(t *testing.T) {
	f := newFixture(t)
	link := filepath.Join(f.dir, "link.sh")
	_ = os.Symlink(f.script, link)
	o := f.opts()
	o.Script = link
	o.PR = ""
	if r := Run(context.Background(), o, f.env); status(t, r, "script.path") != Fail {
		t.Error("symlinked script must fail")
	}
	// parent symlink
	real := filepath.Join(f.dir, "real")
	_ = os.MkdirAll(real, 0o755)
	_ = os.WriteFile(filepath.Join(real, "s.sh"), []byte("#!/bin/bash\n"), 0o755)
	_ = os.Symlink(real, filepath.Join(f.dir, "pl"))
	o.Script = filepath.Join(f.dir, "pl", "s.sh")
	if r := Run(context.Background(), o, f.env); status(t, r, "script.path") != Fail {
		t.Error("parent symlink must fail")
	}
	// swap during the run: change the script inside the rehearse call
	f = newFixture(t)
	f.run.fn = func(a []string, e []string) (Result, error) {
		if a[0] == "handoff-rehearse" {
			_ = os.WriteFile(f.script, []byte("#!/bin/bash\necho evil\n"), 0o755)
		}
		return f.dispatch(a, e)
	}
	o = f.opts()
	o.PR = ""
	r := Run(context.Background(), o, f.env)
	if status(t, r, "script.stable") != Fail || r.Overall != Fail || status(t, r, "script.lint") != Fail {
		t.Error("changed script must void every script item")
	}
}

func TestFIFOAndOversize(t *testing.T) {
	f := newFixture(t)
	fifo := filepath.Join(f.dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readRegular(fifo, 10); err == nil {
		t.Error("fifo must be refused")
	}
	big := filepath.Join(f.dir, "big")
	_ = os.WriteFile(big, make([]byte, 100), 0o644)
	if _, _, err := readRegular(big, 10); err == nil {
		t.Error("oversize must be refused")
	}
}

func TestPRCases(t *testing.T) {
	bad := `{"__typename":"CheckRun","name":"t","status":"COMPLETED","conclusion":"FAILURE"}`
	pending := `{"__typename":"CheckRun","name":"t","status":"IN_PROGRESS","conclusion":""}`
	weird := `{"__typename":"CheckRun","name":"t","status":"COMPLETED","conclusion":"MYSTERY"}`
	ctxOK := `{"__typename":"StatusContext","context":"c","state":"SUCCESS"}`
	ctxBad := `{"__typename":"StatusContext","context":"c","state":"FAILURE"}`
	dupBad := `{"__typename":"CheckRun","name":"ci","status":"COMPLETED","conclusion":"FAILURE"}`
	type tc struct {
		name    string
		first   string
		second  string
		req     []string
		post    bool
		check   string
		want    Status
		overall Status
	}
	cases := []tc{
		{"draft flip", ghJSON("OPEN", false, head1, okRun), ghJSON("OPEN", true, head1, okRun), nil, false, "pr.state", Fail, Fail},
		{"closed", ghJSON("OPEN", false, head1, okRun), ghJSON("CLOSED", false, head1, okRun), nil, false, "pr.state", Fail, Fail},
		{"merged expected", ghJSON("MERGED", false, head1, okRun), ghJSON("MERGED", false, head1, okRun), nil, true, "pr.state", Pass, Pass},
		{"head moved", ghJSON("OPEN", false, head1, okRun), ghJSON("OPEN", false, head2, okRun), nil, false, "pr.head", Fail, Fail},
		{"new failing check", ghJSON("OPEN", false, head1, okRun), ghJSON("OPEN", false, head1, okRun, bad), nil, false, "pr.checks", Fail, Fail},
		{"pending", ghJSON("OPEN", false, head1, okRun, pending), ghJSON("OPEN", false, head1, okRun, pending), nil, false, "pr.checks", Fail, Fail},
		{"unknown conclusion", ghJSON("OPEN", false, head1, weird), ghJSON("OPEN", false, head1, weird), nil, false, "pr.checks", Unknown, Unknown},
		{"required absent", ghJSON("OPEN", false, head1, okRun), ghJSON("OPEN", false, head1, okRun), []string{"other"}, false, "pr.checks", Fail, Fail},
		{"mixed ok", ghJSON("OPEN", false, head1, okRun, ctxOK), ghJSON("OPEN", false, head1, okRun, ctxOK), []string{"c"}, false, "pr.checks", Pass, Pass},
		{"mixed bad", ghJSON("OPEN", false, head1, okRun, ctxBad), ghJSON("OPEN", false, head1, okRun, ctxBad), nil, false, "pr.checks", Fail, Fail},
		{"duplicate names", ghJSON("OPEN", false, head1, okRun, dupBad), ghJSON("OPEN", false, head1, okRun, dupBad), nil, false, "pr.checks", Fail, Fail},
		{"no checks", ghJSON("OPEN", false, head1), ghJSON("OPEN", false, head1), nil, false, "pr.checks", Unknown, Unknown},
	}
	for _, c := range cases {
		f := newFixture(t)
		f.gh = []string{c.first, c.second}
		r := Run(context.Background(), Options{PR: prURL, RequireChecks: c.req, PostMerge: c.post}, f.env)
		if got := status(t, r, c.check); got != c.want {
			t.Errorf("%s: %s = %s want %s", c.name, c.check, got, c.want)
		}
		if c.want == Fail && r.Overall != Fail {
			t.Errorf("%s: overall %s", c.name, r.Overall)
		}
	}
	// gh failing and offline are UNKNOWN, never PASS
	f := newFixture(t)
	r := Run(context.Background(), Options{PR: prURL}, f.env)
	if status(t, r, "pr.head") != Unknown {
		t.Error("gh failure must be unknown")
	}
	r = Run(context.Background(), Options{PR: prURL, Offline: true}, f.env)
	if status(t, r, "pr.checks") != Unknown || r.Overall != Unknown {
		t.Error("offline must be unknown")
	}
	// bad URL
	r = Run(context.Background(), Options{PR: "https://example.com/o/r/pull/1"}, f.env)
	if status(t, r, "pr.url") != Fail {
		t.Error("bad url")
	}
}

func TestReviewCases(t *testing.T) {
	type tc struct {
		name         string
		lines        []string
		verdict, bnd Status
	}
	pr := "REVIEWED-PR: o/r#7"
	h1 := "REVIEWED-HEAD: " + head1
	h2 := "REVIEWED-HEAD: " + head2
	cases := []tc{
		{"ok", []string{pr, h1, "VERDICT: PASS"}, Pass, Pass},
		{"fail verdict", []string{pr, h1, "VERDICT: FAIL"}, Fail, Pass},
		{"no verdict", []string{pr, h1, "looks fine"}, Unknown, Pass},
		{"narrative only", []string{"I reviewed " + head1, "VERDICT: PASS"}, Pass, Unknown},
		{"older head", []string{pr, h2, "VERDICT: PASS"}, Pass, Fail},
		{"two differing", []string{pr, h1, h2, "VERDICT: PASS"}, Pass, Fail},
		{"other pr", []string{"REVIEWED-PR: o/r#8", h1, "VERDICT: PASS"}, Pass, Fail},
		{"short digest", []string{pr, "REVIEWED-HEAD: abc123", "VERDICT: PASS"}, Pass, Fail},
		{"verdict not last", []string{pr, h1, "VERDICT: PASS", "extra"}, Unknown, Pass},
	}
	for _, c := range cases {
		f := newFixture(t)
		f.gh = []string{ghJSON("OPEN", false, head1, okRun), ghJSON("OPEN", false, head1, okRun)}
		r := Run(context.Background(), Options{PR: prURL, Review: f.review(c.lines...)}, f.env)
		if status(t, r, "review.verdict") != c.verdict || status(t, r, "review.binds") != c.bnd {
			t.Errorf("%s: verdict %s binds %s", c.name, status(t, r, "review.verdict"), status(t, r, "review.binds"))
		}
	}
	// script sha binding
	f := newFixture(t)
	o := f.opts()
	o.PR = ""
	o.Review = f.review("REVIEWED-SCRIPT-SHA256: "+strings.Repeat("e", 64), "VERDICT: PASS")
	if r := Run(context.Background(), o, f.env); status(t, r, "review.binds") != Fail {
		t.Error("other script sha must fail")
	}
	o.Review = f.review("REVIEWED-SCRIPT-SHA256: "+f.digest, "VERDICT: PASS WITH NOTES")
	if r := Run(context.Background(), o, f.env); status(t, r, "review.binds") != Pass {
		t.Error("script sha binding")
	}
}

func TestNotesAndSidecar(t *testing.T) {
	f := newFixture(t)
	f.gh = []string{ghJSON("OPEN", false, head1, okRun), ghJSON("OPEN", false, head1, okRun)}
	var lines []string
	for i := 0; i < 25; i++ {
		lines = append(lines, fmt.Sprintf("NOTE: n%d", i))
	}
	lines = append(lines, "REVIEWED-PR: o/r#7", "REVIEWED-HEAD: "+head1, "REVIEWED-HEAD: "+head1, "VERDICT: PASS WITH NOTES")
	p := f.review(lines...)
	_ = os.WriteFile(p+".verdict", []byte("STATUS: warn\nNo verdict line\n"), 0o644)
	r := Run(context.Background(), Options{PR: prURL, Review: p}, f.env)
	var it Item
	for _, x := range r.Items {
		if x.ID == "review.verdict" {
			it = x
		}
	}
	notes, _ := it.Data["notes"].([]string)
	if it.Status != Pass || len(notes) != 20 || it.Data["notes_truncated"] != true {
		t.Errorf("notes: %v %v", it.Status, it.Data)
	}
	if !strings.Contains(it.Detail, ".verdict") {
		t.Errorf("sidecar not reported: %s", it.Detail)
	}
	if status(t, r, "review.binds") != Pass {
		t.Error("repeated identical binding lines are fine")
	}
}

func ledger(t *testing.T, lines ...string) (string, Env) {
	f := newFixture(t)
	p := filepath.Join(f.dir, "rulings.jsonl")
	_ = os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	return p, f.env
}

func TestRulings(t *testing.T) {
	good := `{"id":"q1","ts":"2026-10-14T01:02:03Z","by":"mk"}`
	newShape := `{"row_id":"r9","ruling_id":"q2","date":"2026-10-14","ruled_by":"vizier"}`
	cases := []struct {
		name   string
		lines  []string
		id     string
		want   Status
		shape  Status
		detail string
	}{
		{"found", []string{good}, "q1", Pass, Pass, ""},
		{"new shape", []string{good, newShape}, "q2", Pass, Pass, ""},
		{"absent", []string{good}, "q3", Fail, Fail, ""},
		{"identical dup", []string{good, good}, "q1", Pass, Pass, ""},
		{"conflicting dup", []string{good, `{"id":"q1","ts":"2026-10-15T00:00:00Z","by":"mk"}`}, "q1", Unknown, Unknown, "lines"},
		{"malformed line", []string{good, `not json`}, "q1", Unknown, Unknown, "malformed"},
		{"dup key", []string{`{"id":"q1","id":"q9","ts":"2026-10-14","by":"mk"}`}, "q9", Unknown, Unknown, "malformed"},
		{"escaped dup key", []string{`{"id":"q1","id":"q9"}`}, "q9", Unknown, Unknown, "malformed"},
		{"trailing garbage", []string{good + " garbage"}, "q1", Unknown, Unknown, "malformed"},
		{"null id beside match", []string{good, `{"id":null}`}, "q1", Unknown, Unknown, "malformed"},
		{"no id", []string{`{"ts":"2026-10-14"}`}, "q1", Unknown, Unknown, "malformed"},
		{"id null beside ruling_id", []string{`{"id":null,"ruling_id":"q1","ts":"2026-10-14","by":"mk"}`}, "q1", Unknown, Unknown, "malformed"},
		{"id int beside ruling_id", []string{`{"id":17,"ruling_id":"q1","ts":"2026-10-14","by":"mk"}`}, "q1", Unknown, Unknown, "malformed"},
		{"ruling_id null beside id", []string{`{"id":"q1","ruling_id":null,"ts":"2026-10-14","by":"mk"}`}, "q1", Unknown, Unknown, "malformed"},
		{"both ids same", []string{`{"id":"q1","ruling_id":"q1","ts":"2026-10-14","by":"mk"}`}, "q1", Pass, Pass, ""},
		{"both ids differ", []string{`{"id":"a","ruling_id":"b"}`}, "a", Unknown, Unknown, "malformed"},
		{"no by", []string{`{"id":"q1","ts":"2026-10-14"}`}, "q1", Pass, Fail, ""},
		{"bad date", []string{`{"id":"q1","ts":"yesterday","by":"mk"}`}, "q1", Pass, Fail, ""},
		{"huge int", []string{good, `{"id":"q5","n":123456789012345678901234567890,"ts":"2026-10-14","by":"mk"}`}, "q5", Pass, Pass, ""},
	}
	for _, c := range cases {
		p, env := ledger(t, c.lines...)
		r := Run(context.Background(), Options{Ruling: c.id, Rulings: p}, env)
		if status(t, r, "ruling.present") != c.want || status(t, r, "ruling.shape") != c.shape {
			t.Errorf("%s: present %s shape %s", c.name, status(t, r, "ruling.present"), status(t, r, "ruling.shape"))
		}
		if c.detail != "" && !strings.Contains(r.Items[0].Detail, c.detail) {
			t.Errorf("%s: detail %q", c.name, r.Items[0].Detail)
		}
	}
	// unreadable ledger
	f := newFixture(t)
	r := Run(context.Background(), Options{Ruling: "q1", Rulings: filepath.Join(f.dir, "none")}, f.env)
	if status(t, r, "ruling.present") != Unknown {
		t.Error("missing ledger must be unknown")
	}
	// default path from XDG_STATE_HOME
	_ = os.MkdirAll(filepath.Join(f.dir, "xdg", "vizier"), 0o755)
	_ = os.WriteFile(filepath.Join(f.dir, "xdg", "vizier", "rulings.jsonl"), []byte(good+"\n"), 0o644)
	r = Run(context.Background(), Options{Ruling: "q1"}, f.env)
	if status(t, r, "ruling.present") != Pass {
		t.Error("default ledger path")
	}
}

func TestValidateMatrix(t *testing.T) {
	d := strings.Repeat("a", 64)
	bad := []Options{
		{},
		{Script: "s"},
		{SHA: d},
		{Script: "s", SHA: "xyz"},
		{Review: "r"},
		{PostMerge: true, Ruling: "q1"},
		{RequireChecks: []string{"a"}, Ruling: "q1"},
		{RehearseState: "/x", Ruling: "q1"},
		{Rulings: "/x", PR: prURL},
		{Ruling: "bad id!"},
		{PR: prURL, RequireChecks: []string{"a\nb"}},
	}
	for i, o := range bad {
		if o.Validate() == nil {
			t.Errorf("case %d should be a usage error", i)
		}
	}
	good := []Options{{Ruling: "q1"}, {PR: prURL}, {Script: "s", SHA: d}, {Review: "r", PR: prURL}, {Review: "r", Script: "s", SHA: strings.ToUpper(d)}}
	for i, o := range good {
		if err := o.Validate(); err != nil {
			t.Errorf("good %d: %v", i, err)
		}
	}
}

func TestRollAndExit(t *testing.T) {
	if s, _ := roll([]Item{{Status: Pass}, {Status: Unknown}}); s != Unknown {
		t.Error("unknown")
	}
	if s, _ := roll([]Item{{Status: Unknown}, {Status: Fail}}); s != Fail {
		t.Error("fail")
	}
	if s, _ := roll(nil); s != Unknown {
		t.Error("empty")
	}
	if ExitCode(Pass) != 0 || ExitCode(Fail) != 1 || ExitCode(Unknown) != 3 {
		t.Error("exit codes")
	}
}

func TestExecRunner(t *testing.T) {
	r := ExecRunner{}
	res, err := r.Run(context.Background(), []string{"sh", "-c", "yes | head -c 300000; echo err >&2; exit 3"}, []string{"PATH=/usr/bin:/bin"})
	if err != nil || res.Code != 3 || !res.Truncated || len(res.Stdout) != cmdOutputCap {
		t.Errorf("flood: %+v %v", res.Code, err)
	}
	if _, err := r.Run(context.Background(), []string{"definitely-not-a-tool-xyz"}, nil); err == nil {
		t.Error("missing tool must error")
	}
	if _, err := r.Run(context.Background(), nil, nil); err == nil {
		t.Error("empty argv")
	}
}

func TestStubGhOnPath(t *testing.T) {
	dir := t.TempDir()
	stub := "#!/bin/sh\necho '" + ghJSON("OPEN", false, head1, okRun) + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	env := Env{Runner: ExecRunner{}, Now: time.Now, Environ: func() []string { return []string{"PATH=" + dir + ":/usr/bin:/bin"} }}
	r := Run(context.Background(), Options{PR: prURL}, env)
	if r.Overall != Pass || status(t, r, "pr.head") != Pass {
		b, _ := json.Marshal(r)
		t.Errorf("stub gh: %s", b)
	}
}

func TestFinalPRReadComesAfterEvidenceReads(t *testing.T) {
	f := newFixture(t)
	var events []string
	f.env.OnRead = func(p string) { events = append(events, "read:"+filepath.Base(p)) }
	f.run.fn = func(a []string, e []string) (Result, error) {
		if a[0] == "gh" {
			events = append(events, "gh")
		}
		return f.dispatch(a, e)
	}
	f.gh = []string{ghJSON("OPEN", false, head1, okRun), ghJSON("OPEN", false, head1, okRun)}
	led := filepath.Join(f.dir, "rulings.jsonl")
	_ = os.WriteFile(led, []byte(`{"id":"q1","ts":"2026-10-14","by":"mk"}`+"\n"), 0o644)
	rv := f.review("REVIEWED-PR: o/r#7", "REVIEWED-HEAD: "+head1, "VERDICT: PASS")
	Run(context.Background(), Options{PR: prURL, Review: rv, Ruling: "q1", Rulings: led}, f.env)
	last := -1
	for i, e := range events {
		if e == "gh" {
			last = i
		}
	}
	reads := 0
	for i, e := range events {
		if strings.HasPrefix(e, "read:") {
			reads++
			if i > last {
				t.Errorf("%s was read after the final PR read: %v", e, events)
			}
		}
	}
	if reads < 3 {
		t.Errorf("expected ledger, review and sidecar reads, got %v", events)
	}
}
