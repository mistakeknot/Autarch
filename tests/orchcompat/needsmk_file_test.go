package orchcompat

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fileArgs(ask string, more ...string) []string {
	return append([]string{"needs-mk", "file", "--project", "P1", "--title", "Ship it?", "--ask-file", ask}, more...)
}

func wantCode(t *testing.T, r result, code int, errHas ...string) {
	t.Helper()
	if r.Code != code {
		t.Fatalf("exit code %d, want %d\nstdout=%q\nstderr=%q", r.Code, code, r.Stdout, r.Stderr)
	}
	for _, s := range errHas {
		if !strings.Contains(r.Stderr, s) {
			t.Errorf("stderr %q lacks %q", r.Stderr, s)
		}
	}
}

func TestFileHappyPath(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	r := e.autarch(fileArgs(ask)...)
	wantCode(t, r, 0)
	if r.Stderr != "" {
		t.Errorf("stderr = %q, want empty", r.Stderr)
	}
	o := oracleReq{project: "alpha", title: "Ship it?", ask: askMap(t, nil)}
	ident := oracleIdent(t, o)
	key := oracleKey("thr_a", ident)
	got := parseFiled(t, r)
	want := filed{Card: "T1", Request: key, Project: "P1", Replay: false, CardKey: "CK-T1"}
	if got != want {
		t.Fatalf("filed = %+v, want %+v", got, want)
	}
	if strings.Count(strings.TrimSpace(r.Stdout), "\n") != 0 || !strings.HasSuffix(r.Stdout, "\n") {
		t.Errorf("stdout is not exactly one JSON line: %q", r.Stdout)
	}
	seq := heads(e.calls(), 3)
	wantSeq := []string{"tasks project list", "home binding P1", "tasks label list", "tasks label create", "home get --request",
		"tasks list --status", "tasks create --project", "tasks comment T1", "home get --card"}
	if !reflect.DeepEqual(seq, wantSeq) {
		t.Fatalf("bb calls:\n got %q\nwant %q", seq, wantSeq)
	}
	st := e.st()
	if len(st.Tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(st.Tasks))
	}
	tk := st.Tasks[0]
	if tk.Desc != oracleDesc(o, key, ident, "") {
		t.Errorf("description:\n%s\nwant:\n%s", tk.Desc, oracleDesc(o, key, ident, ""))
	}
	if tk.Title != "Ship it?" || tk.Project != "P1" || !reflect.DeepEqual(tk.Labels, []string{"needs-mk"}) {
		t.Errorf("task = title %q project %q labels %v", tk.Title, tk.Project, tk.Labels)
	}
	if len(tk.Comments) != 1 || tk.Comments[0].Kind != "agent" || tk.Comments[0].ThreadID != "thr_a" ||
		tk.Comments[0].Body != "Asked from this thread (Request "+key+")." {
		t.Errorf("comments = %+v", tk.Comments)
	}
	if !st.Labels["needs-mk"] {
		t.Errorf("needs-mk label was not created")
	}
	if reg := st.Registry[key]; reg.Status != "registered" || reg.TaskID != "T1" || reg.Identity != ident {
		t.Errorf("registry = %+v", reg)
	}
}

func TestFileCallEnvironments(t *testing.T) {
	e := newEnv(t)
	e.vars["BB_PROJECT_ID"] = "P-ambient"
	ask := e.writeFile("ask.json", askJSON(nil))
	wantCode(t, e.autarch(fileArgs(ask)...), 0)
	for _, c := range e.calls() {
		h := strings.Join(c.Args[:min(3, len(c.Args))], " ")
		switch h {
		case "tasks create --project", "tasks comment T1":
			if c.Thread != "thr_a" {
				t.Errorf("%s ran with BB_THREAD_ID=%q, want thr_a", h, c.Thread)
			}
		default:
			if c.Thread != "thr_a" && c.Thread != "" {
				t.Errorf("%s ran with BB_THREAD_ID=%q", h, c.Thread)
			}
		}
		if strings.HasPrefix(h, "tasks list") && c.Project != "" {
			t.Errorf("tasks list ran with BB_PROJECT_ID=%q, want it removed", c.Project)
		}
	}
}

func TestFileDescriptionVariants(t *testing.T) {
	cases := []struct {
		name   string
		extra  map[string]any
		more   []string
		wire   map[string]any
		blocks []string
	}{
		{name: "blocks sorted in the identity, kept in order in the line, deduplicated", blocks: []string{"bead:mk-2", "project:alpha", "thread:thr_x"},
			more: []string{"--blocks", "bead:mk-2", "--blocks", "project:alpha", "--blocks", "bead:mk-2", "--blocks", "thread:thr_x"}},
		{name: "subject and recommendation", extra: map[string]any{"subject": "Release 4", "recommendation": "yes"}},
		{name: "empty members do not change the identity", extra: map[string]any{"subject": ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			ask := e.writeFile("ask.json", askJSON(c.extra))
			r := e.autarch(fileArgs(ask, c.more...)...)
			wantCode(t, r, 0)
			wire := askMap(t, c.extra)
			for k, v := range c.wire {
				wire[k] = v
			}
			o := oracleReq{project: "alpha", title: "Ship it?", blocks: c.blocks, ask: wire}
			ident := oracleIdent(t, o)
			key := oracleKey("thr_a", ident)
			if got := parseFiled(t, r).Request; got != key {
				t.Errorf("request = %s, want %s", got, key)
			}
			if d := e.st().Tasks[0].Desc; d != oracleDesc(o, key, ident, "") {
				t.Errorf("description:\n%s\nwant:\n%s", d, oracleDesc(o, key, ident, ""))
			}
		})
	}
}

func TestFileIdentityIgnoresEmptySubject(t *testing.T) {
	a, b := newEnv(t), newEnv(t)
	ka := parseFiled(t, a.autarch(fileArgs(a.writeFile("ask.json", askJSON(nil)))...)).Request
	kb := parseFiled(t, b.autarch(fileArgs(b.writeFile("ask.json", askJSON(map[string]any{"subject": ""})))...)).Request
	if ka != kb {
		t.Errorf("an empty subject changed the derived key: %s vs %s", ka, kb)
	}
}

func TestFileRootRunBlock(t *testing.T) {
	e := newEnv(t)
	script := e.writeFile("rr/run.sh", "echo hi\n")
	ask := e.writeFile("ask.json", askJSON(nil))
	r := e.autarch(fileArgs(ask, "--root-run", "script="+script+",timeout=90,set=release")...)
	wantCode(t, r, 0)
	d := e.st().Tasks[0].Desc
	blk := d[strings.Index(d, "```root-run"):]
	lines := strings.Split(strings.TrimSpace(blk), "\n")
	digest := strings.TrimPrefix(lines[2], "sha256: ")
	if len(lines) != 6 || lines[0] != "```root-run" || lines[1] != "script: "+script || len(digest) != 64 ||
		lines[3] != "timeout: 90" || lines[4] != "set: release" || lines[5] != "```" {
		t.Fatalf("root-run block = %q", blk)
	}
	sum := sha256.Sum256([]byte("echo hi\n"))
	if digest != hex.EncodeToString(sum[:]) {
		t.Errorf("root-run sha256 = %s, want the digest of the script bytes", digest)
	}
	o := oracleReq{project: "alpha", title: "Ship it?", ask: askMap(t, nil),
		rootRun: map[string]any{"script": script, "sha256": digest, "timeout": 90, "set": "release"}}
	ident := oracleIdent(t, o)
	if got := parseFiled(t, r).Request; got != oracleKey("thr_a", ident) {
		t.Errorf("request key %s does not derive from the root-run identity %s", got, ident)
	}
	if d != oracleDesc(o, parseFiled(t, r).Request, ident, "\n"+blk) {
		t.Errorf("description:\n%s", d)
	}
}

func TestFileRootRunScriptBytesChangeTheRequest(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	flag := "script=" + e.root + "/rr/run.sh,timeout=90,set=release"
	e.writeFile("rr/run.sh", "echo one\n")
	a := parseFiled(t, e.autarch(fileArgs(ask, "--root-run", flag)...))
	e.writeFile("rr/run.sh", "echo two\n")
	b := parseFiled(t, e.autarch(fileArgs(ask, "--root-run", flag)...))
	if a.Request == b.Request || a.Card == b.Card || b.Replay {
		t.Errorf("a = %+v, b = %+v: changed script bytes must file a new card", a, b)
	}
}

func TestFileReplaySameThread(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	first := parseFiled(t, e.autarch(fileArgs(ask)...))
	n := len(e.calls())
	r := e.autarch(fileArgs(ask)...)
	wantCode(t, r, 0)
	again := parseFiled(t, r)
	want := first
	want.Replay = true
	if again != want {
		t.Fatalf("replay = %+v, want %+v", again, want)
	}
	st := e.st()
	if len(st.Tasks) != 1 || len(st.Tasks[0].Comments) != 1 {
		t.Fatalf("replay changed state: %d tasks, comments %+v", len(st.Tasks), st.Tasks[0].Comments)
	}
	seq := heads(e.calls()[n:], 3)
	wantSeq := []string{"tasks project list", "home binding P1", "tasks label list", "home get --request", "tasks show T1", "home get --card"}
	if !reflect.DeepEqual(seq, wantSeq) {
		t.Fatalf("replay bb calls:\n got %q\nwant %q", seq, wantSeq)
	}
}

func TestFileExplicitRequestKey(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	r := e.autarch(fileArgs(ask, "--request", "my-key:1")...)
	wantCode(t, r, 0)
	if got := parseFiled(t, r).Request; got != "my-key:1" {
		t.Fatalf("request = %q", got)
	}
	if !strings.Contains(e.st().Tasks[0].Desc, "Request: my-key:1 sha256:") {
		t.Errorf("description lacks the explicit key: %s", e.st().Tasks[0].Desc)
	}
	e.thread = "thr_b"
	r = e.autarch(fileArgs(ask, "--request", "my-key:1")...)
	wantCode(t, r, 2, "filed from thread thr_a")
	if len(e.st().Tasks) != 1 {
		t.Errorf("a refused replay created a card")
	}
}

func TestFileSameAskFromTwoThreadsIsTwoCards(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	a := parseFiled(t, e.autarch(fileArgs(ask)...))
	e.thread = "thr_b"
	b := parseFiled(t, e.autarch(fileArgs(ask)...))
	if a.Request == b.Request || a.Card == b.Card || b.Replay {
		t.Fatalf("a=%+v b=%+v: threads must not share a card", a, b)
	}
}

func TestFileReusedRequest(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	wantCode(t, e.autarch(fileArgs(ask, "--request", "k1")...), 0)
	other := e.writeFile("ask2.json", askJSON(map[string]any{"question": "Something else?"}))
	r := e.autarch(fileArgs(other, "--request", "k1")...)
	wantCode(t, r, 2, "Request reused for a different card")
	if len(e.st().Tasks) != 1 {
		t.Errorf("a reused Request created a card")
	}
}

func TestFileCommentFailureThenRepair(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	e.update(func(s *fbState) {
		s.Fail = []fbFail{{Prefix: "tasks comment", Code: 1, Stderr: "boom\n", Times: 1}}
	})
	r := e.autarch(fileArgs(ask)...)
	wantCode(t, r, 4, "T1")
	if r.Stdout != "" {
		t.Errorf("stdout = %q on a failed filing", r.Stdout)
	}
	st := e.st()
	if len(st.Tasks) != 1 || len(st.Tasks[0].Comments) != 0 {
		t.Fatalf("after failure: %d tasks, comments %+v", len(st.Tasks), st.Tasks[0].Comments)
	}
	r = e.autarch(fileArgs(ask)...)
	wantCode(t, r, 0)
	got := parseFiled(t, r)
	if got.Card != "T1" || !got.Replay {
		t.Errorf("repair run = %+v, want card T1 replay true", got)
	}
	st = e.st()
	if len(st.Tasks) != 1 || len(st.Tasks[0].Comments) != 1 || st.Tasks[0].Comments[0].ThreadID != "thr_a" {
		t.Errorf("after repair: %d tasks, comments %+v", len(st.Tasks), st.Tasks[0].Comments)
	}
}

func TestFileCreateFailure(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	e.update(func(s *fbState) {
		s.Fail = []fbFail{{Prefix: "tasks create", Code: 1, Stderr: "Error: connect ECONNREFUSED\n", Times: 1}}
	})
	r := e.autarch(fileArgs(ask)...)
	wantCode(t, r, 3)
	if r.Stdout != "" || len(e.st().Tasks) != 0 {
		t.Errorf("a failed create left stdout %q and %d tasks", r.Stdout, len(e.st().Tasks))
	}
	r = e.autarch(fileArgs(ask)...)
	wantCode(t, r, 0)
	if parseFiled(t, r).Replay {
		t.Errorf("a first successful filing reported replay")
	}
}

func TestFileCreateLostAnswerThenReplay(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	e.update(func(s *fbState) {
		s.Fail = []fbFail{{Prefix: "tasks create", Code: 1, Stderr: "Error: socket hang up\n", Times: 1, ApplyThenFail: true}}
	})
	r := e.autarch(fileArgs(ask)...)
	wantCode(t, r, 0)
	got := parseFiled(t, r)
	if got.Card != "T1" || !got.Replay || len(e.st().Tasks) != 1 || len(e.st().Tasks[0].Comments) != 1 {
		t.Fatalf("a lost create answer = %+v with %d tasks", got, len(e.st().Tasks))
	}
	if n := len(e.callsWith("home", "get", "--request")); n != 2 {
		t.Errorf("the registry was read %d times, want 2 (before the create and to recover from it)", n)
	}
}

func TestFileLegacyRegistryRefused(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	e.update(func(s *fbState) { s.Registry["k-legacy"] = fbReg{Status: "legacy", DecisionID: "d1"} })
	r := e.autarch(fileArgs(ask, "--request", "k-legacy")...)
	wantCode(t, r, 2, "pre-cards ask")
	if len(e.st().Tasks) != 0 {
		t.Errorf("a legacy Request created a card")
	}
}

func TestFileRegisteredCardGoneOrChanged(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	e.update(func(s *fbState) {
		s.Registry["k-gone"] = fbReg{Status: "registered", TaskID: "T404", Identity: "0123456789abcdef"}
	})
	r := e.autarch(fileArgs(ask, "--request", "k-gone")...)
	t.Logf("gone: code=%d stderr=%q", r.Code, r.Stderr)
	if r.Code == 0 || len(e.st().Tasks) != 0 {
		t.Errorf("a registered-but-missing card exited %d with %d tasks", r.Code, len(e.st().Tasks))
	}
}

func TestFileUnparseableCardWithRequestLine(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	e.update(func(s *fbState) {
		s.NoRegisterOnMake = true
		s.Tasks = append(s.Tasks, &fbTask{ID: "T9", Project: "P1", Created: fbStamp(0, 9), Desc: "free text\n\nRequest: k-bad sha256:0123456789abcdef\n"})
		s.Seq = 9
	})
	r := e.autarch(fileArgs(ask, "--request", "k-bad")...)
	wantCode(t, r, 2, "Request k-bad appears on unparseable card T9")
	if len(e.st().Tasks) != 1 {
		t.Errorf("an unparseable duplicate led to a new card")
	}
}

func TestFileCanonicalCardIsEarliestAcrossPages(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	first := parseFiled(t, e.autarch(fileArgs(ask, "--request", "k-dup")...))
	st := e.st()
	desc := st.Tasks[0].Desc
	comment := st.Tasks[0].Comments
	e.update(func(s *fbState) {
		delete(s.Registry, "k-dup")
		s.Tasks[0].Created = fbStamp(0, 50)
		for i := 2; i <= 4; i++ {
			s.Tasks = append(s.Tasks, &fbTask{ID: "T" + string(rune('0'+i)), Project: "P1", Created: fbStamp(0, 10-i), Desc: desc, Comments: comment})
		}
		s.Seq = 4
	})
	r := e.autarch(fileArgs(ask, "--request", "k-dup")...)
	wantCode(t, r, 0)
	got := parseFiled(t, r)
	if first.Card != "T1" || got.Card != "T4" || !got.Replay {
		t.Fatalf("first = %+v, canonical = %+v: want the earliest-created card T4 as a replay", first, got)
	}
	if n := len(e.callsWith("tasks", "list")); n < 2 {
		t.Errorf("tasks list was called %d time(s); the fake pages by 2, so a 4-card search needs more than one page", n)
	}
}

func TestFileLockFileUnderRuntimeDir(t *testing.T) {
	e := newEnv(t)
	ask := e.writeFile("ask.json", askJSON(nil))
	r := e.autarch(fileArgs(ask, "--request", "lock-key")...)
	wantCode(t, r, 0)
	ents, err := filepath.Glob(filepath.Join(e.run, "autarch-needsmk-*", "lock-key.lock"))
	if err != nil || len(ents) != 1 {
		t.Fatalf("lock files = %v (%v)", ents, err)
	}
	fi, err := os.Stat(filepath.Dir(ents[0]))
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("lock dir mode = %v (%v), want 0700", fi.Mode().Perm(), err)
	}
}
