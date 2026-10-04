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
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/homeask"
	"github.com/mistakeknot/autarch/internal/serve"
)

// stubBB answers the whole filing protocol for an empty tasks service.
type stubBB struct {
	mu    sync.Mutex
	calls [][]string
	desc  string
	envs  []string

	creates          int  // tasks create calls that wrote a card
	loseCreateAnswer bool // the card is written but the create answer is lost
	failComment      bool // the routing comment call fails
	card             bool // a card exists
	commented        bool
	thread           string
}

func (s *stubBB) run(_ context.Context, env []string, args ...string) homeask.BBResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, args)
	for _, kv := range env {
		if strings.HasPrefix(kv, "BB_THREAD_ID=") {
			s.envs = append(s.envs, strings.Join(args[:2], " ")+" "+kv)
		}
	}
	j := func(v string) homeask.BBResult { return homeask.BBResult{Stdout: []byte(v)} }
	switch strings.Join(args[:min(len(args), 3)], " ") {
	case "home get --request":
		return j(`{"status":"absent"}`)
	case "tasks label list":
		return j(`{"labels":[{"name":"needs-mk"}]}`)
	case "tasks project list":
		return j(`{"projects":[{"id":"P1","name":"P1"}]}`)
	case "tasks list --project":
		if s.card {
			return j(`{"tasks":[{"id":"T1","projectId":"P1","description":` + strconv.Quote(s.desc) + `}],"nextCursor":null}`)
		}
		return j(`{"tasks":[],"nextCursor":null}`)
	case "tasks create --project":
		for i, a := range args {
			if a == "--description-file" {
				b, _ := os.ReadFile(args[i+1])
				s.desc = string(b)
			}
		}
		s.card = true
		s.creates++
		if s.loseCreateAnswer {
			s.loseCreateAnswer = false
			return homeask.BBResult{Err: errors.New("deadline exceeded")}
		}
		return j(`{"task":{"id":"T1","projectId":"P1"}}`)
	case "tasks show T1":
		cm := `[]`
		if s.commented {
			cm = `[{"id":"c1","kind":"agent","threadId":"` + s.thread + `","createdAt":"2026-10-01T00:00:01Z"}]`
		}
		return j(`{"task":{"id":"T1","projectId":"P1","createdAt":"2026-10-01T00:00:00Z","description":` + strconv.Quote(s.desc) + `},"comments":` + cm + `}`)
	case "tasks comment T1":
		if s.failComment {
			return homeask.BBResult{Err: errors.New("deadline exceeded")}
		}
		thr := "thr_x"
		for _, kv := range env {
			if v, ok := strings.CutPrefix(kv, "BB_THREAD_ID="); ok {
				thr = v
			}
		}
		s.commented = true
		s.thread = thr
		return j(`{"comment":{"id":"c1","kind":"agent","threadId":"` + thr + `"}}`)
	}
	return homeask.BBResult{Code: 1, Stderr: []byte("unexpected")}
}

// serveCheckSet is true while a test installs its own checkFilingPreconditions.
var serveCheckSet bool

func runNeedsMk(t *testing.T, s *stubBB, args ...string) (string, error) {
	t.Helper()
	old := newCardFiler
	newCardFiler = func() *homeask.CardFiler {
		return &homeask.CardFiler{Run: s.run, LockDir: t.TempDir()}
	}
	t.Cleanup(func() { newCardFiler = old })
	oldCheck := checkFilingPreconditions
	if !serveCheckSet {
		checkFilingPreconditions = func(string, string) error { return nil }
	}
	t.Cleanup(func() { checkFilingPreconditions = oldCheck })
	root := &cobra.Command{Use: "autarch", SilenceErrors: true}
	root.AddCommand(needsMkCmd())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"needs-mk"}, args...))
	err := root.Execute()
	return strings.TrimSpace(out.String()), err
}

func writeAsk(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "ask.json")
	os.WriteFile(p, []byte(`{"project":"autarch","project_root":"/srv/autarch","question":"Ship it?","options":[{"id":"a","label":"A","kind":"ruling-only"},{"id":"b","label":"B","kind":"ruling-only"}]}`), 0o644)
	return p
}

func TestNeedsMkFileFilesAndPrintsTheCard(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	s := &stubBB{}
	out, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Ship?", "--blocks", "thread:thr_x", "--blocks", "bead:A-1", "--ask-file", writeAsk(t))
	if err != nil {
		t.Fatal(err)
	}
	var res homeask.CardResult
	if json.Unmarshal([]byte(out), &res) != nil || res.ID != "T1" || res.Replay || res.Key == "" {
		t.Fatalf("out=%q", out)
	}
	c, err := homeask.ParseCard(s.desc)
	if err != nil || len(c.Blocks) != 2 || c.Request.Key != res.Key {
		t.Fatalf("desc=%q err=%v", s.desc, err)
	}
	// the comment is run from the thread; nothing else is
	if len(s.envs) != 2 || !strings.HasPrefix(s.envs[0], "tasks create") || !strings.HasPrefix(s.envs[1], "tasks comment") {
		t.Fatalf("thread env: %v", s.envs)
	}
	// a derived key is stable across runs
	s2 := &stubBB{}
	out2, _ := runNeedsMk(t, s2, "file", "--project", "P1", "--title", "Ship?", "--blocks", "bead:A-1", "--blocks", "thread:thr_x", "--ask-file", writeAsk(t))
	var res2 homeask.CardResult
	json.Unmarshal([]byte(out2), &res2)
	if res2.Key != res.Key {
		t.Fatalf("keys differ: %s %s", res.Key, res2.Key)
	}
}

func TestNeedsMkFileRequiresThreadAndFlags(t *testing.T) {
	s := &stubBB{}
	t.Setenv("BB_THREAD_ID", "")
	_, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "x", "--ask-file", writeAsk(t))
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "BB_THREAD_ID") || len(s.calls) != 0 {
		t.Fatalf("err=%v calls=%v", err, s.calls)
	}
	t.Setenv("BB_THREAD_ID", "thr_x")
	for _, args := range [][]string{
		{"file", "--title", "x", "--ask-file", writeAsk(t)},
		{"file", "--project", "P1", "--ask-file", writeAsk(t)},
		{"file", "--project", "P1", "--title", "x"},
		{"file", "--project", "P1", "--title", "x", "--ask-file", "/nonexistent"},
	} {
		if _, err := runNeedsMk(t, s, args...); exitCode(err) != 2 {
			t.Errorf("%v: err=%v", args, err)
		}
	}
	if len(s.calls) != 0 {
		t.Fatalf("bb was run: %v", s.calls)
	}
}

func TestNeedsMkFileRootRunHashesTheScript(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	script := filepath.Join(t.TempDir(), "run.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho hi\n"), 0o755)
	sum := sha256.Sum256([]byte("#!/bin/sh\necho hi\n"))
	s := &stubBB{}
	if _, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "x", "--ask-file", writeAsk(t), "--root-run", "script="+script+",timeout=30,set=deploy"); err != nil {
		t.Fatal(err)
	}
	c, err := homeask.ParseCard(s.desc)
	if err != nil || c.RootRun == nil || c.RootRun.SHA256 != hex.EncodeToString(sum[:]) || c.RootRun.Timeout != 30 {
		t.Fatalf("%+v %v", c.RootRun, err)
	}
	for _, bad := range []string{"script=rel.sh,timeout=30,set=a", "script=" + script + ",timeout=x,set=a", "script=" + script + ",timeout=30", "script=" + script + ",timeout=30,set=-bad"} {
		if _, err := runNeedsMk(t, &stubBB{}, "file", "--project", "P1", "--title", "x", "--ask-file", writeAsk(t), "--root-run", bad); exitCode(err) != 2 {
			t.Errorf("%q: err=%v", bad, err)
		}
	}
}

func TestNeedsMkFileHomeDownIsExit3(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	old := newCardFiler
	oldCheck := checkFilingPreconditions
	checkFilingPreconditions = func(string, string) error { return nil }
	t.Cleanup(func() { newCardFiler = old; checkFilingPreconditions = oldCheck })
	newCardFiler = func() *homeask.CardFiler {
		return &homeask.CardFiler{LockDir: t.TempDir(), Run: func(context.Context, []string, ...string) homeask.BBResult {
			return homeask.BBResult{Code: 3, Stderr: []byte("not ready")}
		}}
	}
	root := &cobra.Command{Use: "autarch", SilenceErrors: true}
	root.AddCommand(needsMkCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"needs-mk", "file", "--project", "P1", "--title", "x", "--ask-file", writeAsk(t)})
	if err := root.Execute(); exitCode(err) != 3 {
		t.Fatalf("err=%v", err)
	}
}

func TestNeedsMkFileRejectsNonObjectAskJSON(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	for _, body := range []string{"null", "[]", `"s"`, "7", "true"} {
		p := filepath.Join(t.TempDir(), "ask.json")
		os.WriteFile(p, []byte(body), 0o644)
		s := &stubBB{}
		_, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "x", "--ask-file", p)
		if exitCode(err) != 2 {
			t.Fatalf("%s: want usage error (exit 2), got %v", body, err)
		}
	}
}

func fileWithCheck(t *testing.T, s *stubBB, check func(string, string) error) error {
	t.Helper()
	t.Setenv("BB_THREAD_ID", "thr_x")
	old := checkFilingPreconditions
	checkFilingPreconditions = check
	serveCheckSet = true
	t.Cleanup(func() { checkFilingPreconditions = old; serveCheckSet = false })
	_, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Ship?", "--ask-file", writeAsk(t))
	return err
}

// mk-okek.19: with serve down, filing stops before it touches bb, exits 3 and says why.
func TestNeedsMkFileServeDownCreatesNothing(t *testing.T) {
	s := &stubBB{}
	err := fileWithCheck(t, s, func(string, string) error {
		return fmt.Errorf("%w: %w", homeask.ErrHomeDown, serve.ErrNotRunning)
	})
	if exitCode(err) != 3 || !strings.Contains(err.Error(), "autarch serve is not running") {
		t.Fatalf("exit=%d err=%v", exitCode(err), err)
	}
	if len(s.calls) != 0 || s.creates != 0 {
		t.Fatalf("bb was called: %v", s.calls)
	}
}

// mk-okek.20: a project_root serve does not resolve is refused at file time, exit 2.
func TestNeedsMkFileRefusesAnUnresolvedRoot(t *testing.T) {
	s := &stubBB{}
	var got string
	err := fileWithCheck(t, s, func(_, root string) error {
		got = root
		return fmt.Errorf("%w: project_root %q is not a project root serve resolves", homeask.ErrInvalid, root)
	})
	if got != "/srv/autarch" || exitCode(err) != 2 {
		t.Fatalf("root=%q exit=%d err=%v", got, exitCode(err), err)
	}
	if len(s.calls) != 0 {
		t.Fatalf("bb was called: %v", s.calls)
	}
}
