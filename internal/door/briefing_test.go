package door

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEncodeTranscriptDir(t *testing.T) {
	cases := map[string]string{
		"/Users/sma/projects/Sylveste":                                     "-Users-sma-projects-Sylveste",
		"/Users/sma/projects/Sylveste/apps/Autarch/.claude/worktrees/door": "-Users-sma-projects-Sylveste-apps-Autarch--claude-worktrees-door",
	}
	for in, want := range cases {
		if got := encodeTranscriptDir(in); got != want {
			t.Fatalf("encodeTranscriptDir(%q) = %q, want %q", in, got, want)
		}
	}
}

// touch creates path (and its directory) as a transcript whose last real turn
// is at, with the mtime set to match.
func touch(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"user","timestamp":"` + at.UTC().Format(time.RFC3339Nano) + `","message":{"role":"user"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestIndexSessionsUsesLastTurnNotMtime(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	ps := []Project{{Name: "foo", Root: "/e/foo"}}
	stale := filepath.Join(root, "-e-foo", "stale.jsonl")
	touch(t, stale, now.Add(-10*24*time.Hour))          // last real turn ten days ago...
	if err := os.Chtimes(stale, now, now); err != nil { // ...but touched just now, as bookkeeping rows do
		t.Fatal(err)
	}
	idx, err := IndexSessions(root, ps, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx["/e/foo"]; ok {
		t.Fatal("a transcript whose last real turn is ten days old must not count as moved because its mtime is fresh")
	}
}

func TestIndexSessionsRollsUpNestedAndKeepsSiblingsApart(t *testing.T) {
	root := t.TempDir()
	since := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	old, fresh := since.Add(-time.Hour), since.Add(time.Hour)
	ps := []Project{
		{Name: "foo", Root: "/e/foo"},
		{Name: "foo-bar", Root: "/e/foo-bar"},
		{Name: "quiet", Root: "/e/quiet"},
	}
	touch(t, filepath.Join(root, "-e-foo", "a.jsonl"), fresh)
	touch(t, filepath.Join(root, "-e-foo", "b.jsonl"), old)          // before the window
	touch(t, filepath.Join(root, "-e-foo", "notes.txt"), fresh)      // not a transcript
	touch(t, filepath.Join(root, "-e-foo-apps-x", "c.jsonl"), fresh) // nested repo rolls up
	touch(t, filepath.Join(root, "-e-foo-bar", "d.jsonl"), fresh)    // sibling, not nested
	touch(t, filepath.Join(root, "-e-elsewhere", "e.jsonl"), fresh)  // not in the estate

	idx, err := IndexSessions(root, ps, since)
	if err != nil {
		t.Fatal(err)
	}
	if got := idx["/e/foo"].Files; got != 2 {
		t.Fatalf("foo: want 2 fresh transcripts (own + nested), got %d", got)
	}
	if !idx["/e/foo"].Latest.Equal(fresh) {
		t.Fatalf("foo latest = %v, want %v", idx["/e/foo"].Latest, fresh)
	}
	if got := idx["/e/foo-bar"].Files; got != 1 {
		t.Fatalf("foo-bar: want 1, got %d -- foo must not absorb its sibling", got)
	}
	if _, ok := idx["/e/quiet"]; ok {
		t.Fatal("a garden with no transcripts must be absent from the index, not present at zero")
	}
	if _, err := IndexSessions(filepath.Join(root, "nope"), ps, since); err == nil {
		t.Fatal("an unreadable transcripts root is a failure to look, not an empty estate")
	}
}

// gitEnv isolates test repositories from mk's global git config: no signing,
// no hooks path, a fixed identity, plus any dates the caller pins.
func gitEnv(extra ...string) []string {
	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	return append(env, extra...)
}

func gitRun(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGitMovementCountsOnlyTheWindow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, gitEnv(), "init", "-q")
	writeFile(t, filepath.Join(dir, "a.txt"), "a")
	gitRun(t, dir, gitEnv(), "add", "a.txt")
	gitRun(t, dir, gitEnv("GIT_AUTHOR_DATE=2026-08-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-08-01T00:00:00Z"),
		"commit", "-q", "-m", "old work")
	writeFile(t, filepath.Join(dir, "b.txt"), "b")
	gitRun(t, dir, gitEnv(), "add", "b.txt")
	gitRun(t, dir, gitEnv("GIT_AUTHOR_DATE=2026-09-01T12:00:00Z", "GIT_COMMITTER_DATE=2026-09-01T12:00:00Z"),
		"commit", "-q", "-m", "new work")

	since := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	mv := GitMovement(context.Background(), Project{Name: "g", Root: dir}, since)
	if mv.Err != nil {
		t.Fatalf("unexpected error: %v", mv.Err)
	}
	if len(mv.Commits) != 1 || mv.Commits[0].Subject != "new work" {
		t.Fatalf("want exactly the commit inside the window, got %+v", mv.Commits)
	}
	if want := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC); !mv.Latest.Equal(want) {
		t.Fatalf("Latest = %v, want %v", mv.Latest, want)
	}
	if mv.Dirty != 0 || !mv.Moved() {
		t.Fatalf("clean tree with one new commit: Dirty=%d Moved=%v", mv.Dirty, mv.Moved())
	}

	// Work in flight counts as movement even when nothing was committed
	// inside the window.
	writeFile(t, filepath.Join(dir, "c.txt"), "c")
	mv = GitMovement(context.Background(), Project{Name: "g", Root: dir}, time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if mv.Err != nil || len(mv.Commits) != 0 || mv.Dirty != 1 || !mv.Moved() {
		t.Fatalf("an untracked file must read as 1 uncommitted and Moved: %+v", mv)
	}
}

func TestGitMovementUnreadIsNeverQuiet(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	mv := GitMovement(context.Background(), Project{Name: "x", Root: t.TempDir()}, time.Now().Add(-time.Hour))
	if mv.Err == nil {
		t.Fatal("a directory that is not a repository must carry an error")
	}
	if mv.Moved() {
		t.Fatal("an unread garden must never count as moved")
	}
}

func TestMovementsReportsEveryGarden(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	var ps []Project
	for i := 0; i < 12; i++ {
		ps = append(ps, Project{Name: fmt.Sprint("g", i), Root: t.TempDir()})
	}
	var mu sync.Mutex
	got := 0
	Movements(context.Background(), ps, time.Now().Add(-time.Hour), nil, func(Movement) {
		mu.Lock()
		got++
		mu.Unlock()
	})
	if got != len(ps) {
		t.Fatalf("Movements reported %d of %d -- a dropped garden is a silent coverage gap", got, len(ps))
	}
}

// briefingModel is a door with the briefing on, a fixed clock, and a window
// that opened 26 hours ago.
func briefingModel(t *testing.T, ps []Project, now time.Time) Model {
	t.Helper()
	m := NewModel(ps, Ranking{}, "", nil, "checker", nil).WithBriefing(BriefingOptions{
		Since:           now.Add(-26 * time.Hour),
		SinceSource:     "last visit",
		TranscriptsRoot: t.TempDir(),
		Now:             func() time.Time { return now },
	})
	m.width, m.height = 110, 30
	return m
}

func feed(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func key(s string) tea.KeyMsg {
	if s == "tab" {
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestBriefingListsOnlyWhatMovedNewestFirst(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	ps := []Project{
		{Name: "alpha", Root: "/e/alpha", Verdict: VerdictConfirmed, Strength: Strength{Score: 6, Of: 6}},
		{Name: "quietgarden", Root: "/e/quietgarden", Verdict: VerdictAbsent},
		{Name: "broken", Root: "/e/broken", Verdict: VerdictAbsent},
		{Name: "delta", Root: "/e/delta", Verdict: VerdictAbsent},
	}
	m := briefingModel(t, ps, now)
	since := m.window
	m = feed(t, m,
		movementsStartedMsg{count: 4},
		movementMsg{m: Movement{Root: "/e/delta", Name: "delta", Since: since, Sessions: 1, Latest: now.Add(-3 * time.Hour)}},
		movementMsg{m: Movement{Root: "/e/quietgarden", Name: "quietgarden", Since: since}},
		movementMsg{m: Movement{Root: "/e/broken", Name: "broken", Since: since, Err: errors.New("not a repository")}},
		movementMsg{m: Movement{Root: "/e/alpha", Name: "alpha", Since: since, Dirty: 2, Latest: now.Add(-time.Hour),
			Commits: []Commit{
				{Hash: "abc", Subject: "docs: all four CUJs validated", When: now.Add(-time.Hour)},
				{Hash: "def", Subject: "older", When: now.Add(-2 * time.Hour)},
			}}},
	)
	view := m.View()
	for _, want := range []string{
		"since", "last visit", "2 of 4 gardens moved", "2 commits", "2 uncommitted",
		"1 session", "docs: all four CUJs validated", "1h ago", "could not read: broken",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("briefing missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "quietgarden") {
		t.Fatalf("a garden that did not move must not be listed:\n%s", view)
	}
	if strings.Contains(view, "CONFIRMED") {
		t.Fatalf("alone layout must not show verdict rows before the rows are asked for:\n%s", view)
	}
	if strings.Index(view, "alpha") > strings.Index(view, "delta") {
		t.Fatalf("newest movement must come first:\n%s", view)
	}
	if strings.Contains(view, "reading") {
		t.Fatalf("all four reported; the header must not still say reading:\n%s", view)
	}
}

func TestBriefingNothingMovedAndUnreadSessionsRoot(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	ps := []Project{{Name: "a", Root: "/e/a", Verdict: VerdictAbsent}}
	m := briefingModel(t, ps, now)
	since := m.window
	view := m.View()
	if strings.Contains(view, "nothing moved") {
		t.Fatalf("before any garden is read, nothing-moved is a claim nobody measured:\n%s", view)
	}
	m = feed(t, m,
		movementsStartedMsg{count: 1, sessionsErr: errors.New("transcripts root missing")},
		movementMsg{m: Movement{Root: "/e/a", Name: "a", Since: since}},
	)
	view = m.View()
	if !strings.Contains(view, "nothing moved since") {
		t.Fatalf("a fully-read quiet estate must say so:\n%s", view)
	}
	if !strings.Contains(view, "claude sessions: UNCHECKED") {
		t.Fatalf("an unreadable transcripts root must read as UNCHECKED, not as zero sessions:\n%s", view)
	}
}

func TestLayoutsAndScreens(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	ps := []Project{{Name: "alpha", Root: "/e/alpha", Verdict: VerdictConfirmed, Strength: Strength{Score: 6, Of: 6}}}
	m := briefingModel(t, ps, now)

	view := m.View()
	if strings.Contains(view, "CONFIRMED") || !strings.Contains(view, "last visit") {
		t.Fatalf("alone layout opens on the briefing, rows hidden:\n%s", view)
	}
	m = feed(t, m, key("tab"))
	view = m.View()
	if !strings.Contains(view, "CONFIRMED") || strings.Contains(view, "last visit") {
		t.Fatalf("tab must reach the rows and leave the briefing behind:\n%s", view)
	}
	m = feed(t, m, key("b"))
	view = m.View()
	if !strings.Contains(view, "CONFIRMED") || !strings.Contains(view, "last visit") {
		t.Fatalf("above layout shows briefing and rows on one screen:\n%s", view)
	}
	if m.layout != LayoutAbove {
		t.Fatalf("b must flip the layout, got %s", m.layout)
	}
}

func TestBriefingScreenHasNoActions(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	ps := []Project{
		{Name: "a", Root: "/e/a", Verdict: VerdictAbsent},
		{Name: "b", Root: "/e/b", Verdict: VerdictAbsent},
	}
	m := briefingModel(t, ps, now)
	sel := m.selRoot
	m = feed(t, m, key("j"), key("G"))
	if m.selRoot != sel || m.moved {
		t.Fatalf("navigation on the briefing screen must be inert: sel %s -> %s, moved=%v", sel, m.selRoot, m.moved)
	}
}

func TestWidenCyclesAndReturns(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	ps := []Project{{Name: "a", Root: "/e/a", Verdict: VerdictAbsent}}
	m := briefingModel(t, ps, now)
	configured := m.window

	want := []struct {
		since  time.Time
		source string
	}{
		{now.Add(-24 * time.Hour), "widened to 24h"},
		{now.Add(-72 * time.Hour), "widened to 3d"},
		{now.Add(-7 * 24 * time.Hour), "widened to 7d"},
		{now.Add(-30 * 24 * time.Hour), "widened to 30d"},
		{configured, "last visit"},
	}
	for i, w := range want {
		m.moveRemaining = 0 // The previous read has completed before the next key.
		m = feed(t, m, key("w"))
		if !m.window.Equal(w.since) || m.windowSource != w.source {
			t.Fatalf("step %d: window %v %q, want %v %q", i, m.window, m.windowSource, w.since, w.source)
		}
	}
}

func TestQuitSavesVisit(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "sub", "last-visit")
	ps := []Project{{Name: "a", Root: "/e/a", Verdict: VerdictAbsent}}
	m := NewModel(ps, Ranking{}, "", nil, "checker", nil).WithBriefing(BriefingOptions{
		Since:       now.Add(-time.Hour),
		SinceSource: "last visit",
		VisitPath:   path,
		Now:         func() time.Time { return now },
	})
	_, cmd := m.Update(key("q"))
	if cmd == nil {
		t.Fatal("q must quit")
	}
	got, ok, err := LoadLastVisit(path)
	if err != nil || !ok {
		t.Fatalf("quit must stamp the visit: ok=%v err=%v", ok, err)
	}
	if !got.Equal(now) {
		t.Fatalf("stamp = %v, want %v", got, now)
	}
}
