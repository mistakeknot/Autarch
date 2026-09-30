package main

import (
	"bytes"
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

func askJSON(thread string) string {
	m := map[string]any{
		"v": 1, "kind": "decide", "asker": "thread", "subject": "autarch/decide: order", "project": "Autarch",
		"project_root": "/home/mk/projects/Autarch", "question": "Which order?",
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

func TestDecideFileThreadFlagReachesBBAndTheRequest(t *testing.T) {
	dir := fakeBB(t)
	setVerb(t, dir, "ask", `echo '{"id":"dec-1"}'`)
	out, err := runDecide(t, askJSON(""), "file", "--thread", "thr-x")
	if err != nil || out != "dec-1" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env"))
	if !strings.Contains(string(env), "BB_THREAD_ID=thr-x") {
		t.Fatalf("env = %q", env)
	}
	stdin, _ := os.ReadFile(filepath.Join(dir, "stdin"))
	if !strings.Contains(string(stdin), `"thread":"thr-x"`) {
		t.Fatalf("stdin = %s", stdin)
	}
}

func TestDecideFileThreadFromEnv(t *testing.T) {
	dir := fakeBB(t)
	setVerb(t, dir, "ask", `echo '{"id":"dec-1"}'`)
	t.Setenv("BB_THREAD_ID", "thr-env")
	if _, err := runDecide(t, askJSON(""), "file"); err != nil {
		t.Fatal(err)
	}
	stdin, _ := os.ReadFile(filepath.Join(dir, "stdin"))
	if !strings.Contains(string(stdin), `"thread":"thr-env"`) {
		t.Fatalf("stdin = %s", stdin)
	}
}

func TestDecideThreadConflictExitsTwoAndRunsNothing(t *testing.T) {
	dir := fakeBB(t)
	t.Setenv("BB_THREAD_ID", "thr-y")
	_, err := runDecide(t, askJSON(""), "file", "--thread", "thr-x")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "thread flag conflicts with BB_THREAD_ID") {
		t.Fatalf("err = %v code %d", err, exitCode(err))
	}
	if _, statErr := os.Stat(filepath.Join(dir, "calls")); statErr == nil {
		t.Fatal("bb ran")
	}
}

func TestDecideNeedsAThreadUnlessMycroft(t *testing.T) {
	fakeBB(t)
	_, err := runDecide(t, askJSON(""), "file")
	if exitCode(err) != 2 {
		t.Fatalf("err = %v code %d", err, exitCode(err))
	}
}

func TestDecideMycroftFilingRunsWithoutAThread(t *testing.T) {
	dir := fakeBB(t)
	setVerb(t, dir, "ask", `echo '{"id":"dec-m"}'`)
	t.Setenv("BB_THREAD_ID", "thr-leak")
	in := `{"v":1,"kind":"decide","project":"estate","project_root":"/tmp/estate","question":"Ship?","options":[{"id":"y","label":"Yes","kind":"ruling-only"},{"id":"n","label":"No","kind":"ruling-only"}]}`
	out, err := runDecide(t, in, "file", "--asker", "mycroft")
	if err != nil || out != "dec-m" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env"))
	if !strings.Contains(string(env), "BB_THREAD_ID=unset") {
		t.Fatalf("env = %q", env)
	}
}

func TestDecideCommitThenHangSucceedsOnTheFirstRun(t *testing.T) {
	dir := fakeBB(t)
	setVerb(t, dir, "ask", "sleep 5")
	setVerb(t, dir, "get", getReply(expectedAsk(t, "thr-x"), ""))
	out, err := runDecide(t, askJSON("thr-x"), "file")
	if err != nil || out != "dec-1" || exitCode(err) != 0 {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestDecideUnknownOutcomeExitsFourAndTheRetryReturnsTheSameID(t *testing.T) {
	dir := fakeBB(t)
	setVerb(t, dir, "ask", "sleep 5")
	setVerb(t, dir, "get", "sleep 5")
	_, err := runDecide(t, askJSON("thr-x"), "file")
	if exitCode(err) != 4 {
		t.Fatalf("err = %v code %d", err, exitCode(err))
	}
	setVerb(t, dir, "ask", `echo '{"id":"dec-1"}'`)
	out, err := runDecide(t, askJSON("thr-x"), "file")
	if err != nil || out != "dec-1" {
		t.Fatalf("retry out=%q err=%v", out, err)
	}
}

func TestDecideConflictingRequestWithLostResponseExitsTwo(t *testing.T) {
	dir := fakeBB(t)
	setVerb(t, dir, "ask", "sleep 5")
	setVerb(t, dir, "get", getReply(expectedAsk(t, "thr-x"), strings.Repeat("0", 64)))
	out, err := runDecide(t, askJSON("thr-x"), "file")
	if exitCode(err) != 2 || out != "" {
		t.Fatalf("out=%q err=%v code %d", out, err, exitCode(err))
	}
}

func TestDecideExitCodesThreeTwoFive(t *testing.T) {
	dir := fakeBB(t)
	for _, tc := range []struct {
		script string
		want   int
	}{{"exit 2", 2}, {"exit 3", 3}, {"exit 5", 5}} {
		setVerb(t, dir, "ask", "echo boom >&2; "+tc.script)
		_, err := runDecide(t, askJSON("thr-x"), "file")
		if exitCode(err) != tc.want {
			t.Fatalf("%s: err=%v code %d", tc.script, err, exitCode(err))
		}
	}
}

func TestDecideBBAbsentExitsThree(t *testing.T) {
	fakeBB(t)
	t.Setenv("PATH", t.TempDir())
	_, err := runDecide(t, askJSON("thr-x"), "file")
	if exitCode(err) != 3 {
		t.Fatalf("err=%v code %d", err, exitCode(err))
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
	dir := fakeBB(t)
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
	if _, statErr := os.Stat(filepath.Join(dir, "calls")); statErr == nil {
		t.Fatal("bb ran")
	}
}
