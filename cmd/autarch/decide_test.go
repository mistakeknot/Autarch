package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/homeask"
)

// fakeBB puts a fake `bb` first on PATH. `home <verb>` sources $FAKE_DIR/<verb>.sh.
func fakeBB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
echo "$@" >> "$FAKE_DIR/calls"
echo "BB_THREAD_ID=${BB_THREAD_ID-unset}" >> "$FAKE_DIR/env"
verb="$2"
if [ "$verb" = ask ]; then cat > "$FAKE_DIR/stdin"; fi
[ -f "$FAKE_DIR/$verb.sh" ] && . "$FAKE_DIR/$verb.sh"
`
	if err := os.WriteFile(filepath.Join(dir, "bb"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_DIR", dir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BB_THREAD_ID", "")
	old := filerTimeout
	filerTimeout = 400 * time.Millisecond
	t.Cleanup(func() { filerTimeout = old })
	return dir
}

func setVerb(t *testing.T, dir, verb, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, verb+".sh"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// withCardFiler runs decide file against an in-memory tasks service (stubBB).
func withCardFiler(t *testing.T, s *stubBB, tune ...func(*homeask.CardFiler)) {
	t.Helper()
	old := newFiler
	t.Cleanup(func() { newFiler = old })
	newFiler = func() *homeask.CardFiler {
		f := &homeask.CardFiler{Run: s.run, LockDir: t.TempDir(), TasksProject: "P1"}
		for _, fn := range tune {
			fn(f)
		}
		return f
	}
}

func askJSON(thread string) string {
	m := map[string]any{
		"v": 1, "kind": "decide", "asker": "thread", "subject": "autarch/decide: order", "project": "Autarch",
		"project_root": "/home/dev/projects/Autarch", "question": "Which order?",
		"options": []map[string]any{
			{"id": "a", "label": "A first", "kind": "instruction", "reversible": true, "instruction": "Do A then B and report."},
			{"id": "b", "label": "B first", "kind": "needs-context"},
		},
	}
	if thread != "" {
		m["thread"] = thread
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func runDecide(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "autarch", SilenceErrors: true}
	root.AddCommand(decideCmd())
	var out, errBuf bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs(append([]string{"decide"}, args...))
	err := root.Execute()
	return strings.TrimSpace(out.String()), err
}

func expectedAsk(t *testing.T, thread string) homeask.Ask {
	t.Helper()
	a, err := homeask.Parse([]byte(askJSON(thread)))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func getReply(a homeask.Ask, identity string) string {
	if identity == "" {
		identity = homeask.Identity(a)
	}
	b, _ := json.Marshal(map[string]any{"result": "decision", "decision_id": "dec-1", "identity": identity, "thread": a.Thread, "project": a.Project})
	return "echo '" + string(b) + "'\n"
}

func TestDecideFileThreadFlagReachesTheCardRoutingComment(t *testing.T) {
	s := &stubBB{}
	withCardFiler(t, s)
	t.Setenv("BB_THREAD_ID", "")
	out, err := runDecide(t, askJSON(""), "file", "--thread", "thr-x")
	if err != nil || out != "T1" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if len(s.envs) != 2 || !strings.HasPrefix(s.envs[0], "tasks create") || !strings.HasSuffix(s.envs[0], "BB_THREAD_ID=thr-x") || !strings.HasPrefix(s.envs[1], "tasks comment") {
		t.Fatalf("thread env: %v", s.envs)
	}
	c, err := homeask.ParseCard(s.desc)
	if err != nil || c.Pull != "" {
		t.Fatalf("card %+v %v", c, err)
	}
}

func TestDecideFileThreadFromEnv(t *testing.T) {
	s := &stubBB{}
	withCardFiler(t, s)
	t.Setenv("BB_THREAD_ID", "thr-env")
	if _, err := runDecide(t, askJSON(""), "file"); err != nil {
		t.Fatal(err)
	}
	if len(s.envs) == 0 || !strings.HasSuffix(s.envs[0], "BB_THREAD_ID=thr-env") {
		t.Fatalf("env = %v", s.envs)
	}
}

func TestDecideThreadConflictExitsTwoAndRunsNothing(t *testing.T) {
	s := &stubBB{}
	withCardFiler(t, s)
	t.Setenv("BB_THREAD_ID", "thr-y")
	_, err := runDecide(t, askJSON(""), "file", "--thread", "thr-x")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "thread flag conflicts with BB_THREAD_ID") || len(s.calls) != 0 {
		t.Fatalf("err = %v code %d calls %v", err, exitCode(err), s.calls)
	}
}

func TestDecideNeedsAThread(t *testing.T) {
	s := &stubBB{}
	withCardFiler(t, s)
	t.Setenv("BB_THREAD_ID", "")
	_, err := runDecide(t, askJSON(""), "file")
	if exitCode(err) != 2 || len(s.calls) != 0 {
		t.Fatalf("err = %v code %d", err, exitCode(err))
	}
}

// The autarch CLI has no way to file threadless: only Mycroft's FileForPull does (operator question 4).
func TestDecideRefusesAThreadlessMycroftAskAndRunsNothing(t *testing.T) {
	s := &stubBB{}
	withCardFiler(t, s)
	t.Setenv("BB_THREAD_ID", "thr-leak")
	in := `{"v":1,"kind":"decide","project":"estate","project_root":"/tmp/estate","question":"Ship?","options":[{"id":"y","label":"Yes","kind":"ruling-only"},{"id":"n","label":"No","kind":"ruling-only"}]}`
	out, err := runDecide(t, in, "file", "--asker", "mycroft")
	if exitCode(err) != 2 || out != "" || len(s.calls) != 0 {
		t.Fatalf("out=%q err=%v code %d calls %v", out, err, exitCode(err), s.calls)
	}
}

func TestDecideFileUsesTheConfiguredTasksProjectBinding(t *testing.T) {
	s := &stubBB{}
	old := newFiler
	t.Cleanup(func() { newFiler = old })
	newFiler = func() *homeask.CardFiler {
		f := homeask.CardFilerFromEnv(filerTimeout)
		f.Run, f.LockDir = s.run, t.TempDir()
		return f
	}
	t.Setenv("BB_THREAD_ID", "thr-x")
	t.Setenv(homeask.EnvTasksProject, "")
	t.Setenv(homeask.EnvTasksProjects, "")
	if _, err := runDecide(t, askJSON("thr-x"), "file"); exitCode(err) != 2 || len(s.calls) != 0 {
		t.Fatalf("unbound: err=%v calls=%v", err, s.calls)
	}
	t.Setenv(homeask.EnvTasksProjects, "autarch=P1")
	t.Setenv("BB_THREAD_ID", "")
	if out, err := runDecide(t, askJSON("thr-x"), "file"); err != nil || out != "T1" {
		t.Fatalf("bound: out=%q err=%v", out, err)
	}
	var created string
	for _, c := range s.calls {
		if len(c) > 2 && c[0] == "tasks" && c[1] == "create" {
			created = strings.Join(c, " ")
		}
	}
	if !strings.Contains(created, "--project P1") {
		t.Fatalf("created: %q", created)
	}
}

func TestDecideCommitThenLostAnswerSucceedsOnTheFirstRun(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "")
	s := &stubBB{loseCreateAnswer: true}
	withCardFiler(t, s)
	out, err := runDecide(t, askJSON("thr-x"), "file")
	if err != nil || out != "T1" || exitCode(err) != 0 {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestDecideUnknownOutcomeExitsFourAndTheRetryReturnsTheSameCard(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "")
	s := &stubBB{failComment: true}
	withCardFiler(t, s)
	_, err := runDecide(t, askJSON("thr-x"), "file")
	if exitCode(err) != 4 {
		t.Fatalf("err = %v code %d", err, exitCode(err))
	}
	s.failComment = false
	out, err := runDecide(t, askJSON("thr-x"), "file")
	if err != nil || out != "T1" {
		t.Fatalf("retry out=%q err=%v", out, err)
	}
	if s.creates != 1 {
		t.Fatalf("creates = %d, want 1", s.creates)
	}
}

func TestDecideExitCodesTwoThreeAndBBAbsent(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "")
	for _, tc := range []struct {
		name string
		run  homeask.BBRunner
		want int
	}{
		{"home not ready", func(context.Context, []string, ...string) homeask.BBResult {
			return homeask.BBResult{Code: 3, Stderr: []byte("not ready")}
		}, 3},
		{"bb absent", func(context.Context, []string, ...string) homeask.BBResult {
			return homeask.BBResult{Err: errors.New("exec: bb not found")}
		}, 3},
	} {
		old := newFiler
		newFiler = func() *homeask.CardFiler {
			return &homeask.CardFiler{Run: tc.run, LockDir: t.TempDir(), TasksProject: "P1"}
		}
		_, err := runDecide(t, askJSON("thr-x"), "file")
		newFiler = old
		if exitCode(err) != tc.want {
			t.Fatalf("%s: err=%v code %d", tc.name, err, exitCode(err))
		}
	}
}

func TestDecideListAndStatsRunBBHome(t *testing.T) {
	dir := fakeBB(t)
	setVerb(t, dir, "list", `echo '[{"id":"d1"}]'`)
	setVerb(t, dir, "stats", `echo '{"filed":3}'`)
	out, err := runDecide(t, "", "list", "--asker", "mycroft")
	if err != nil || out != `[{"id":"d1"}]` {
		t.Fatalf("list out=%q err=%v", out, err)
	}
	out, err = runDecide(t, "", "stats")
	if err != nil || out != `{"filed":3}` {
		t.Fatalf("stats out=%q err=%v", out, err)
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if !strings.Contains(string(calls), "home list --json --asker mycroft") && !strings.Contains(string(calls), "home list --asker mycroft --json") {
		t.Fatalf("calls = %q", calls)
	}
}

func TestDecideHasNoPick(t *testing.T) {
	for _, c := range decideCmd().Commands() {
		if c.Name() == "pick" {
			t.Fatal("the Go CLI must not pick")
		}
	}
}

func TestExitCodeMapping(t *testing.T) {
	for err, want := range map[error]int{
		nil:                                      0,
		errors.New("x"):                          1,
		fmt.Errorf("w: %w", homeask.ErrInvalid):  2,
		fmt.Errorf("w: %w", homeask.ErrHomeDown): 3,
		fmt.Errorf("w: %w", homeask.ErrOutcomeUnknown): 4,
		fmt.Errorf("w: %w", homeask.ErrAlreadyRuled):   5,
	} {
		if got := exitCode(err); got != want {
			t.Errorf("exitCode(%v) = %d, want %d", err, got, want)
		}
	}
}

func TestVersionJSONReportsRevisionAndExecutableHash(t *testing.T) {
	root := &cobra.Command{Use: "autarch"}
	root.AddCommand(versionCmd())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var v struct {
		VCS struct {
			Revision string `json:"revision"`
			Modified *bool  `json:"modified"`
		} `json:"vcs"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	exe, _ := os.Executable()
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if v.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha256 = %q", v.SHA256)
	}
	if v.VCS.Modified == nil {
		t.Fatalf("vcs.modified missing: %s", out.String())
	}
}

func TestDecideNonStringProjectFieldsAreValidationErrorsNotPanics(t *testing.T) {
	s := &stubBB{}
	withCardFiler(t, s)
	t.Setenv("BB_THREAD_ID", "thr-x")
	for _, in := range []string{
		`{"v":1,"kind":"decide","project_root":5,"question":"q?"}`,
		`{"v":1,"kind":"decide","project_root":null,"question":"q?"}`,
		`{"v":1,"kind":"decide","project_root":"/tmp/x","project":["a"],"question":"q?"}`,
	} {
		_, err := runDecide(t, in, "file")
		if exitCode(err) != 2 {
			t.Fatalf("%s: err = %v code %d", in, err, exitCode(err))
		}
	}
	if len(s.calls) != 0 {
		t.Fatalf("bb ran: %v", s.calls)
	}
}

// Review s1-3 P2: JSON null (and any non-object) on stdin is a validation error, never a nil-map panic.
func TestBuildAskRejectsNonObjectJSON(t *testing.T) {
	for _, raw := range []string{"null", "[]", `"s"`, "7", "true"} {
		t.Run(raw, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("buildAsk(%s) panicked: %v", raw, r)
				}
			}()
			_, err := buildAsk([]byte(raw), "thread-a", "", "/srv/x", "x")
			var ue *usageError
			if !errors.As(err, &ue) {
				t.Fatalf("buildAsk(%s) = %v, want a usageError", raw, err)
			}
		})
	}
}
