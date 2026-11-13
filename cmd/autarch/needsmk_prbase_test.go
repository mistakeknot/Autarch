package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	defErr  bool   // the repo lookup fails
	rawBase string // when set, the pr view answer verbatim
}

func (g *ghStub) run(_ context.Context, args ...string) ([]byte, error) {
	g.calls = append(g.calls, args)
	if g.fail {
		return nil, errors.New("gh: not logged in")
	}
	joined := strings.Join(args, " ")
	switch {
	case strings.HasPrefix(joined, "pr view 211 --repo github.com/o/r --json baseRefName"):
		if g.rawBase != "" {
			return []byte(g.rawBase), nil
		}
		return []byte(`{"baseRefName":"` + g.base + `"}`), nil
	case strings.HasPrefix(joined, "repo view github.com/o/r --json defaultBranchRef"):
		if g.defErr {
			return nil, errors.New("gh: repo not found")
		}
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
	if s.creates != 0 || len(writeCalls(s)) != 0 {
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
	if exitCode(err) != 2 || s.creates != 0 || len(writeCalls(s)) != 0 {
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

func TestNeedsMkMergeLabelWithoutAShaStillTriggersTheCheck(t *testing.T) {
	g := &ghStub{base: "sync/x", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	body := strings.Replace(mergeAskJSON, "58814293", "HEAD", 1)
	if _, err := fileMerge(t, s, body); exitCode(err) != 2 || s.creates != 0 {
		t.Fatalf("err=%v creates=%d", err, s.creates)
	}
}

func TestNeedsMkEmptyPRFlagIsRefused(t *testing.T) {
	g := &ghStub{base: "sync/x", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	if _, err := fileMerge(t, s, plainAskJSON, "--pr="); exitCode(err) != 2 || s.creates != 0 {
		t.Fatalf("err=%v creates=%d", err, s.creates)
	}
}

func TestNeedsMkStaleIntoTextIsReplacedByTheVerifiedBranch(t *testing.T) {
	g := &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	body := strings.Replace(mergeAskJSON, "58814293", "58814293 into sync/x", 1)
	if _, err := fileMerge(t, s, body); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s.desc, "sync/x") || !strings.Contains(s.desc, "Manually merge it at 58814293 into main") {
		t.Fatalf("card:\n%s", s.desc)
	}
}

func TestNeedsMkMergeAskRefusedOnBadGitHubMetadata(t *testing.T) {
	for name, g := range map[string]*ghStub{
		"repo lookup fails": {base: "main", def: "main", defErr: true},
		"empty base":        {base: "", def: "main", rawBase: `{"baseRefName":""}`},
		"not json":          {base: "main", def: "main", rawBase: `oops`},
		"empty default":     {base: "main", def: ""},
		"both empty":        {base: "", def: "", rawBase: `{"baseRefName":""}`},
	} {
		useGH(t, g)
		s := &stubBB{}
		if _, err := fileMerge(t, s, mergeAskJSON); exitCode(err) != 2 || s.creates != 0 {
			t.Errorf("%s: err=%v creates=%d", name, err, s.creates)
		}
	}
}

const nestedMergeAskJSON = `{"project":"autarch","project_root":"/srv/autarch","question":"Merge https://github.com/o/r/pull/211 ?","questions":[{"id":"q1","prompt":"Merge it?","options":[{"id":"a","label":"A) Manually merge it at 58814293"},{"id":"b","label":"B) Not now"}]}]}`

func TestNeedsMkMergeOptionInsideQuestionsIsChecked(t *testing.T) {
	g := &ghStub{base: "sync/x", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	if _, err := fileMerge(t, s, nestedMergeAskJSON); exitCode(err) != 2 || s.creates != 0 {
		t.Fatalf("err=%v creates=%d gh=%v", err, s.creates, g.calls)
	}
	g = &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s = &stubBB{}
	if _, err := fileMerge(t, s, nestedMergeAskJSON); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.desc, "Manually merge it at 58814293 into main") {
		t.Fatalf("card:\n%s", s.desc)
	}
}

func TestNeedsMkStaleIntoTextWithoutAShaIsReplaced(t *testing.T) {
	g := &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	body := strings.Replace(mergeAskJSON, "at 58814293", "at into sync/x", 1)
	if _, err := fileMerge(t, s, body); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s.desc, "sync/x") || !strings.Contains(s.desc, `"A) Manually merge it at into main"`) {
		t.Fatalf("card:\n%s", s.desc)
	}
}

// The ask parser reads keys without regard to case, so detection must too.
func TestNeedsMkMergeLabelKeyCaseDoesNotEscapeTheCheck(t *testing.T) {
	for name, body := range map[string]string{
		"top level": strings.Replace(mergeAskJSON, `"label":"A) Manually`, `"Label":"A) Manually`, 1),
		"nested":    strings.Replace(nestedMergeAskJSON, `"label":"A) Manually`, `"LABEL":"A) Manually`, 1),
	} {
		g := &ghStub{base: "sync/x", def: "main"}
		useGH(t, g)
		s := &stubBB{}
		if _, err := fileMerge(t, s, body); exitCode(err) != 2 || s.creates != 0 {
			t.Errorf("%s: err=%v creates=%d gh=%v", name, err, s.creates, g.calls)
		}
	}
}

func TestNeedsMkPRFlagMustMatchThePRTheAskNames(t *testing.T) {
	g := &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	body := strings.Replace(mergeAskJSON, "pull/211", "pull/212", 1)
	if _, err := fileMerge(t, s, body, "--pr", "https://github.com/o/r/pull/211"); exitCode(err) != 2 || s.creates != 0 {
		t.Fatalf("err=%v creates=%d gh=%v", err, s.creates, g.calls)
	}
	// the same PR named both ways is fine
	s = &stubBB{}
	if _, err := fileMerge(t, s, mergeAskJSON, "--pr", "https://github.com/o/r/pull/211"); err != nil || s.creates != 1 {
		t.Fatalf("err=%v creates=%d", err, s.creates)
	}
}

func TestNeedsMkPRURLWithDotSegmentsIsRefused(t *testing.T) {
	for _, u := range []string{"https://github.com/o/r/pull/211/../212", "https://github.com/o/r/pull/211/%2e%2e/212", "https://github.com/o/r/pull/211/./files/../../212", "https://github.com/o/r/pull/211.5", "https://github.com/o/r/pull/211/other"} {
		g := &ghStub{base: "main", def: "main"}
		useGH(t, g)
		s := &stubBB{}
		body := strings.Replace(mergeAskJSON, "https://github.com/o/r/pull/211", u, 1)
		if _, err := fileMerge(t, s, body); exitCode(err) != 2 || s.creates != 0 {
			t.Errorf("%s: err=%v creates=%d", u, err, s.creates)
		}
	}
	// a plain /files link to the same PR is fine
	g := &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	body := strings.Replace(mergeAskJSON, "pull/211 ?", "pull/211/files ?", 1)
	if _, err := fileMerge(t, s, body); err != nil || s.creates != 1 {
		t.Fatalf("files link: err=%v creates=%d", err, s.creates)
	}
}

// askWithQuestion is mergeAskJSON with its question replaced by q.
func askWithQuestion(t *testing.T, q string) string {
	t.Helper()
	enc, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Replace(mergeAskJSON, `"Merge https://github.com/o/r/pull/211 ?"`, string(enc), 1)
}

// Only the plain https://github.com/<o>/<r>/pull/<n> link (plus a PR tab, fragment or query) may name the PR; any
// other spelling could resolve to a PR whose base was not checked.
func TestNeedsMkOtherSpellingsOfAnotherPRAreRefused(t *testing.T) {
	cases := []string{
		"Merge https://github.com/o/r/pull/211 and https://github.com/o/r/pull/211/files./../../212 ?",
		"Merge https://github.com/o/r/pull/211 and https://github.com/o/r/pull/211,/../212 ?",
		`Merge https://github.com/o/r/pull/211 and https://github.com/o/r/pull/211\..\212 ?`,
		"Merge https://github.com/o/r/pull/211 and http://github.com/o/r/pull/212 ?",
		"Merge https://github.com/o/r/pull/211 and https://GitHub.com/o/r/pull/212 ?",
		"Merge https://github.com/o/r/pull/211 and https://github.com:443/o/r/pull/212 ?",
		"Merge https://github.com/o/r/pull/211 and https://api.github.com/repos/o/r/pulls/212 ?",
		"Merge https://github.com/o/r/pull/211 and github.com/o/r/pull/212 ?",
		"Merge https://github.com/o/r/pull/211 and #212 ?",
		"Merge https://github.com/o/r/pull/211 and o/r#212 ?",
		"Merge https://github.com/o/r/pull/211 and other/repo#211 ?",
		"Merge https://github.com/o/r/pull/211 and PR 212 ?",
	}
	for _, q := range cases {
		for _, withFlag := range []bool{false, true} {
			g := &ghStub{base: "main", def: "main"}
			useGH(t, g)
			s := &stubBB{}
			var extra []string
			if withFlag {
				extra = []string{"--pr", "https://github.com/o/r/pull/211"}
			}
			if _, err := fileMerge(t, s, askWithQuestion(t, q), extra...); exitCode(err) != 2 || s.creates != 0 {
				t.Errorf("%q flag=%v: err=%v creates=%d", q, withFlag, err, s.creates)
			}
		}
	}
	// --pr with only a short reference to the same PR is fine, and so are other mentions of the PR itself.
	for _, q := range []string{"Merge #211 ?", "Merge o/r#211 ?", "Merge PR 211 (https://github.com/o/r/pull/211/files#diff-1)."} {
		g := &ghStub{base: "main", def: "main"}
		useGH(t, g)
		s := &stubBB{}
		if _, err := fileMerge(t, s, askWithQuestion(t, q), "--pr", "https://github.com/o/r/pull/211"); err != nil || s.creates != 1 {
			t.Errorf("%q: err=%v creates=%d", q, err, s.creates)
		}
	}
}

func TestNeedsMkDotSegmentOwnerOrRepoIsRefused(t *testing.T) {
	for _, u := range []string{"https://github.com/../r/pull/211", "https://github.com/o/../pull/211", "https://github.com/./r/pull/211"} {
		g := &ghStub{base: "main", def: "main"}
		useGH(t, g)
		s := &stubBB{}
		if _, err := fileMerge(t, s, askWithQuestion(t, "Merge "+u+" ?")); exitCode(err) != 2 || s.creates != 0 || len(g.calls) != 0 {
			t.Errorf("%s: err=%v creates=%d gh=%v", u, err, s.creates, g.calls)
		}
	}
}

// The card is more than the ask JSON: its title, --move and the other text flags reach Citadel too, so a PR
// named there must be the PR whose base was checked.
func TestNeedsMkPRNamedOutsideTheAskIsChecked(t *testing.T) {
	other := `{"schema":"home-move/v1","kind":"pr","pr":{"url":"https://github.com/o/r/pull/212"}}`
	same := `{"schema":"home-move/v1","kind":"pr","pr":{"url":"https://github.com/o/r/pull/211"}}`
	run := func(args ...string) (*stubBB, error) {
		g := &ghStub{base: "main", def: "main"}
		useGH(t, g)
		s := &stubBB{}
		t.Setenv("BB_THREAD_ID", "thread-x")
		a := append([]string{"file", "--project", "P1", "--ask-file", writeMergeAsk(t, mergeAskJSON)}, args...)
		_, err := runNeedsMk(t, s, a...)
		return s, err
	}
	for _, args := range [][]string{
		{"--title", "Merge https://github.com/o/r/pull/212 ?"},
		{"--title", "Merge #212 ?"},
		{"--title", "Merge ?", "--move", writeMove(t, other)},
	} {
		if s, err := run(args...); exitCode(err) != 2 || s.creates != 0 {
			t.Errorf("%v: err=%v creates=%d", args, err, s.creates)
		}
	}
	for _, args := range [][]string{
		{"--title", "Merge https://github.com/o/r/pull/211 ?"},
		{"--title", "Merge ?", "--move", writeMove(t, same)},
	} {
		if s, err := run(args...); err != nil || s.creates != 1 {
			t.Errorf("%v: err=%v creates=%d", args, err, s.creates)
		}
	}
}

// The project is resolved (bound board, folder name) after the ask is read; the guard must see the resolved card.
func TestNeedsMkResolvedProjectNameIsChecked(t *testing.T) {
	body := strings.Replace(mergeAskJSON, `"project":"autarch",`, "", 1)
	for _, bound := range []string{"Merge PR 212", "Merge https://github.com/o/r/pull/212"} {
		g := &ghStub{base: "main", def: "main"}
		useGH(t, g)
		s := &stubBB{bound: bound}
		if _, err := fileMerge(t, s, body); exitCode(err) != 2 || s.creates != 0 {
			t.Errorf("bound %q: err=%v creates=%d", bound, err, s.creates)
		}
	}
	g := &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s := &stubBB{bound: "Autarch"}
	if _, err := fileMerge(t, s, body); err != nil || s.creates != 1 {
		t.Fatalf("plain bound project: err=%v creates=%d", err, s.creates)
	}
}

// gh must be pinned to github.com: a GH_HOST for another server would answer for a different repository.
func TestNeedsMkGHLookupsAreQualifiedWithGitHubCom(t *testing.T) {
	g := &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	if _, err := fileMerge(t, s, mergeAskJSON); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 2 {
		t.Fatalf("calls=%v", g.calls)
	}
	for _, c := range g.calls {
		if !strings.Contains(strings.Join(c, " "), " github.com/o/r") {
			t.Errorf("gh call not pinned to github.com: %v", c)
		}
	}
}

// A host spelled with percent escapes or look-alike characters normalizes to github.com in a browser.
func TestNeedsMkSchemeRelativeAndUnicodeHostsAreRefused(t *testing.T) {
	for _, u := range []string{
		"[this PR](//\u24d6ithub.com/o/r/pull/212)",
		"//\u24d6ithub.com/o/r/pull/212",
		"\\\\\u24d6ithub.com/o/r/pull/212",
		"/\\\u24d6ithub.com/o/r/pull/212",
		"https:\u24d6ithub.com/o/r/pull/212",
		"//example.com:8080/o/r/pull/212",
		"\\\\example.com:8080/o/r/pull/212",
		"//user@example.com/x",
		"\u1d4dithub.com/o/r/pull/212",
	} {
		g := &ghStub{base: "main", def: "main"}
		useGH(t, g)
		s := &stubBB{}
		q := "Checked https://github.com/o/r/pull/211; merge " + u + " ?"
		if _, err := fileMerge(t, s, askWithQuestion(t, q), "--pr", "https://github.com/o/r/pull/211"); exitCode(err) != 2 || s.creates != 0 {
			t.Errorf("%s: err=%v creates=%d", u, err, s.creates)
		}
	}
}

func TestNeedsMkCommandsAndEscapedLinksNameAnUncheckedPR(t *testing.T) {
	for _, u := range []string{
		"`gh pr merge 212 --repo o/r`",
		"gh pr view 212",
		"gh pr merge 212",
		"merge 212",
		"merge PR 212",
		"merge #212",
		"gh pr merge 211 --repo other/repo",
		"gh pr merge 211 -R other/repo",
		"gh pr merge --squash 212 --repo o/r",
		"gh pr merge '212' --repo o/r",
		"gh pr merge 211 --repo 'other/repo'",
		"gh pr merge 211 -Rother/repo",
		"gh api repos/o/r/pulls/212/merge -X PUT",
		"gh pr merge 211",
		"gh -Rother/repo pr merge 211",
		"gh -Ro/r pr merge '212' --squash",
		"gh 'pr' merge '212' --squash",
		"GH.exe pr merge 212",
		"env GH_REPO=other/repo gh pr merge 211",
		"\"gh\" pr merge 212",
		"[this PR](https:&#x2f;&#x2f;github&#x2e;com/o/r/pull/212)",
		"[this PR](https:&sol;&sol;github.com/o/r/pull/212)",
		"[this PR](https://github&period;com/o/r/pull/212)",
		"[this PR](https://github\\.com/o/r/pull/212)",
		"[this PR](https://github&amp;#46;com/o/r/pull/212)",
	} {
		g := &ghStub{base: "main", def: "main"}
		useGH(t, g)
		s := &stubBB{}
		q := "Checked https://github.com/o/r/pull/211; " + u + " ?"
		if _, err := fileMerge(t, s, askWithQuestion(t, q), "--pr", "https://github.com/o/r/pull/211"); exitCode(err) != 2 || s.creates != 0 {
			t.Errorf("%s: err=%v creates=%d", u, err, s.creates)
		}
	}
	// the verified PR itself, and a sha after the merge phrase, stay fine; a gh command never does
	g := &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	q := "Merge 211 now (head abc1234, closes #211, at https://github.com/o/r/pull/211)"
	if _, err := fileMerge(t, s, askWithQuestion(t, q), "--pr", "https://github.com/o/r/pull/211"); err != nil || s.creates != 1 {
		t.Fatalf("verified PR: err=%v creates=%d", err, s.creates)
	}
}

func TestNeedsMkShellQuotingAndEscapingIsRefused(t *testing.T) {
	for _, u := range []string{
		"'g''h' pr merge '212' '--repo' 'other/repo'",
		"\"g\"\"h\" pr merge 212",
		"g\\h pr merge 212",
		"$'\\x67h' pr merge 212",
		"${X}h pr merge 212",
		"$(echo gh) pr merge 212",
		"'g''h' pr merge 211",
		"g\\h pr merge 211",
		"$'\\x67h' pr merge 211",
		"${X} pr merge 211",
		"run g''h pr merge --squash 212",
		"run g\"\"h pr merge 211",
		"run g'h' pr merge 211",
		"merge --squash 212",
		"merge -d --admin #212",
		"merge '212'",
		"merge \"212\"",
		"x '--repo' other/repo",
		"x \"-R\"other/repo",
	} {
		g := &ghStub{base: "main", def: "main"}
		useGH(t, g)
		s := &stubBB{}
		q := "Checked https://github.com/o/r/pull/211; " + u + " ?"
		if _, err := fileMerge(t, s, askWithQuestion(t, q), "--pr", "https://github.com/o/r/pull/211"); exitCode(err) != 2 || s.creates != 0 {
			t.Errorf("%s: err=%v creates=%d", u, err, s.creates)
		}
	}
	// ordinary prose with an apostrophe or a quoted word is fine
	g := &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	q := "It's ready: merge https://github.com/o/r/pull/211 (the \"safe\" option)"
	if _, err := fileMerge(t, s, askWithQuestion(t, q), "--pr", "https://github.com/o/r/pull/211"); err != nil || s.creates != 1 {
		t.Fatalf("prose: err=%v creates=%d", err, s.creates)
	}
}

func TestNeedsMkObfuscatedGitHubHostIsRefused(t *testing.T) {
	for _, u := range []string{
		"https://%67ithub.com/o/r/pull/212",
		"https://github%2ecom/o/r/pull/212",
		"https://%2567ithub.com/o/r/pull/212",
		"//%67ithub.com/o/r/pull/212",
		"https://ｇｉｔｈｕｂ.com/o/r/pull/212",
		"https://github。com/o/r/pull/212",
		"https://example.com:8080/o/r/pull/212",
		"https://user@example.com/x",
	} {
		g := &ghStub{base: "main", def: "main"}
		useGH(t, g)
		s := &stubBB{}
		q := "Check https://github.com/o/r/pull/211 but merge " + u + " ?"
		if _, err := fileMerge(t, s, askWithQuestion(t, q), "--pr", "https://github.com/o/r/pull/211"); exitCode(err) != 2 || s.creates != 0 {
			t.Errorf("%s: err=%v creates=%d", u, err, s.creates)
		}
	}
	// an ordinary link to another site is not a PR reference
	g := &ghStub{base: "main", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	q := "Merge https://github.com/o/r/pull/211 (notes: https://example.com/doc?a=1#top)"
	if _, err := fileMerge(t, s, askWithQuestion(t, q)); err != nil || s.creates != 1 {
		t.Fatalf("plain other-site link: err=%v creates=%d", err, s.creates)
	}
}

func TestNeedsMkIntoMatchingBase(t *testing.T) {
	g := &ghStub{base: "integration/v1", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	_, err := fileMerge(t, s, mergeAskJSON, "--pr", "https://github.com/o/r/pull/211", "--into", g.base)
	if err != nil {
		t.Fatalf("matching opt-in: %v (creates=%d)", err, s.creates)
	}
	if s.creates != 1 || len(g.calls) != 2 {
		t.Fatalf("creates=%d gh=%v", s.creates, g.calls)
	}
	for _, want := range []string{"Manually merge it at 58814293 into integration/v1", "Merging PR o/r#211 goes into integration/v1. This base is not the default branch main."} {
		if !strings.Contains(s.desc, want) {
			t.Errorf("card lacks %q:\n%s", want, s.desc)
		}
	}
}

func TestNeedsMkIntoRefusedWithoutOptIn(t *testing.T) {
	g := &ghStub{base: "integration/v1", def: "main"}
	useGH(t, g)
	s := &stubBB{}
	// A label claiming the default branch cannot bypass the real PR base.
	body := strings.Replace(mergeAskJSON, "58814293", "58814293 into main", 1)
	_, err := fileMerge(t, s, body, "--pr", "https://github.com/o/r/pull/211")
	if exitCode(err) != 2 || len(writeCalls(s)) != 0 || len(g.calls) != 2 {
		t.Fatalf("err=%v writes=%v gh=%v", err, writeCalls(s), g.calls)
	}
	for _, want := range []string{"--into", g.base, g.def, "nothing was filed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func TestNeedsMkIntoMismatch(t *testing.T) {
	for _, base := range []string{"integration/v1", "main"} {
		for _, into := range []string{"other", strings.ToUpper(base), " " + base, base + " ", ""} {
			t.Run(base+"/"+into, func(t *testing.T) {
				g := &ghStub{base: base, def: "main"}
				useGH(t, g)
				s := &stubBB{}
				_, err := fileMerge(t, s, mergeAskJSON, "--pr", "https://github.com/o/r/pull/211", "--into", into)
				if exitCode(err) != 2 || len(writeCalls(s)) != 0 || len(g.calls) != 2 {
					t.Fatalf("err=%v writes=%v gh=%v", err, writeCalls(s), g.calls)
				}
				for _, want := range []string{"--into", `"` + into + `"`, `"` + base + `"`, "nothing was filed"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q lacks %q", err, want)
					}
				}
			})
		}
	}
}

func TestNeedsMkIntoDefaultBehavior(t *testing.T) {
	for _, base := range []string{"main", "master"} {
		for _, body := range []string{mergeAskJSON, plainAskJSON} {
			var without string
			for _, explicit := range []bool{false, true} {
				g := &ghStub{base: base, def: base}
				useGH(t, g)
				s := &stubBB{}
				args := []string{"--pr", "https://github.com/o/r/pull/211"}
				if explicit {
					args = append(args, "--into", base)
				}
				if _, err := fileMerge(t, s, body, args...); err != nil {
					t.Fatal(err)
				}
				if s.creates != 1 || !strings.Contains(s.desc, "into "+base) || strings.Contains(s.desc, "not the default branch") {
					t.Fatalf("default behavior changed: creates=%d card=%s", s.creates, s.desc)
				}
				if body == mergeAskJSON && strings.Contains(s.desc, "Merging PR") {
					t.Fatalf("default merge option gained a question line: %s", s.desc)
				}
				if !explicit {
					without = s.desc
				} else if s.desc != without {
					t.Fatalf("explicit default changed card:\n%s\n%s", without, s.desc)
				}
			}
		}
	}
}

func TestNeedsMkIntoWithRewordedOrAbsentMergeLabel(t *testing.T) {
	for _, body := range []string{plainAskJSON, strings.Replace(mergeAskJSON, "Manually merge it at", "Land it at", 1)} {
		g := &ghStub{base: "integration/v1", def: "main"}
		useGH(t, g)
		s := &stubBB{}
		if _, err := fileMerge(t, s, body, "--pr", "https://github.com/o/r/pull/211"); exitCode(err) != 2 || len(writeCalls(s)) != 0 {
			t.Fatalf("reworded label bypassed base check: %v", err)
		}
		s = &stubBB{}
		if _, err := fileMerge(t, s, body, "--pr", "https://github.com/o/r/pull/211", "--into", g.base); err != nil {
			t.Fatal(err)
		}
		if s.creates != 1 || !strings.Contains(s.desc, "Merging PR o/r#211 goes into integration/v1. This base is not the default branch main.") {
			t.Fatalf("card=%s creates=%d", s.desc, s.creates)
		}
	}
}

func TestNeedsMkIntoOnNonMergeWarnsAndStillFiles(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thread-x")
	g := &ghStub{}
	useGH(t, g)
	var baseline string
	for _, into := range []bool{false, true} {
		s := &stubBB{}
		oldFiler, oldCheck := newCardFiler, checkFilingPreconditions
		t.Cleanup(func() { newCardFiler, checkFilingPreconditions = oldFiler, oldCheck })
		newCardFiler = func() *homeask.CardFiler { return &homeask.CardFiler{Run: s.run, LockDir: t.TempDir()} }
		checkFilingPreconditions = func(string, string) error { return nil }
		cmd := needsMkFileCmd()
		var stderr, stdout bytes.Buffer
		cmd.SetErr(&stderr)
		cmd.SetOut(&stdout)
		args := []string{"--project", "P1", "--title", "Review?", "--ask-file", writeMergeAsk(t, plainAskJSON)}
		if into {
			args = append(args, "--into", "sync/x")
		}
		cmd.SetArgs(args)
		err := cmd.Execute()
		if exitCode(err) != 0 || s.creates != 1 {
			t.Fatalf("into=%v: err=%v creates=%d", into, err, s.creates)
		}
		if into {
			if s.desc != baseline {
				t.Errorf("--into changed card: %s", s.desc)
			}
			for _, want := range []string{"warning", "--into", "ignored"} {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr lacks %q: %q", want, stderr.String())
				}
			}
		} else {
			baseline = s.desc
			if stderr.Len() != 0 {
				t.Errorf("unexpected warning: %s", &stderr)
			}
		}
	}
	if len(g.calls) != 0 {
		t.Fatalf("unexpected gh calls: %v", g.calls)
	}
}
