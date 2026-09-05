package door

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The Ubuntu CI tmux prints the unit separator as an octal escape, while
// tmux 3.6b on macOS emits the control byte. Both are real CLI responses.
func TestListSessionsParsesEscapedSeparators(t *testing.T) {
	out := `1\0371\037iterm[reader - abc\0371788564126\037/tmp/reader\037bash` + "\n"
	got, err := parseSessionLines(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "iterm[reader - abc" || got[0].Path != "/tmp/reader" || got[0].Command != "bash" {
		t.Fatalf("wrong tmux fields: %+v", got)
	}
}

func TestListSessionsDistinguishesStoppedServerFromConnectionFailure(t *testing.T) {
	bin := t.TempDir()
	writeExec(t, filepath.Join(bin, "tmux"), "#!/bin/sh\nprintf '%s\\n' \"$TMUX_TEST_ERROR\" >&2\nexit 1\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tc := range []struct {
		message   string
		wantError bool
	}{
		{"no server running on /tmp/tmux-1000/default", false},
		{"error connecting to /tmp/tmux-1000/default (No such file or directory)", false},
		{"error connecting to /tmp/tmux-1000/default (Connection refused)", false},
		{"error connecting to /tmp/tmux-1000/default (Permission denied)", true},
		{"error connecting to /tmp/tmux-1000/default (Protocol wrong type for socket)", true},
	} {
		t.Setenv("TMUX_TEST_ERROR", tc.message)
		sessions, err := ListSessions(context.Background())
		if (err != nil) != tc.wantError {
			t.Errorf("%q: sessions=%v err=%v", tc.message, sessions, err)
		}
	}
}

func TestResolveSessionsCountsAndNamesUnresolved(t *testing.T) {
	projects := []Project{
		{Name: "aaa", Root: "/est/aaa"},
		{Name: "bbb", Root: "/est/bbb"},
	}
	sessions := []TmuxSession{
		{Name: "work1", Path: "/est/aaa", Activity: 10},
		{Name: "work2", Path: "/est/aaa/sub/worktree", Activity: 20},
		{Name: "other", Path: "/est/bbb", Activity: 5},
		{Name: "stray", Path: "/somewhere/else", Activity: 7},
	}
	set := ResolveSessions(sessions, projects)

	if set.Total != 4 || set.Resolved != 3 {
		t.Fatalf("fraction = %d/%d, want 3/4", set.Resolved, set.Total)
	}
	if n := set.Count("/est/aaa"); n != 2 {
		t.Fatalf("aaa count = %d, want 2", n)
	}
	if n := set.Count("/est/bbb"); n != 1 {
		t.Fatalf("bbb count = %d, want 1", n)
	}
	// The GATE clause: every unresolvable session is named, none dropped.
	if len(set.Unresolved) != 1 || set.Unresolved[0] != "stray" {
		t.Fatalf("unresolved = %v, want [stray]", set.Unresolved)
	}
}

// A root must only claim paths at a path-component boundary: /est/aa does not
// contain /est/aab, and a bare string prefix would silently misfile sessions.
func TestResolveSessionsPrefixBoundary(t *testing.T) {
	projects := []Project{{Name: "aa", Root: "/est/aa"}}
	set := ResolveSessions([]TmuxSession{{Name: "s", Path: "/est/aab"}}, projects)
	if set.Resolved != 0 || len(set.Unresolved) != 1 {
		t.Fatalf("boundary violated: %+v", set)
	}
}

func TestTargetPicksMostRecentlyActive(t *testing.T) {
	projects := []Project{{Name: "aaa", Root: "/est/aaa"}}
	set := ResolveSessions([]TmuxSession{
		{Name: "old", Path: "/est/aaa", Activity: 10},
		{Name: "fresh", Path: "/est/aaa", Activity: 99},
	}, projects)
	target, ok := set.Target("/est/aaa")
	if !ok || target.Name != "fresh" {
		t.Fatalf("target = %v ok=%v, want fresh", target, ok)
	}
	if _, ok := set.Target("/est/none"); ok {
		t.Fatal("a root with no sessions must report no target")
	}
}

func TestSessionsLineStatesFractionAndNamesEveryUnresolvable(t *testing.T) {
	line := sessionsLine(SessionSet{Total: 3, Resolved: 2, Unresolved: []string{"stray"}})
	if !strings.Contains(line, "2/3") {
		t.Fatalf("no fraction: %q", line)
	}
	if !strings.Contains(line, "stray") {
		t.Fatalf("unresolvable session dropped silently: %q", line)
	}

	clean := sessionsLine(SessionSet{Total: 2, Resolved: 2})
	if strings.Contains(clean, "unresolved") {
		t.Fatalf("fully-resolved set should not warn: %q", clean)
	}

	// Could-not-look is its own state, never a zero.
	failed := sessionsLine(SessionSet{Err: exec.ErrNotFound})
	if !strings.Contains(failed, "UNCHECKED") || strings.Contains(failed, "0/0") {
		t.Fatalf("failure must read UNCHECKED, not a measurement: %q", failed)
	}

	none := sessionsLine(SessionSet{})
	if !strings.Contains(none, "0/0") {
		t.Fatalf("a stopped server is a true zero: %q", none)
	}
}

// Gate B clauses (a) and (c) at the view level: the row shows the count, the
// footer shows both fractions with every unresolvable named.
func TestViewShowsSessionCountAndBothFractions(t *testing.T) {
	ps := []Project{
		{Name: "aaa", Root: "/est/aaa", Verdict: VerdictConfirmed, Strength: Strength{Score: 6, Of: 6}},
		{Name: "bbb", Root: "/est/bbb", Verdict: VerdictAbsent},
	}
	m := NewModel(ps, Ranking{}, "", nil, "checker", nil)
	m.width, m.height = 100, 20

	next, _ := m.Update(sessionsMsg{set: ResolveSessions([]TmuxSession{
		{Name: "w1", Path: "/est/aaa", Activity: 2},
		{Name: "w2", Path: "/est/aaa", Activity: 1},
		{Name: "stray", Path: "/nowhere", Activity: 3},
	}, ps)})
	m = next.(Model)
	view := m.View()

	if !strings.Contains(view, "◆2") {
		t.Fatalf("row missing session count:\n%s", view)
	}
	if !strings.Contains(view, "cards: 1 confirmed") {
		t.Fatalf("footer missing cards fraction:\n%s", view)
	}
	if !strings.Contains(view, "sessions: 2/3 resolved") {
		t.Fatalf("footer missing sessions fraction:\n%s", view)
	}
	if !strings.Contains(view, "unresolved: stray") {
		t.Fatalf("footer dropped an unresolvable session silently:\n%s", view)
	}
}

func TestViewSessionsUncheckedIsNamedNeverZero(t *testing.T) {
	ps := []Project{{Name: "aaa", Root: "/est/aaa", Verdict: VerdictAbsent}}
	m := NewModel(ps, Ranking{}, "", nil, "checker", nil)
	m.width, m.height = 100, 20

	before := m.View()
	if !strings.Contains(before, "sessions: checking…") {
		t.Fatalf("pre-snapshot view must say it has not looked yet:\n%s", before)
	}

	next, _ := m.Update(sessionsMsg{set: SessionSet{Err: exec.ErrNotFound}})
	m = next.(Model)
	view := m.View()
	if !strings.Contains(view, "sessions: UNCHECKED") {
		t.Fatalf("failed snapshot must read UNCHECKED:\n%s", view)
	}
	if strings.Contains(view, "◆") {
		t.Fatalf("no row may show a count nobody measured:\n%s", view)
	}
}

// Enter falls back to the card only when the row has no live session; the
// switch path itself is exercised end-to-end by the tmux capture test.
func TestEnterRoutesBySessionPresence(t *testing.T) {
	ps := []Project{{Name: "aaa", Root: "/est/aaa", Verdict: VerdictAbsent}}
	m := NewModel(ps, Ranking{}, "", nil, "checker", nil)
	m.width, m.height = 100, 20

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("enter with no sessions must still open the card (backfill on first touch)")
	}

	next, _ = m.Update(sessionsMsg{set: ResolveSessions(
		[]TmuxSession{{Name: "w", Path: "/est/aaa"}}, ps)})
	m = next.(Model)
	if _, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Fatal("enter with a live session must produce a switch command")
	}
}

// TestListSessionsParsesSixFields is WI-2's parser contract: the sixth field
// (pane_current_command) comes through, and a line with the old five-field
// shape is rejected rather than silently misparsed.
func TestListSessionsParsesSixFields(t *testing.T) {
	out := "1\x1f1\x1fwork\x1f12345\x1f/est/aaa\x1f2.1.258\n" +
		"0\x1f0\x1fwork\x1f12345\x1f/est/aaa\x1fzsh\n" // not window_active+pane_active: dropped
	sessions, err := parseSessionLines(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 session (the other line is filtered by active flags), got %d: %+v", len(sessions), sessions)
	}
	if sessions[0].Command != "2.1.258" {
		t.Fatalf("Command not parsed: %+v", sessions[0])
	}

	if _, err := parseSessionLines("1\x1f1\x1fwork\x1f12345\x1f/est/aaa\n"); err == nil {
		t.Fatal("a five-field line (the old shape) must be rejected, not silently accepted")
	}
}

// ListSessions' error contract: a missing binary is an error (could not
// look); with tmux present, both a running and a stopped server are answers.
func TestListSessionsLiveContract(t *testing.T) {
	sessions, err := ListSessions(context.Background())
	if _, lookErr := exec.LookPath("tmux"); lookErr != nil {
		if err == nil || !strings.Contains(err.Error(), "tmux not on PATH") {
			t.Fatalf("missing tmux must be an error, got sessions=%v err=%v", sessions, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("live tmux query failed: %v", err)
	}
	for _, s := range sessions {
		if s.Name == "" || s.Path == "" {
			t.Fatalf("malformed session from live server: %+v", s)
		}
	}
}
