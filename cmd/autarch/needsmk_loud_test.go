package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// An index timeout is an ambiguous outcome, never a successful filing without a key.
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
	if exitCode(err) != 4 || !strings.Contains(fmt.Sprint(err), "may have been filed") || !strings.Contains(fmt.Sprint(err), "--request") {
		t.Fatalf("err=%v stderr=%q", err, stderr)
	}
}

// B5: with no project in the ask, needs-mk file takes the project from the task board's binding, not
// from the folder name, which is wrong in a bb worktree.
func TestNeedsMkFileDefaultsTheProjectToTheBoardBindingInAWorktree(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thread-x")
	s := &stubBB{}
	var gotProject string
	oldF, oldC := newCardFiler, checkFilingPreconditions
	newCardFiler = func() *homeask.CardFiler { return &homeask.CardFiler{Run: bindingStub(s), LockDir: t.TempDir()} }
	checkFilingPreconditions = func(project, _ string) error { gotProject = project; return nil }
	t.Cleanup(func() { newCardFiler, checkFilingPreconditions = oldF, oldC })
	ask := writeAskBody(t, `{"project_root":"`+worktreeRoot+`","question":"Ship it?","options":[{"id":"a","label":"A","kind":"ruling-only"},{"id":"b","label":"B","kind":"ruling-only"}]}`)
	_, err := execNeedsMkFile("--project", "SHWK", "--title", "Ship?", "--ask-file", ask)
	if err != nil || gotProject != "shadow-work" {
		t.Fatalf("err=%v checked project=%q", err, gotProject)
	}
}

func TestNeedsMkCardKeyGuarantee(t *testing.T) {
	for _, mode := range []string{"late", "timeout", "outside", "json"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("BB_THREAD_ID", "thread-x")
			if mode == "outside" {
				t.Chdir(t.TempDir())
			}
			s := &stubBB{}
			oldF, oldC := newCardFiler, checkFilingPreconditions
			polls := 0
			newCardFiler = func() *homeask.CardFiler {
				return &homeask.CardFiler{LockDir: t.TempDir(), ReadbackTimeout: 40 * time.Millisecond, ReadbackInterval: time.Millisecond, Run: func(ctx context.Context, env []string, args ...string) homeask.BBResult {
					if strings.Join(args[:min(len(args), 3)], " ") == "home get --card" {
						polls++
						if mode == "timeout" || (mode == "late" && polls < 4) {
							return homeask.BBResult{Stdout: []byte(`{"card_key":""}`)}
						}
						if mode == "outside" && !strings.Contains(strings.Join(args, " "), "--project P1") {
							return homeask.BBResult{Code: 1, Stderr: []byte("project context missing")}
						}
					}
					return s.run(ctx, env, args...)
				}}
			}
			checkFilingPreconditions = func(string, string) error { return nil }
			t.Cleanup(func() { newCardFiler, checkFilingPreconditions = oldF, oldC })
			cmd := needsMkCmd()
			var out, errs bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errs)
			args := []string{"file", "--project", "P1", "--title", "Choice?", "--ask-file", writeAsk(t), "--request", "key-index-test"}
			if mode == "json" {
				args = append(args, "--json")
			}
			cmd.SetArgs(args)
			err := cmd.Execute()
			if mode == "timeout" {
				if exitCode(err) != 4 || out.Len() != 0 || !strings.Contains(errs.String(), "key-index-test") || !strings.Contains(errs.String(), "may have been filed") {
					t.Fatalf("exit=%d err=%v stdout=%q", exitCode(err), err, out.String())
				}
				return
			}
			var res homeask.CardResult
			if err != nil || json.Unmarshal(out.Bytes(), &res) != nil || res.CardKey == "" {
				t.Fatalf("err=%v stdout=%q polls=%d", err, out.String(), polls)
			}
		})
	}
}

func TestNeedsMkReadbackAllowsSlowIndex(t *testing.T) {
	if readbackTimeout != 30*time.Second {
		t.Fatalf("readback timeout=%s, want 30s", readbackTimeout)
	}
}
