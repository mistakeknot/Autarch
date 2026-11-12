package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
)

// ghStub answers the two gh reads the base guard makes and records every call.
type ghStub struct {
	calls   [][]string
	base    string
	def     string
	fail    bool
	baseErr bool
}

func (g *ghStub) run(_ context.Context, args ...string) ([]byte, error) {
	g.calls = append(g.calls, args)
	if g.fail {
		return nil, errors.New("gh: not logged in")
	}
	joined := strings.Join(args, " ")
	switch {
	case strings.HasPrefix(joined, "pr view 211 --repo o/r --json baseRefName"):
		return []byte(`{"baseRefName":"` + g.base + `"}`), nil
	case strings.HasPrefix(joined, "repo view o/r --json defaultBranchRef"):
		return []byte(`{"defaultBranchRef":{"name":"` + g.def + `"}}`), nil
	}
	return nil, errors.New("unexpected gh call: " + joined)
}

func useGH(t *testing.T, g *ghStub) {
	t.Helper()
	old := ghRun
	ghRun = g.run
	t.Cleanup(func() { ghRun = old })
}

func writeMergeAsk(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ask.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const mergeAskJSON = `{"project":"autarch","project_root":"/srv/autarch","question":"Merge https://github.com/o/r/pull/211 ?","options":[{"id":"a","label":"A) Manually merge it at 58814293","kind":"ruling-only"},{"id":"b","label":"B) Not now","kind":"ruling-only"}]}`

func fileMerge(t *testing.T, s *stubBB, body string, extra ...string) (string, error) {
	t.Helper()
	t.Setenv("BB_THREAD_ID", "thread-x")
	args := append([]string{"file", "--project", "P1", "--title", "Merge?", "--ask-file", writeMergeAsk(t, body)}, extra...)
	return runNeedsMk(t, s, args...)
}

func TestNeedsMkMergeAskOnNonDefaultBaseIsRefused(t *testing.T) {
	g := &ghStub{base: "sync/x", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	_, err := fileMerge(t, s, mergeAskJSON)
	if exitCode(err) != 2 {
		t.Fatalf("want exit 2, got err=%v", err)
	}
	for _, want := range []string{"o/r#211", "sync/x", "main"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q lacks %q", err, want)
		}
	}
	if s.creates != 0 || len(s.calls) != 0 {
		t.Fatalf("something was filed: creates=%d calls=%v", s.creates, s.calls)
	}
}

func TestNeedsMkMergeAskOnDefaultBaseSaysInto(t *testing.T) {
	for _, def := range []string{"main", "master"} {
		g := &ghStub{base: def, def: def}
		useGH(t, g)
		s := &stubBB{}
		if _, err := fileMerge(t, s, mergeAskJSON); err != nil {
			t.Fatalf("%s: %v", def, err)
		}
		if s.creates != 1 {
			t.Fatalf("%s: creates=%d", def, s.creates)
		}
		c, err := homeask.ParseCard(s.desc)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(s.desc, "Manually merge it at 58814293 into "+def) {
			t.Errorf("%s: card does not say the branch:\n%s", def, s.desc)
		}
		_ = c
	}
}

func TestNeedsMkPRFlagChecksTheBaseWithoutMergeOption(t *testing.T) {
	g := &ghStub{base: "sync/x", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	_, err := fileMerge(t, s, plainAskJSON, "--pr", "https://github.com/o/r/pull/211")
	if exitCode(err) != 2 || s.creates != 0 {
		t.Fatalf("err=%v creates=%d", err, s.creates)
	}
	g = &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s = &stubBB{}
	if _, err := fileMerge(t, s, plainAskJSON, "--pr", "https://github.com/o/r/pull/211"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.desc, "into main") {
		t.Errorf("card does not say the branch:\n%s", s.desc)
	}
}

const plainAskJSON = `{"project":"autarch","project_root":"/srv/autarch","question":"Ship it?","options":[{"id":"a","label":"A","kind":"ruling-only"},{"id":"b","label":"B","kind":"ruling-only"}]}`

func TestNeedsMkMergeAskRefusedWhenGHFailsOrPRUnknown(t *testing.T) {
	g := &ghStub{fail: true}
	useGH(t, g)
	s := &stubBB{}
	_, err := fileMerge(t, s, mergeAskJSON)
	if exitCode(err) != 2 || s.creates != 0 || len(s.calls) != 0 {
		t.Fatalf("gh failure: err=%v creates=%d", err, s.creates)
	}
	// a merge option with no PR URL anywhere and no --pr cannot be checked, so it is not filed
	g = &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s = &stubBB{}
	noURL := strings.Replace(mergeAskJSON, "https://github.com/o/r/pull/211", "the PR", 1)
	_, err = fileMerge(t, s, noURL)
	if exitCode(err) != 2 || s.creates != 0 || len(g.calls) != 0 {
		t.Fatalf("no PR named: err=%v creates=%d gh=%v", err, s.creates, g.calls)
	}
	// a malformed --pr is refused
	_, err = fileMerge(t, &stubBB{}, mergeAskJSON, "--pr", "https://example.com/o/r/pull/1")
	if exitCode(err) != 2 {
		t.Fatalf("bad --pr: err=%v", err)
	}
}

func TestNeedsMkNonMergeAskMakesNoGHCall(t *testing.T) {
	g := &ghStub{base: "sync/x", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	// a PR URL in a non-merge question is not a merge ask
	body := strings.Replace(plainAskJSON, "Ship it?", "Review https://github.com/o/r/pull/211 ?", 1)
	if _, err := fileMerge(t, s, body); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 0 || s.creates != 1 || strings.Contains(s.desc, "into ") {
		t.Fatalf("gh=%v creates=%d desc=%s", g.calls, s.creates, s.desc)
	}
}
