package door

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
)

const paneFixture = "/tmp/door.sock\x1f42\x1f1700000000\x1f$1\x1f@2\x1f%3\x1f100\x1f0\x1fwork\x1feditor\x1f12345\x1f/est/a\x1fcodex\n"

func TestAllPanesReachThreadNavigationAndGrouping(t *testing.T) {
	out := paneFixture + strings.ReplaceAll(strings.ReplaceAll(paneFixture, "%3", "%4"), "@2", "@5")
	sessions, e := parseSessionLines(out)
	if e != nil {
		t.Fatal(e)
	}
	if len(sessions) != 2 {
		t.Fatalf("want both panes including inactive window: %+v", sessions)
	}
	var threads []Thread
	ReadThreads(context.Background(), sessions, []Project{{Root: "/est/a"}}, nil, "", func(th Thread) { // callbacks can be concurrent
		_ = th
	})
	// Construct snapshots directly to test selection independently of async order.
	for _, s := range sessions {
		threads = append(threads, Thread{Session: s.Name, Target: s.Target, Path: s.Path, Runtime: RuntimeCodex})
	}
	m := NewModel([]Project{{Root: "/est/a"}}, Ranking{}, "", nil, "", nil)
	m.threads = ThreadSet{Threads: threads}
	m.finishThreads()
	next, _ := m.handleThreadsKey("down")
	m = next.(Model)
	if m.threadIndex() != 1 {
		t.Fatal("selection collapsed two panes with the same session name")
	}
	if got := m.sessions.ByRoot["/est/a"]; len(got) != 2 || got[0].Target == got[1].Target {
		t.Fatalf("group lost exact identity: %+v", got)
	}
	groups := GroupPanes(sessions, []Project{{Root: "/est/a"}}, "@5")
	if len(groups) != 1 || len(groups[0].Panes) != 1 || groups[0].Panes[0].Target.WindowID != "@5" {
		t.Fatalf("pane search: %+v", groups)
	}
}
func TestSplitPanesDoNotInheritSessionTranscript(t *testing.T) {
	transcripts := t.TempDir()
	id := "aaaa1111-bbbb-2222-cccc-333344445555"
	path := filepath.Join(transcripts, "project", id+".jsonl")
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(userRequest+"\n"+assistantContext+"\n"+questionTool+"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	name := "iterm[reader - " + id
	sessions := []TmuxSession{{Name: name, Path: "/est", Command: "claude", Target: agenttransport.Target{Socket: "/tmp/s", SessionID: "$1", PaneID: "%1"}}, {Name: name, Path: "/est", Command: "codex", Target: agenttransport.Target{Socket: "/tmp/s", SessionID: "$1", PaneID: "%2"}}}
	var mu sync.Mutex
	var got []Thread
	ReadThreads(context.Background(), sessions, []Project{{Root: "/est/a"}}, nil, transcripts, func(th Thread) { mu.Lock(); defer mu.Unlock(); got = append(got, th) })
	for _, th := range got {
		if th.Transcript != "" || th.Conversation.Source != "" || len(th.Gardens) > 0 {
			t.Fatalf("pane inherited another pane's evidence: %+v", th)
		}
	}
}

func TestPaneSearchOwnsGlobalShortcutCharacters(t *testing.T) {
	m := NewModel(nil, Ranking{}, "", nil, "", nil).WithThreads(ThreadsOptions{})
	m.screen = screenThreads
	next, _ := m.handleThreadsKey("/")
	m = next.(Model)
	for _, r := range "codex @5" {
		next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	if m.threadQuery != "codex @5" || m.screen != screenThreads {
		t.Fatalf("query intercepted by global keys: %q", m.threadQuery)
	}
}
func TestPaneSearchEscapeClearsFilterAfterBrowsing(t *testing.T) {
	m := NewModel(nil, Ranking{}, "", nil, "", nil).WithThreads(ThreadsOptions{})
	m.screen = screenThreads
	m.threadQuery = "codex"
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.threadQuery != "" || cmd != nil {
		t.Fatal("escape quit instead of clearing pane search")
	}
}
func TestSharedWorkspacePaneRemainsUnassigned(t *testing.T) {
	p := TmuxSession{Name: "shared", Path: "/est", Target: agenttransport.Target{PaneID: "%1"}}
	groups := GroupPanes([]TmuxSession{p}, []Project{{Root: "/est/a"}, {Root: "/est/b"}}, "")
	if len(groups) != 1 || groups[0].Root != "" {
		t.Fatalf("inferred shared workspace ownership: %+v", groups)
	}
}

func TestThreadInventoryIsGroupedByCWDProjectWithUnassignedLast(t *testing.T) {
	projects := []Project{{Name: "a", Root: "/est/a"}, {Name: "b", Root: "/est/b"}}
	threads := []Thread{
		{Session: "shared", Path: "/est", Activity: 30, Target: agenttransport.Target{PaneID: "%3"}},
		{Session: "b", Path: "/est/b", Activity: 20, Target: agenttransport.Target{PaneID: "%2"}},
		{Session: "a", Path: "/est/a", Activity: 10, Target: agenttransport.Target{PaneID: "%1"}},
	}
	m := NewModel(projects, Ranking{}, "", nil, "", nil)
	m.threads = ThreadSet{Threads: threads}
	m.finishThreads()

	got := m.threads.Threads
	if len(got) != 3 || got[0].Project != "/est/a" || got[1].Project != "/est/b" || got[2].Project != "" {
		t.Fatalf("panes were not grouped by project with shared cwd unassigned: %+v", got)
	}
	if columnsFor(got[0], m.now()).gardens != "a" || columnsFor(got[2], m.now()).gardens != "unassigned" {
		t.Fatalf("group labels are not visible: %+v %+v", columnsFor(got[0], m.now()), columnsFor(got[2], m.now()))
	}
}
