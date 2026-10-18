package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/homeask"
)

func writeAskBody(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "ask.json")
	os.WriteFile(p, []byte(body), 0o644)
	return p
}

func execNeedsMkFile(args ...string) (string, error) {
	root := &cobra.Command{Use: "autarch", SilenceErrors: true}
	root.AddCommand(needsMkCmd())
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(append([]string{"needs-mk", "file"}, args...))
	err := root.Execute()
	return errb.String(), err
}

// Uppercase option ids are refused up front (exit 2, says lowercase) before serve is probed or bb is run,
// so no ruling row or message is written for an ask that cannot be filed.
func TestNeedsMkFileRefusesUppercaseOptionIDsBeforeAnyCall(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thread-x")
	s := &stubBB{}
	probed := false
	ask := writeAskBody(t, `{"project":"autarch","project_root":"/srv/autarch","question":"Ship it?","options":[{"id":"A","label":"A","kind":"ruling-only"},{"id":"B","label":"B","kind":"ruling-only"}]}`)
	oldF, oldC := newCardFiler, checkFilingPreconditions
	newCardFiler = func() *homeask.CardFiler { return &homeask.CardFiler{Run: s.run, LockDir: t.TempDir()} }
	checkFilingPreconditions = func(string, string) error { probed = true; return nil }
	t.Cleanup(func() { newCardFiler, checkFilingPreconditions = oldF, oldC })
	_, err := execNeedsMkFile("--project", "P1", "--title", "Ship?", "--ask-file", ask)
	if exitCode(err) != 2 || err == nil || !strings.Contains(err.Error(), "lowercase") || !strings.Contains(err.Error(), `"A"`) {
		t.Fatalf("exit=%d err=%v", exitCode(err), err)
	}
	if probed || len(s.calls) != 0 {
		t.Fatalf("probed=%v bb calls=%v", probed, s.calls)
	}
}

// bindingStub answers the binding reads for tasks project SHWK bound to Home project shadow-work.
func bindingStub(s *stubBB) homeask.BBRunner {
	return func(ctx context.Context, env []string, args ...string) homeask.BBResult {
		switch strings.Join(args[:min(len(args), 3)], " ") {
		case "tasks project list":
			return homeask.BBResult{Stdout: []byte(`{"projects":[{"id":"P1","name":"Shadow","prefix":"SHWK"}]}`)}
		case "home binding P1":
			return homeask.BBResult{Stdout: []byte(`{"home_project":"shadow-work","state":"confirmed"}`)}
		}
		return s.run(ctx, env, args...)
	}
}

func runWithBinding(t *testing.T, s *stubBB, project string, check func(string, string) error) error {
	t.Helper()
	t.Setenv("BB_THREAD_ID", "thread-x")
	oldF, oldC := newCardFiler, checkFilingPreconditions
	newCardFiler = func() *homeask.CardFiler { return &homeask.CardFiler{Run: bindingStub(s), LockDir: t.TempDir()} }
	checkFilingPreconditions = check
	t.Cleanup(func() { newCardFiler, checkFilingPreconditions = oldF, oldC })
	ask := writeAskBody(t, `{"project":"`+project+`","project_root":"/srv/autarch","question":"Ship it?","options":[{"id":"a","label":"A","kind":"ruling-only"},{"id":"b","label":"B","kind":"ruling-only"}]}`)
	_, err := execNeedsMkFile("--project", "SHWK", "--title", "Ship?", "--ask-file", ask)
	return err
}

// A project that is not the bound Home project is refused, naming the expected one, and nothing is created.
func TestNeedsMkFileRefusesTheWrongProjectNamingTheBoundOne(t *testing.T) {
	s := &stubBB{}
	err := runWithBinding(t, s, "shadow-workipedia", func(string, string) error { return nil })
	if exitCode(err) != 2 || err == nil || !strings.Contains(err.Error(), `expected project "shadow-work"`) {
		t.Fatalf("exit=%d err=%v", exitCode(err), err)
	}
	if s.creates != 0 {
		t.Fatalf("created %d cards", s.creates)
	}
}

// The tasks key is an accepted alias, and the root check must see the bound name, not the alias:
// serve lists projects by Home name, so checking the alias would skip the project_root match.
func TestNeedsMkFileChecksTheRootAgainstTheBoundProjectName(t *testing.T) {
	s := &stubBB{}
	var gotProject string
	err := runWithBinding(t, s, "SHWK", func(project, _ string) error {
		gotProject = project
		return fmt.Errorf("%w: project_root does not match", homeask.ErrInvalid)
	})
	if gotProject != "shadow-work" || exitCode(err) != 2 {
		t.Fatalf("checked project=%q exit=%d err=%v", gotProject, exitCode(err), err)
	}
	if s.creates != 0 {
		t.Fatalf("created %d cards", s.creates)
	}
}

// With nothing listening, the real precondition check exits 3, says serve is not running, and runs no bb.
func TestNeedsMkFileRealServeProbeFailsLoudlyWhenServeIsDown(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thread-x")
	tok := filepath.Join(t.TempDir(), "serve.token")
	os.WriteFile(tok, []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"), 0o600)
	t.Setenv("AUTARCH_SERVE_TOKEN_FILE", tok)
	t.Setenv("AUTARCH_SERVE_URL", "http://127.0.0.1:1")
	s := &stubBB{}
	oldF := newCardFiler
	newCardFiler = func() *homeask.CardFiler { return &homeask.CardFiler{Run: s.run, LockDir: t.TempDir()} }
	t.Cleanup(func() { newCardFiler = oldF })
	_, err := execNeedsMkFile("--project", "P1", "--title", "x", "--ask-file", writeAsk(t))
	if exitCode(err) != 3 || err == nil || !strings.Contains(err.Error(), "not running") || len(writeCalls(s)) != 0 {
		t.Fatalf("exit=%d err=%v calls=%v", exitCode(err), err, s.calls)
	}
}

// The filer prints card_key once Home has indexed the card, and warns when it had not.
func TestNeedsMkFilePrintsCardKeyFromReadback(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thread-x")
	s := &stubBB{}
	oldF := newCardFiler
	newCardFiler = func() *homeask.CardFiler {
		return &homeask.CardFiler{LockDir: t.TempDir(), ReadbackTimeout: 1, Run: func(ctx context.Context, env []string, args ...string) homeask.BBResult {
			if strings.Join(args[:min(len(args), 3)], " ") == "home get --card" {
				return homeask.BBResult{Code: 1, Stderr: []byte("unknown card")}
			}
			return s.run(ctx, env, args...)
		}}
	}
	t.Cleanup(func() { newCardFiler = oldF })
	oldC := checkFilingPreconditions
	checkFilingPreconditions = func(string, string) error { return nil }
	t.Cleanup(func() { checkFilingPreconditions = oldC })
	stderr, err := execNeedsMkFile("--project", "P1", "--title", "x", "--ask-file", writeAsk(t))
	if err != nil || !strings.Contains(stderr, "not indexed") {
		t.Fatalf("err=%v stderr=%q", err, stderr)
	}
}
