package orchcompat

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const moveSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func opt(id, label, kind string) map[string]any {
	return map[string]any{"id": id, "label": label, "kind": kind}
}

func nothingFiled(t *testing.T, e *env) {
	t.Helper()
	if n := len(e.st().Tasks); n != 0 {
		t.Errorf("%d task(s) exist, want none", n)
	}
	if got := e.callsWith("tasks", "create"); len(got) != 0 {
		t.Errorf("tasks create was called %d time(s), want 0", len(got))
	}
}

func TestAskValidationRejectsAtTheCLI(t *testing.T) {
	cases := []struct {
		name  string
		extra map[string]any
		more  []string
		err   string
	}{
		{"unknown field", map[string]any{"bogus": 1}, nil, `unknown field "bogus" in home-ask/v2`},
		{"v1-only field", map[string]any{"kind": "x"}, nil, `v1-only field "kind" in home-ask/v2`},
		{"schema of another version", map[string]any{"schema": "home-ask/v1"}, nil, "ask schema must be home-ask/v2"},
		{"pull is for threadless filing only", map[string]any{"pull": "mycroft"}, nil, "only Mycroft's threadless filing may set pull"},
		{"pull with another value", map[string]any{"pull": "x"}, nil, "only Mycroft's threadless filing may set pull"},
		{"auto_close that is not an object", map[string]any{"auto_close": "yes"}, nil, "auto_close must be an object"},
		{"auto_close without evidence", map[string]any{"auto_close": map[string]any{"option": "merged"}}, nil, "auto_close evidence must be an object"},
		{"auto_close with an unknown key", map[string]any{"auto_close": map[string]any{"option": "merged", "x": 1}}, nil, `unknown field "x" in auto_close`},
		{"auto_close option other than merged", map[string]any{"auto_close": map[string]any{"option": "closed", "evidence": map[string]any{}}}, nil, `auto_close option must be "merged"`},
		{"missing question", map[string]any{"question": nil}, nil, "question must be 1-8000 characters (got 0)"},
		{"blank question", map[string]any{"question": "  "}, nil, "question must be 1-8000 characters"},
		{"missing options", map[string]any{"options": nil}, nil, "decide needs 2-6 options"},
		{"a single option", map[string]any{"options": []any{opt("a", "A", "ruling-only")}}, nil, "decide needs 2-6 options"},
		{"duplicate option id", map[string]any{"options": []any{opt("a", "A", "ruling-only"), opt("a", "B", "ruling-only")}}, nil, `duplicate option id "a"`},
		{"unknown option kind", map[string]any{"options": []any{opt("a", "A", "zzz"), opt("b", "B", "ruling-only")}}, nil, "option kind must be instruction, needs-context or ruling-only"},
		{"option id with uppercase and a space", map[string]any{"options": []any{opt("A B", "A", "ruling-only"), opt("b", "B", "ruling-only")}}, nil, `options[0].id "A B" must be lowercase`},
		{"malformed Blocks ref", nil, []string{"--blocks", "zzz"}, `invalid Blocks ref "zzz"`},
		{"project_root that serve does not list", map[string]any{"project_root": "/nonexistent"}, nil, "is not a project root serve resolves"},
		{"project that is not the bound one", map[string]any{"project": "zzz"}, nil, `ask project "zzz" is not the Home project bound to tasks project "P1"; expected project "alpha" (the tasks key "P1" is also accepted)`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			ask := e.writeFile("ask.json", askJSON(c.extra))
			r := e.autarch(fileArgs(ask, c.more...)...)
			wantCode(t, r, 2, c.err)
			if r.Stdout != "" {
				t.Errorf("stdout = %q, want empty", r.Stdout)
			}
			nothingFiled(t, e)
		})
	}
}

func TestAskValidationAccepts(t *testing.T) {
	cases := []struct {
		name  string
		extra map[string]any
		more  []string
	}{
		{"schema home-ask/v2 stated in the file", map[string]any{"schema": "home-ask/v2"}, nil},
		{"project left out is filled from the binding", map[string]any{"project": nil}, nil},
		{"the tasks key as the ask project", map[string]any{"project": "P1"}, nil},
		{"an arbitrary but well-formed --request key", nil, []string{"--request", "nope"}},
		{"Blocks refs", nil, []string{"--blocks", "bead:mk-1", "--blocks", "thread:thr_x", "--blocks", "project:alpha"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			ask := e.writeFile("ask.json", askJSON(c.extra))
			r := e.autarch(fileArgs(ask, c.more...)...)
			wantCode(t, r, 0)
			if len(e.st().Tasks) != 1 {
				t.Fatalf("tasks = %d, want 1", len(e.st().Tasks))
			}
			var back map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(r.Stdout)), &back); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProjectAliasIsRewrittenToTheBoundName(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(map[string]any{"project": "P1"}))
	r := e.autarch(fileArgs(ask)...)
	wantCode(t, r, 0)
	if r.Stderr != "" {
		t.Errorf("stderr = %q, want empty", r.Stderr)
	}
	if d := e.st().Tasks[0].Desc; !strings.Contains(d, `"project":"alpha"`) || strings.Contains(d, `"project":"P1"`) {
		t.Errorf("the card carries the alias, want the bound name:\n%s", d)
	}
}

func TestUsageErrors(t *testing.T) {
	cases := []struct {
		name   string
		thread string
		args   func(e *env) []string
		err    string
	}{
		{"no thread", "", func(e *env) []string { return fileArgs(e.writeFile("a.json", askJSON(nil))) }, "BB_THREAD_ID is required: run inside the asking bb thread"},
		{"no flags", "thr_a", func(e *env) []string { return []string{"needs-mk", "file"} },
			"--project, --title and --ask-file are required (--ask-file may be left out with --move)"},
		{"no project", "thr_a", func(e *env) []string {
			return []string{"needs-mk", "file", "--title", "x", "--ask-file", e.writeFile("a.json", askJSON(nil))}
		}, "--project, --title and --ask-file are required"},
		{"ask file missing", "thr_a", func(e *env) []string { return fileArgs("/nonexistent/ask.json") }, "--ask-file: open /nonexistent/ask.json: no such file or directory"},
		{"ask file over 1 MiB", "thr_a", func(e *env) []string {
			return fileArgs(e.writeFile("big.json", `{"question":"`+strings.Repeat("a", 1<<20+16)+`"}`))
		}, "--ask-file is over 1 MiB"},
		{"ask file is null", "thr_a", func(e *env) []string { return fileArgs(e.writeFile("n.json", "null")) }, "--ask-file is not a JSON object: got null"},
		{"ask file is not JSON", "thr_a", func(e *env) []string { return fileArgs(e.writeFile("n.json", "{{")) }, "--ask-file is not a JSON object: invalid character"},
		{"ask file is an array", "thr_a", func(e *env) []string { return fileArgs(e.writeFile("n.json", "[]")) }, "--ask-file is not a JSON object: json: cannot unmarshal array"},
		{"empty --request", "thr_a", func(e *env) []string { return fileArgs(e.writeFile("a.json", askJSON(nil)), "--request", "") },
			`--request: invalid ask: Request line missing: the Request key "" must match ^[A-Za-z0-9:_.-]{1,128}$`},
		{"--request with a space", "thr_a", func(e *env) []string { return fileArgs(e.writeFile("a.json", askJSON(nil)), "--request", "a b") },
			`the Request key "a b" must match`},
		{"--root-run without key=value", "thr_a", func(e *env) []string { return fileArgs(e.writeFile("a.json", askJSON(nil)), "--root-run", "x") },
			"--root-run takes script=<absolute path>,timeout=<seconds>,set=<name>"},
		{"--root-run repeats a key", "thr_a", func(e *env) []string {
			return fileArgs(e.writeFile("a.json", askJSON(nil)), "--root-run", "script=/a,script=/b")
		}, "--root-run repeats script"},
		{"--root-run lacks set", "thr_a", func(e *env) []string {
			return fileArgs(e.writeFile("a.json", askJSON(nil)), "--root-run", "script=/a,timeout=5")
		}, "--root-run needs set="},
		{"--root-run timeout not a number", "thr_a", func(e *env) []string {
			return fileArgs(e.writeFile("a.json", askJSON(nil)), "--root-run", "script=/a,timeout=x,set=s")
		}, "--root-run timeout must be a number of seconds"},
		{"--root-run script relative", "thr_a", func(e *env) []string {
			return fileArgs(e.writeFile("a.json", askJSON(nil)), "--root-run", "script=a.sh,timeout=5,set=s")
		}, "--root-run script must be an absolute path"},
		{"--root-run script unreadable", "thr_a", func(e *env) []string {
			return fileArgs(e.writeFile("a.json", askJSON(nil)), "--root-run", "script=/nonexistent/s.sh,timeout=5,set=s")
		}, "--root-run script is not readable: open /nonexistent/s.sh"},
		{"--root-run set with a leading dash", "thr_a", func(e *env) []string {
			s := e.writeFile("s.sh", "echo hi\n")
			return fileArgs(e.writeFile("a.json", askJSON(nil)), "--root-run", "script="+s+",timeout=5,set=-x")
		}, "set must match ^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$"},
		{"--root-run timeout zero", "thr_a", func(e *env) []string {
			s := e.writeFile("s.sh", "echo hi\n")
			return fileArgs(e.writeFile("a.json", askJSON(nil)), "--root-run", "script="+s+",timeout=0,set=s")
		}, "timeout must be 1-3600 seconds"},
		{"--root-run timeout over an hour", "thr_a", func(e *env) []string {
			s := e.writeFile("s.sh", "echo hi\n")
			return fileArgs(e.writeFile("a.json", askJSON(nil)), "--root-run", "script="+s+",timeout=3601,set=s")
		}, "timeout must be 1-3600 seconds"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.thread = c.thread
			r := e.autarch(c.args(e)...)
			wantCode(t, r, 2, c.err)
			if r.Stdout != "" {
				t.Errorf("stdout = %q, want empty", r.Stdout)
			}
			nothingFiled(t, e)
		})
	}
}

func TestUsageErrorsMakeNoBBCallBeforeTheChecksPass(t *testing.T) {
	e := newEnv(t)
	e.thread = ""
	wantCode(t, e.autarch(fileArgs(e.writeFile("a.json", askJSON(nil)))...), 2)
	if n := len(e.calls()); n != 0 {
		t.Errorf("%d bb call(s), want 0", n)
	}
	e2 := newEnv(t)
	wantCode(t, e2.autarch(fileArgs(e2.writeFile("a.json", "null"))...), 2)
	if n := len(e2.calls()); n != 0 {
		t.Errorf("%d bb call(s), want 0", n)
	}
}

func TestOwnerThread(t *testing.T) {
	setup := func(t *testing.T) (*env, string) {
		e := newEnv(t)
		e.update(func(s *fbState) {
			s.Threads["thr_live"] = "live"
			s.Threads["thr_arch"] = "archived"
			s.Threads["thr_del"] = "deleted"
		})
		return e, e.writeFile("ask.json", askJSON(nil))
	}
	t.Run("a live owner receives the routing comment", func(t *testing.T) {
		e, ask := setup(t)
		r := e.autarch(fileArgs(ask, "--owner-thread", "thr_live")...)
		wantCode(t, r, 0)
		create := e.callsWith("tasks", "create")
		comment := e.callsWith("tasks", "comment")
		if len(create) != 1 || create[0].Thread != "thr_a" {
			t.Errorf("tasks create calls = %+v, want one as thr_a", create)
		}
		if len(comment) != 1 || comment[0].Thread != "thr_live" {
			t.Fatalf("tasks comment calls = %+v, want one as thr_live", comment)
		}
		if want := "Asked from this thread (Request " + parseFiled(t, r).Request + ")."; comment[0].Body != want {
			t.Errorf("routing comment body = %q, want %q", comment[0].Body, want)
		}
		cs := e.st().Tasks[0].Comments
		if len(cs) != 1 || cs[0].Kind != "agent" || cs[0].ThreadID != "thr_live" {
			t.Errorf("card comments = %+v, want one agent comment from thr_live", cs)
		}
	})
	refusals := []struct {
		name, owner, err string
	}{
		{"an unknown owner", "thr_zz", "--owner-thread thr_zz is not a thread"},
		{"an archived owner", "thr_arch", "--owner-thread thr_arch is archived or deleted; name a live thread"},
		{"a deleted owner", "thr_del", "--owner-thread thr_del is archived or deleted; name a live thread"},
	}
	for _, c := range refusals {
		t.Run(c.name+" is refused", func(t *testing.T) {
			e, ask := setup(t)
			r := e.autarch(fileArgs(ask, "--owner-thread", c.owner)...)
			wantCode(t, r, 2, c.err)
			nothingFiled(t, e)
		})
	}
	t.Run("a failing thread lookup is Home down", func(t *testing.T) {
		e, ask := setup(t)
		e.update(func(s *fbState) { s.Fail = []fbFail{{Prefix: "thread show", Code: 1, Stderr: "boom\n", Times: -1}} })
		r := e.autarch(fileArgs(ask, "--owner-thread", "thr_x")...)
		wantCode(t, r, 3, "home is down, nothing filed: bb thread show thr_x: boom")
		nothingFiled(t, e)
	})
	t.Run("the owner is part of neither the identity nor the key", func(t *testing.T) {
		a, askA := setup(t)
		b, askB := setup(t)
		ka := parseFiled(t, a.autarch(fileArgs(askA, "--owner-thread", "thr_live")...)).Request
		kb := parseFiled(t, b.autarch(fileArgs(askB)...)).Request
		if ka != kb {
			t.Errorf("--owner-thread changed the derived key: %s vs %s", ka, kb)
		}
	})
}

func TestFilingPreconditions(t *testing.T) {
	t.Run("serve not running", func(t *testing.T) {
		e := newEnv(t)
		addr := closedLoopbackAddr(t)
		e.vars["AUTARCH_SERVE_URL"] = "http://" + addr
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(nil)))...)
		wantCode(t, r, 3, "home is down, nothing filed: autarch serve is not running ("+addr+")", "start `autarch serve` (see AGENTS.md): nothing was filed")
		nothingFiled(t, e)
	})
	t.Run("token file missing", func(t *testing.T) {
		e := newEnv(t)
		e.vars["AUTARCH_SERVE_TOKEN_FILE"] = e.root + "/nope"
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(nil)))...)
		wantCode(t, r, 3, "home is down, nothing filed: serve token: stat ", "no such file or directory: nothing was filed")
		nothingFiled(t, e)
	})
	t.Run("token file readable by others", func(t *testing.T) {
		e := newEnv(t)
		tok := e.writeFile("tok", "wrong\n")
		e.vars["AUTARCH_SERVE_TOKEN_FILE"] = tok
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(nil)))...)
		wantCode(t, r, 3, "has mode 0644; refusing (want 0600)")
		nothingFiled(t, e)
	})
	t.Run("token that serve does not accept", func(t *testing.T) {
		e := newEnv(t)
		tok := e.writeFile("tok", "wrong\n")
		if err := os.Chmod(tok, 0o600); err != nil {
			t.Fatal(err)
		}
		e.vars["AUTARCH_SERVE_TOKEN_FILE"] = tok
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(nil)))...)
		if r.Code != 3 || !strings.Contains(r.Stderr, "nothing was filed") {
			t.Fatalf("code=%d stderr=%q, want exit 3 and 'nothing was filed'", r.Code, r.Stderr)
		}
		nothingFiled(t, e)
	})
	t.Run("a binding to a name that differs from the project the root belongs to", func(t *testing.T) {
		e := newEnv(t)
		e.update(func(s *fbState) { s.Bindings["P1"] = fbBinding{Home: "ghost", State: "confirmed"} })
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(map[string]any{"project": "alpha"})))...)
		wantCode(t, r, 2, `ask project "alpha" is not the Home project bound to tasks project "P1"; expected project "ghost"`)
		nothingFiled(t, e)
	})
	t.Run("a binding that is not confirmed does not name the project", func(t *testing.T) {
		e := newEnv(t)
		e.update(func(s *fbState) { s.Bindings["P1"] = fbBinding{Home: "alpha", State: "unconfirmed"} })
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(map[string]any{"project": "P1"})))...)
		wantCode(t, r, 2, `warning: tasks project "P1" has no confirmed Home project binding`, `project "P1" is not a project serve lists`)
		nothingFiled(t, e)
	})
}

func TestWarnings(t *testing.T) {
	const cmdWarn = "warning: the question mentions scp, ssh or bash but no command stands on its own line, so Home may show no copy button for it; put each command on its own line\n"
	const unbound = "warning: tasks project \"P1\" has no confirmed Home project binding; the card will show in Home flagged \"project not bound\" until the vizier runs `bb home bind` or mk clicks Bind\n"
	t.Run("a command inside a sentence", func(t *testing.T) {
		e := newEnv(t)
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(map[string]any{"question": "please run ssh host now"})))...)
		wantCode(t, r, 0)
		if r.Stderr != cmdWarn {
			t.Errorf("stderr = %q, want %q", r.Stderr, cmdWarn)
		}
	})
	t.Run("a command on its own line draws no warning", func(t *testing.T) {
		e := newEnv(t)
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(map[string]any{"question": "Run this:\nbash /opt/run.sh"})))...)
		wantCode(t, r, 0)
		if r.Stderr != "" {
			t.Errorf("stderr = %q, want empty", r.Stderr)
		}
	})
	t.Run("prose that merely mentions bash draws no warning", func(t *testing.T) {
		e := newEnv(t)
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(map[string]any{"question": "the bash scripts are fine"})))...)
		wantCode(t, r, 0)
		if r.Stderr != "" {
			t.Errorf("stderr = %q, want empty", r.Stderr)
		}
	})
	t.Run("no binding", func(t *testing.T) {
		e := newEnv(t)
		e.update(func(s *fbState) { s.Bindings = map[string]fbBinding{} })
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(nil)))...)
		wantCode(t, r, 0)
		if r.Stderr != unbound {
			t.Errorf("stderr = %q, want %q", r.Stderr, unbound)
		}
	})
	t.Run("a bb without the binding command", func(t *testing.T) {
		e := newEnv(t)
		e.update(func(s *fbState) { s.NoBinding = true })
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(nil)))...)
		wantCode(t, r, 0)
		t.Logf("stderr = %q", r.Stderr)
	})
	t.Run("the card key is not indexed within five seconds", func(t *testing.T) {
		e := newEnv(t)
		e.update(func(s *fbState) { s.CardKeyAfter = -1 })
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(nil)))...)
		// Diagnose the readback contract independently of the command's result.
		// Both Tasks creation and Home readback must carry the canonical Tasks id.
		st := e.st()
		if len(st.Tasks) != 1 || st.Tasks[0].Project != "P1" {
			t.Fatalf("created tasks = %+v, want one in P1", st.Tasks)
		}
		code, raw, stderr := fbDispatch(st, []string{"home", "get", "--card", st.Tasks[0].ID, "--json"}, &fbCall{})
		var view struct {
			Project string `json:"project_id"`
			Key     string `json:"card_key"`
		}
		if code != 0 || json.Unmarshal([]byte(raw), &view) != nil || view.Project != st.Tasks[0].Project || view.Key != "" {
			t.Fatalf("readback exit=%d stdout=%q stderr=%q; want matching project and deliberately missing key", code, raw, stderr)
		}
		t.Logf("created project=%q readback project=%q key=%q CardKeyAfter=%d", st.Tasks[0].Project, view.Project, view.Key, st.CardKeyAfter)
		wantCode(t, r, 0)
		want := "warning: card T1 is filed but Home has not indexed it yet (card_key not indexed within 5s); read it with `bb home get --card T1 --json`\n"
		if r.Stderr != want {
			t.Errorf("stderr = %q, want %q", r.Stderr, want)
		}
		got := parseFiled(t, r)
		if got.CardKey != "" || got.Card != "T1" {
			t.Errorf("filed = %+v, want card T1 without a card_key", got)
		}
		if strings.Contains(r.Stdout, "card_key") {
			t.Errorf("stdout %q carries card_key, want it omitted", r.Stdout)
		}
	})
}

func TestExistingNeedsMkLabelIsNotCreatedAgain(t *testing.T) {
	e := newEnv(t)
	e.update(func(s *fbState) {
		s.Labels["needs-mk"] = true
	})
	r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(nil)))...)
	wantCode(t, r, 0)
	if r.Stderr != "" {
		t.Errorf("stderr = %q, want empty", r.Stderr)
	}
	if got := e.callsWith("tasks", "label", "create"); len(got) != 0 {
		t.Errorf("label create calls = %d, want 0", len(got))
	}
	if got := e.st().Tasks[0].Labels; len(got) != 1 || got[0] != "needs-mk" {
		t.Errorf("labels on the card = %v, want [needs-mk]", got)
	}
}

func moveFile(e *env, body string) string { return e.writeFile("move.json", body) }

func scriptMove() string {
	return `{"schema":"home-move/v1","kind":"script","script":{"path":"/opt/run.sh","sha256":"` + moveSHA + `","args":["--check","x=1"]}}`
}

func TestMoveFiling(t *testing.T) {
	cases := []struct {
		name     string
		move     string
		detail   string
		stderr   string
		moveJSON string
	}{
		{"script", scriptMove(), "Run: bash /opt/run.sh --check x=1\nsha256: " + moveSHA,
			"warning: the question mentions scp, ssh or bash but no command stands on its own line, so Home may show no copy button for it; put each command on its own line\n",
			`{"kind":"script","schema":"home-move/v1","script":{"args":["--check","x=1"],"path":"/opt/run.sh","sha256":"` + moveSHA + `"}}`},
		{"pr", `{"schema":"home-move/v1","kind":"pr","pr":{"url":"https://github.com/o/r/pull/3"}}`, "https://github.com/o/r/pull/3", "",
			`{"kind":"pr","pr":{"url":"https://github.com/o/r/pull/3"},"schema":"home-move/v1"}`},
		{"read", `{"schema":"home-move/v1","kind":"read","read":{"url":"https://github.com/o/r"}}`, "https://github.com/o/r", "",
			`{"kind":"read","read":{"url":"https://github.com/o/r"},"schema":"home-move/v1"}`},
		{"context", `{"schema":"home-move/v1","kind":"context","context":{"need":"the key"}}`, "the key", "",
			`{"context":{"need":"the key"},"kind":"context","schema":"home-move/v1"}`},
	}
	for _, c := range cases {
		t.Run(c.name+" move alone", func(t *testing.T) {
			e := newEnv(t)
			e.cwd = shared.alpha
			r := e.autarch("needs-mk", "file", "--project", "P1", "--title", "Do the thing", "--move", moveFile(e, c.move))
			wantCode(t, r, 0)
			if r.Stderr != c.stderr {
				t.Errorf("stderr = %q, want %q", r.Stderr, c.stderr)
			}
			ask := map[string]any{
				"question":     "Do the thing\n\n" + c.detail,
				"options":      []any{opt("done", "Done", "ruling-only"), opt("not-now", "Not now", "ruling-only")},
				"project":      "alpha",
				"project_root": shared.alpha,
			}
			var mv map[string]any
			if err := json.Unmarshal([]byte(c.move), &mv); err != nil {
				t.Fatal(err)
			}
			o := oracleReq{project: "alpha", title: "Do the thing", ask: ask, move: mv}
			ident := oracleIdent(t, o)
			key := oracleKey("thr_a", ident)
			if got := parseFiled(t, r).Request; got != key {
				t.Errorf("request = %s, want %s", got, key)
			}
			tail := "\n```home-move\n" + c.moveJSON + "\n```\n"
			if d := e.st().Tasks[0].Desc; d != oracleDesc(o, key, ident, tail) {
				t.Errorf("description:\n%s\nwant:\n%s", d, oracleDesc(o, key, ident, tail))
			}
			if got := e.st().Tasks[0].Labels; len(got) != 2 || got[0] != "needs-mk" || got[1] != "mk-move" {
				t.Errorf("labels = %v, want [needs-mk mk-move]", got)
			}
			if got := e.callsWith("tasks", "label", "create"); len(got) != 2 {
				t.Errorf("label create calls = %d, want 2", len(got))
			}
		})
	}

	t.Run("a move with an ask file keeps the ask", func(t *testing.T) {
		e := newEnv(t)
		mv := `{"schema":"home-move/v1","kind":"context","context":{"need":"the key"}}`
		r := e.autarch(fileArgs(e.writeFile("a.json", askJSON(nil)), "--move", moveFile(e, mv))...)
		wantCode(t, r, 0)
		d := e.st().Tasks[0].Desc
		if !strings.HasPrefix(d, "Ship it?\n\nRequest: ") || !strings.Contains(d, "```home-move\n"+`{"context":{"need":"the key"},"kind":"context","schema":"home-move/v1"}`+"\n```\n") {
			t.Errorf("description:\n%s", d)
		}
		if got := e.st().Tasks[0].Labels; len(got) != 2 || got[1] != "mk-move" {
			t.Errorf("labels = %v, want needs-mk and mk-move", got)
		}
	})

	// With no project in the ask, the card takes the Home project its board is bound to, not the working
	// directory's folder name; the working directory is then refused by the root check, as it is not a root serve lists.
	t.Run("a move alone takes the project from the board binding", func(t *testing.T) {
		e := newEnv(t)
		r := e.autarch("needs-mk", "file", "--project", "P1", "--title", "T", "--move", moveFile(e, `{"schema":"home-move/v1","kind":"context","context":{"need":"x"}}`))
		wantCode(t, r, 2, `is not a project root serve resolves`)
		if strings.Contains(r.Stderr, "is not the Home project bound") {
			t.Errorf("the project was not taken from the binding: %q", r.Stderr)
		}
		nothingFiled(t, e)
	})

	bad := []struct{ name, body, err string }{
		{"another schema version", `{"schema":"home-move/v2","kind":"context"}`, "--move is not a valid home-move/v1: schema must be home-move/v1"},
		{"unknown kind", `{"schema":"home-move/v1","kind":"zzz"}`, "kind must be script, pr, read or context"},
		{"not an object", `[]`, "home-move must be a JSON object"},
		{"relative script path", `{"schema":"home-move/v1","kind":"script","script":{"path":"rel","sha256":"x"}}`,
			"script path must be an absolute path of letters, digits and ._/+- with no .. segment"},
		{"script without a good sha256", `{"schema":"home-move/v1","kind":"script","script":{"path":"/opt/run.sh","sha256":"x"}}`, "script sha256 must be 64 lowercase hex characters"},
		{"pr url that is not a pull request", `{"schema":"home-move/v1","kind":"pr","pr":{"url":"https://github.com/o/r"}}`, "pr url must be https://github.com/<owner>/<repo>/pull/<n>"},
		{"read url on another host", `{"schema":"home-move/v1","kind":"read","read":{"url":"https://example.com/a"}}`, "read url must be https on an allowed host"},
		{"missing member", `{"schema":"home-move/v1","kind":"context"}`, `context move needs a "context" member`},
		{"extra member", `{"schema":"home-move/v1","kind":"context","context":{"need":"x"},"extra":1}`, `unknown field "extra" in home-move/v1`},
	}
	for _, c := range bad {
		t.Run("refused: "+c.name, func(t *testing.T) {
			e := newEnv(t)
			e.cwd = shared.alpha
			r := e.autarch("needs-mk", "file", "--project", "P1", "--title", "T", "--move", moveFile(e, c.body))
			wantCode(t, r, 2, c.err)
			nothingFiled(t, e)
		})
	}
	t.Run("refused: file missing", func(t *testing.T) {
		e := newEnv(t)
		r := e.autarch("needs-mk", "file", "--project", "P1", "--title", "T", "--move", "/nonexistent/m.json")
		wantCode(t, r, 2, "--move: open /nonexistent/m.json: no such file or directory")
	})
	t.Run("refused: file over 1 MiB", func(t *testing.T) {
		e := newEnv(t)
		r := e.autarch("needs-mk", "file", "--project", "P1", "--title", "T", "--move", moveFile(e, strings.Repeat(" ", 1<<20+16)))
		wantCode(t, r, 2, "--move is over 1 MiB")
	})
}

func TestAdoptMove(t *testing.T) {
	const ctxMove = `{"schema":"home-move/v1","kind":"context","context":{"need":"the key"}}`
	filedCard := func(t *testing.T) (*env, string) {
		e := newEnv(t)
		wantCode(t, e.autarch(fileArgs(e.writeFile("a.json", askJSON(nil)))...), 0)
		return e, moveFile(e, ctxMove)
	}
	t.Run("adds the block and the label to a card this thread filed", func(t *testing.T) {
		e, mv := filedCard(t)
		before := e.st().Tasks[0].Desc
		r := e.autarch("needs-mk", "adopt-move", "--card", "T1", "--move", mv)
		wantCode(t, r, 0)
		if r.Stdout != `{"card":"T1","replay":false}`+"\n" || r.Stderr != "" {
			t.Errorf("stdout=%q stderr=%q", r.Stdout, r.Stderr)
		}
		tk := e.st().Tasks[0]
		want := strings.TrimRight(before, "\r\n") + "\n\n```home-move\n" + `{"context":{"need":"the key"},"kind":"context","schema":"home-move/v1"}` + "\n```\n"
		if tk.Desc != want {
			t.Errorf("description:\n%s\nwant:\n%s", tk.Desc, want)
		}
		if len(tk.Labels) != 2 || tk.Labels[1] != "mk-move" {
			t.Errorf("labels = %v, want needs-mk and mk-move", tk.Labels)
		}
		upd := e.callsWith("tasks", "update")
		if len(upd) != 1 || upd[0].Thread != "thr_a" {
			t.Errorf("tasks update calls = %+v, want one as thr_a", upd)
		}
	})
	t.Run("adopting the same move again changes nothing", func(t *testing.T) {
		e, mv := filedCard(t)
		wantCode(t, e.autarch("needs-mk", "adopt-move", "--card", "T1", "--move", mv), 0)
		desc := e.st().Tasks[0].Desc
		r := e.autarch("needs-mk", "adopt-move", "--card", "T1", "--move", mv)
		wantCode(t, r, 0)
		if r.Stdout != `{"card":"T1","replay":true}`+"\n" {
			t.Errorf("stdout = %q", r.Stdout)
		}
		if e.st().Tasks[0].Desc != desc || len(e.callsWith("tasks", "update")) != 1 {
			t.Error("a replay changed the card or called tasks update again")
		}
	})
	t.Run("a different move is refused", func(t *testing.T) {
		e, mv := filedCard(t)
		wantCode(t, e.autarch("needs-mk", "adopt-move", "--card", "T1", "--move", mv), 0)
		other := e.writeFile("other.json", `{"schema":"home-move/v1","kind":"context","context":{"need":"other"}}`)
		wantCode(t, e.autarch("needs-mk", "adopt-move", "--card", "T1", "--move", other), 2, "card T1 already carries a different move")
	})
	t.Run("another thread may not adopt", func(t *testing.T) {
		e, mv := filedCard(t)
		e.thread = "thr_b"
		wantCode(t, e.autarch("needs-mk", "adopt-move", "--card", "T1", "--move", mv), 2, `card T1 was filed from "thr_a", not this thread`)
		if len(e.callsWith("tasks", "update")) != 0 {
			t.Error("tasks update was called")
		}
	})
	t.Run("a card that does not parse is refused", func(t *testing.T) {
		e, mv := filedCard(t)
		e.update(func(s *fbState) { s.Tasks[0].Desc = "free text, no ask" })
		wantCode(t, e.autarch("needs-mk", "adopt-move", "--card", "T1", "--move", mv), 2, "card T1 does not parse as a Home card", "nothing was changed")
	})
	t.Run("usage errors", func(t *testing.T) {
		e, mv := filedCard(t)
		wantCode(t, e.autarch("needs-mk", "adopt-move"), 2, "--card and --move are required")
		wantCode(t, e.autarch("needs-mk", "adopt-move", "--card", "T1"), 2, "--card and --move are required")
		wantCode(t, e.autarch("needs-mk", "adopt-move", "--move", mv), 2, "--card and --move are required")
		wantCode(t, e.autarch("needs-mk", "adopt-move", "--card", "T1", "--move", e.writeFile("bad.json", "[]")), 2, "--move is not a valid home-move/v1")
		e.thread = ""
		wantCode(t, e.autarch("needs-mk", "adopt-move", "--card", "T1", "--move", mv), 2, "BB_THREAD_ID is required: run inside the asking bb thread")
	})
	t.Run("a card that does not exist", func(t *testing.T) {
		e, mv := filedCard(t)
		r := e.autarch("needs-mk", "adopt-move", "--card", "T99", "--move", mv)
		wantCode(t, r, 1, "task not found")
	})
}
