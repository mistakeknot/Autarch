package homeask

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeBB installs a fake `bb` whose `home ask|get|list` behaviour is the shell
// snippet written to ask.sh, get.sh and list.sh in the returned directory.
func fakeBB(t *testing.T) (bin, dir string) {
	t.Helper()
	dir = t.TempDir()
	bin = filepath.Join(dir, "bb")
	script := `#!/bin/sh
echo "$@" >> "$FAKE_DIR/calls"
echo "BB_THREAD_ID=${BB_THREAD_ID-unset}" >> "$FAKE_DIR/env"
verb="$2"
if [ "$verb" = ask ]; then cat > "$FAKE_DIR/stdin"; fi
[ -f "$FAKE_DIR/$verb.sh" ] && . "$FAKE_DIR/$verb.sh"
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_DIR", dir)
	return bin, dir
}

func setVerb(t *testing.T, dir, verb, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, verb+".sh"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testAsk(t *testing.T) Ask {
	t.Helper()
	a := base()
	a.Thread = "thr-x"
	a.Asker = "thread"
	return mustNorm(t, a)
}

func newFiler(bin string) *ExecFiler {
	return &ExecFiler{Bin: bin, Timeout: 400 * time.Millisecond, RecoveryTimeout: 400 * time.Millisecond}
}

func getJSON(a Ask, identity string) string {
	if identity == "" {
		identity = Identity(a)
	}
	b, _ := json.Marshal(map[string]any{"result": "decision", "decision_id": "dec-1", "identity": identity, "thread": a.Thread, "project": a.Project})
	return "echo '" + string(b) + "'\n"
}

func TestFilerFilesAndPassesThreadAcrossTheProxy(t *testing.T) {
	bin, dir := fakeBB(t)
	setVerb(t, dir, "ask", `echo '{"id":"dec-1","request_id":"r","mentioned":false}'`)
	a := testAsk(t)
	t.Setenv("BB_THREAD_ID", "thr-other")
	id, err := newFiler(bin).File(context.Background(), a)
	if err != nil || id != "dec-1" {
		t.Fatalf("File = %q, %v", id, err)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env"))
	if !strings.Contains(string(env), "BB_THREAD_ID=thr-x") {
		t.Fatalf("child env = %q, want the request's thread", env)
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if !strings.Contains(string(calls), "home ask --request-stdin") {
		t.Fatalf("calls = %q", calls)
	}
	var sent Ask
	raw, _ := os.ReadFile(filepath.Join(dir, "stdin"))
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.RequestID != Identity(a) || sent.Thread != "thr-x" {
		t.Fatalf("sent request_id=%q thread=%q", sent.RequestID, sent.Thread)
	}
}

func TestFilerMycroftRunsWithoutBBThreadID(t *testing.T) {
	bin, dir := fakeBB(t)
	setVerb(t, dir, "ask", `echo '{"id":"dec-2"}'`)
	a := base()
	a.Asker = "mycroft"
	a.Thread = ""
	a.Recommendation = ""
	a.Options = []Option{{ID: "yes", Label: "Yes", Kind: "ruling-only"}, {ID: "no", Label: "No", Kind: "ruling-only"}}
	a = mustNorm(t, a)
	t.Setenv("BB_THREAD_ID", "thr-leak")
	if _, err := newFiler(bin).File(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env"))
	if !strings.Contains(string(env), "BB_THREAD_ID=unset") {
		t.Fatalf("child env = %q, want BB_THREAD_ID unset", env)
	}
}

func TestFilerCommitThenHangRecoversThroughGet(t *testing.T) {
	bin, dir := fakeBB(t)
	a := testAsk(t)
	setVerb(t, dir, "ask", "sleep 5")
	setVerb(t, dir, "get", getJSON(a, ""))
	id, err := newFiler(bin).File(context.Background(), a)
	if err != nil || id != "dec-1" {
		t.Fatalf("File = %q, %v; want the recovered id", id, err)
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if !strings.Contains(string(calls), "home get --request-id "+Identity(a)) {
		t.Fatalf("calls = %q", calls)
	}
}

func TestFilerHangWithHungRecoveryIsUnknownThenRetryReturnsSameID(t *testing.T) {
	bin, dir := fakeBB(t)
	a := testAsk(t)
	setVerb(t, dir, "ask", "sleep 5")
	setVerb(t, dir, "get", "sleep 5")
	_, err := newFiler(bin).File(context.Background(), a)
	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("err = %v, want ErrOutcomeUnknown", err)
	}
	setVerb(t, dir, "ask", `echo '{"id":"dec-1"}'`)
	id, err := newFiler(bin).File(context.Background(), a)
	if err != nil || id != "dec-1" {
		t.Fatalf("retry = %q, %v", id, err)
	}
}

func TestFilerUnfoundRecoveryIsUnknown(t *testing.T) {
	bin, dir := fakeBB(t)
	a := testAsk(t)
	setVerb(t, dir, "ask", "sleep 5")
	setVerb(t, dir, "get", "echo 'unknown request id' >&2; exit 1")
	if _, err := newFiler(bin).File(context.Background(), a); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("err = %v", err)
	}
}

func TestFilerRecoveryWithOtherIdentityIsAConflictNeverSuccess(t *testing.T) {
	bin, dir := fakeBB(t)
	a := testAsk(t)
	setVerb(t, dir, "ask", "sleep 5")
	setVerb(t, dir, "get", getJSON(a, strings.Repeat("0", 64)))
	id, err := newFiler(bin).File(context.Background(), a)
	if id != "" || !errors.Is(err, ErrInvalid) {
		t.Fatalf("File = %q, %v; want ErrInvalid and no id", id, err)
	}
	if !strings.Contains(err.Error(), "different ask") {
		t.Fatalf("err = %v", err)
	}
}

func TestFilerRecoveryWithOtherScopeIsAConflict(t *testing.T) {
	bin, dir := fakeBB(t)
	a := testAsk(t)
	setVerb(t, dir, "ask", "sleep 5")
	b := a
	b.Thread = "thr-else"
	setVerb(t, dir, "get", getJSON(b, Identity(a)))
	if _, err := newFiler(bin).File(context.Background(), a); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestFilerExitCodes(t *testing.T) {
	bin, dir := fakeBB(t)
	a := testAsk(t)
	for _, tc := range []struct {
		code string
		want error
	}{{"2", ErrInvalid}, {"3", ErrHomeDown}, {"5", ErrAlreadyRuled}} {
		setVerb(t, dir, "ask", "echo 'the message' >&2; exit "+tc.code)
		_, err := newFiler(bin).File(context.Background(), a)
		if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), "the message") {
			t.Fatalf("exit %s: err = %v, want %v with the message", tc.code, err, tc.want)
		}
	}
}

func TestFilerBBAbsentIsHomeDown(t *testing.T) {
	a := testAsk(t)
	_, err := newFiler("/nonexistent/dir/bb").File(context.Background(), a)
	if !errors.Is(err, ErrHomeDown) {
		t.Fatalf("err = %v", err)
	}
}

func TestFilerRefusedConnectionIsHomeDown(t *testing.T) {
	bin, dir := fakeBB(t)
	setVerb(t, dir, "ask", "echo 'connect ECONNREFUSED 127.0.0.1:3000' >&2; exit 1")
	if _, err := newFiler(bin).File(context.Background(), testAsk(t)); !errors.Is(err, ErrHomeDown) {
		t.Fatalf("err = %v", err)
	}
}

func TestFilerInvalidAskRunsNothing(t *testing.T) {
	bin, dir := fakeBB(t)
	bad := base()
	bad.Question = ""
	if _, err := newFiler(bin).File(context.Background(), bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "calls")); err == nil {
		t.Fatal("bb ran for an invalid ask")
	}
}

func TestListerReadsAskerAndReportsDown(t *testing.T) {
	bin, dir := fakeBB(t)
	setVerb(t, dir, "list", `echo '[{"id":"d1","subject":"s","project":"estate","thread":"","asker":"mycroft","filed_at":"x"}]'`)
	rows, err := newFiler(bin).List(context.Background(), "mycroft")
	if err != nil || len(rows) != 1 || rows[0].ID != "d1" {
		t.Fatalf("List = %v, %v", rows, err)
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if !strings.Contains(string(calls), "home list --asker mycroft --json") {
		t.Fatalf("calls = %q", calls)
	}
	if _, err := newFiler("/nonexistent/bb").List(context.Background(), "mycroft"); !errors.Is(err, ErrHomeDown) {
		t.Fatalf("err = %v", err)
	}
}
