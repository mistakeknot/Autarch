package door

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func threadsTestModel(t *testing.T, ps []Project) Model {
	t.Helper()
	m := NewModel(ps, Ranking{}, "", nil, "checker", nil).WithThreads(ThreadsOptions{TranscriptsRoot: t.TempDir()})
	m.width, m.height = 100, 30
	return m
}

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestThreadsScreenToggleAndReturn(t *testing.T) {
	ps := []Project{{Name: "a", Root: "/e/a"}}

	// Rows-only door: t opens the threads screen, t returns to the rows.
	m := threadsTestModel(t, ps)
	next, _ := m.Update(keyRune('t'))
	m = next.(Model)
	if m.screen != screenThreads {
		t.Fatal("t must open the threads screen")
	}
	if !strings.Contains(m.View(), "threads:") {
		t.Fatal("the threads screen must state the seats header")
	}
	next, _ = m.Update(keyRune('t'))
	m = next.(Model)
	if m.screen == screenThreads {
		t.Fatal("t on the threads screen must return to the previous screen")
	}

	// Briefing on, LayoutAlone: the briefing stays first; t opens threads
	// from it and tab returns to it, not to the rows.
	mb := NewModel(ps, Ranking{}, "", nil, "checker", nil).
		WithBriefing(BriefingOptions{Since: time.Now().Add(-time.Hour), SinceSource: "test", TranscriptsRoot: t.TempDir()}).
		WithThreads(ThreadsOptions{TranscriptsRoot: t.TempDir()})
	mb.width, mb.height = 100, 30
	if mb.screen != screenBriefing {
		t.Fatal("with the briefing on, the opening screen is still the briefing")
	}
	next, _ = mb.Update(keyRune('t'))
	mb = next.(Model)
	if mb.screen != screenThreads {
		t.Fatal("t from the briefing must open the threads screen")
	}
	next, _ = mb.Update(tea.KeyMsg{Type: tea.KeyTab})
	mb = next.(Model)
	if mb.screen != screenBriefing {
		t.Fatalf("tab on the threads screen must return to the briefing it came from, got %v", mb.screen)
	}
}

func TestThreadsRenderStatesVisiblyDistinct(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	m := threadsTestModel(t, []Project{{Name: "solwend", Root: "/e/solwend"}})
	m.briefing.Now = func() time.Time { return now }
	m.threads = ThreadSet{Threads: []Thread{
		{Session: "rio]solwend - 21cc6bd2", Seat: ParseSeat("rio]solwend - 21cc6bd2"), Runtime: RuntimeClaude, Version: "2.1.258", Activity: 9,
			Transcript: "/t/a", LastTurn: now.Add(-2 * time.Hour), Gardens: []GardenHit{{Root: "/e/solwend", Name: "solwend", Mentions: 10}}},
		{Session: "rio[taxes - 5a729c1d", Seat: ParseSeat("rio[taxes - 5a729c1d"), Runtime: RuntimeClaude, Version: "2.1.233", Activity: 8,
			Transcript: "/t/b", LastTurn: now.Add(-8 * 24 * time.Hour)},
		{Session: "28", Seat: ParseSeat("28"), Runtime: RuntimeShell, Activity: 7},
		{Session: "iterm[gone - deadbeef", Seat: ParseSeat("iterm[gone - deadbeef"), Runtime: RuntimeClaude, Version: "2.1.258", Activity: 6,
			Err: errors.New("no transcript for deadbeef")},
		{Session: "iterm[torn - beefdead", Seat: ParseSeat("iterm[torn - beefdead"), Runtime: RuntimeClaude, Version: "2.1.258", Activity: 5,
			Transcript: "/t/c", Err: errors.New("read: boom")},
		{Session: "rio[ryan", Seat: ParseSeat("rio[ryan"), Runtime: RuntimeClaude, Version: "2.1.258", Activity: 4},
	}, ByRoot: map[string][]Thread{}}
	m.threadsLoaded, m.sessionsLoaded = true, true
	m.screen = screenThreads

	view := m.View()
	for _, want := range []string{"2.1.258 2h", "idle 8d", "idle shell", "no transcript", "could not read", "2.1.258 no id", "solwend"} {
		if !strings.Contains(view, want) {
			t.Fatalf("threads screen must show %q; got:\n%s", want, view)
		}
	}
	head := "threads: 6 seats · 5 running (claude 5 · codex 0 · other 0) · 1 idle shells · 1 unmarked"
	if !strings.Contains(view, head) {
		t.Fatalf("header must count seats, runtimes, shells and unmarked names; want %q in:\n%s", head, view)
	}
	if plain := ThreadLine(m.threads.Threads[0], now, 120); !strings.Contains(plain, "solwend") || !strings.Contains(plain, "2.1.258 2h") {
		t.Fatalf("ThreadLine must carry the same columns as the screen: %q", plain)
	}
}

func TestThreadsScreenEnterTargetsSession(t *testing.T) {
	m := threadsTestModel(t, nil)
	m.threads = ThreadSet{Threads: []Thread{
		{Session: "iterm[a - 1", Seat: ParseSeat("iterm[a - 1"), Runtime: RuntimeClaude, Activity: 2},
		{Session: "iterm[b - 2", Seat: ParseSeat("iterm[b - 2"), Runtime: RuntimeClaude, Activity: 1},
	}, ByRoot: map[string][]Thread{}}
	m.threadsLoaded, m.sessionsLoaded = true, true
	m.screen = screenThreads

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.threadIndex() != 1 {
		t.Fatalf("down must select the second seat, got %d", m.threadIndex())
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Fatal("enter on a seat must return the switch command")
	}
}

func TestThreadsUncheckedShowsNoRows(t *testing.T) {
	m := threadsTestModel(t, []Project{{Name: "a", Root: "/e/a"}})
	next, _ := m.Update(sessionsListMsg{err: errors.New("tmux not on PATH")})
	m = next.(Model)
	m.screen = screenThreads
	view := m.View()
	if !strings.Contains(view, "UNCHECKED") {
		t.Fatalf("a tmux failure must read as UNCHECKED, got:\n%s", view)
	}
	if strings.Contains(view, "◆") {
		t.Fatal("no seat may be rendered when tmux could not be listed")
	}
	if m.sessions.Err == nil || !m.sessionsLoaded {
		t.Fatal("the rows' sessions axis must carry the same failure")
	}
}

func TestThreadsUpgradeGardenRowsAfterRead(t *testing.T) {
	ps := []Project{{Name: "a", Root: "/e/a"}}
	m := threadsTestModel(t, ps)
	id := "aaaa1111-bbbb-2222-cccc-333344445555"
	sessions := []TmuxSession{{Name: "iterm[a - " + id, Path: "/e", Activity: 5, Command: "2.1.258"}}

	next, _ := m.Update(sessionsListMsg{sessions: sessions})
	m = next.(Model)
	if m.sessions.Count("/e/a") != 0 {
		t.Fatal("a pane at the estate root resolves to no garden by path")
	}
	if m.threadsPending != 1 {
		t.Fatalf("one session to read, pending = %d", m.threadsPending)
	}

	th := Thread{Session: sessions[0].Name, Seat: ParseSeat(sessions[0].Name), Runtime: RuntimeClaude, Version: "2.1.258",
		Activity: 5, Path: "/e", Transcript: "/t", LastTurn: time.Now(), Gardens: []GardenHit{{Root: "/e/a", Name: "a", Mentions: 5}}}
	next, _ = m.Update(threadMsg{t: th})
	m = next.(Model)
	if !m.threadsLoaded || m.threadsPending != 0 {
		t.Fatal("the last pending thread must finish the read")
	}
	if m.sessions.Count("/e/a") != 1 {
		t.Fatal("attribution must upgrade the garden row's live-thread count")
	}
	if target, ok := m.sessions.Target("/e/a"); !ok || target.Name != sessions[0].Name {
		t.Fatalf("enter on the garden row must find the attributed thread, got %+v %v", target, ok)
	}
	if !strings.Contains(m.View(), "◆1") {
		t.Fatalf("the garden row must show one live thread after attribution:\n%s", m.View())
	}
}

func TestThreadsRegistryBlockShowsDrift(t *testing.T) {
	m := NewModel(nil, Ranking{}, "", nil, "checker", nil).
		WithThreads(ThreadsOptions{TranscriptsRoot: t.TempDir(), RegistryPath: "testdata/session-note.txt"})
	m.width, m.height = 120, 40
	if m.registryErr != nil || len(m.registry) != 52 {
		t.Fatalf("the note must parse on WithThreads: %d seats, %v", len(m.registry), m.registryErr)
	}
	m.threads = ThreadSet{Threads: threadsFromNames(
		"iterm[ushas/bridger - ef9ad21a-0965-45cb-b4bf-fda7f5a358b3",
		"iterm[]rakes-of-the-new-book - 5c7a44a3-4bff-461e-9ad2-06f0f7c7e18a",
	), ByRoot: map[string][]Thread{}}
	m.finishThreads()
	m.screen = screenThreads
	view := m.View()
	for _, want := range []string{"registry: testdata/session-note.txt", "stale id: ushas/bridger", "renamed: rakes-of-the-new-sun"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the registry block must show %q; got:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "+") || !strings.Contains(view, "more") {
		t.Fatal("drift beyond the cap must be counted, not hidden")
	}
}
