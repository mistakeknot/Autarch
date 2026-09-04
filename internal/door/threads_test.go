package door

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestParseSeatEveryShapeInTheNote is the table from the probe: every mark,
// every emulator, and the edge cases the note itself contains (no terminal,
// no id, a double space before the id, and bare topics with no bracket at
// all -- the note's own drift).
func TestParseSeatEveryShapeInTheNote(t *testing.T) {
	cases := []struct {
		name string
		want Seat
	}{
		{
			"wezterm[finorcs - d256fbdd-98c1-48f2-80fa-f58bfc618394",
			Seat{Terminal: "wezterm", Mark: MarkLeft, Topic: "finorcs", ResumeID: "d256fbdd-98c1-48f2-80fa-f58bfc618394"},
		},
		{
			"iterm]unc-rancher - 818281f9-9a63-460a-86f3-f2fb124648cb",
			Seat{Terminal: "iterm", Mark: MarkRight, Topic: "unc-rancher", ResumeID: "818281f9-9a63-460a-86f3-f2fb124648cb"},
		},
		{
			"iterm[]rakes-of-the-new-sun - 5c7a44a3-4bff-461e-9ad2-06f0f7c7e18a",
			Seat{Terminal: "iterm", Mark: MarkCenter, Topic: "rakes-of-the-new-sun", ResumeID: "5c7a44a3-4bff-461e-9ad2-06f0f7c7e18a"},
		},
		{
			"rio[jetty/fissionchips - 228255df-83a7-4fa2-8e19-334a7951d0cd",
			Seat{Terminal: "rio", Mark: MarkLeft, Topic: "jetty/fissionchips", ResumeID: "228255df-83a7-4fa2-8e19-334a7951d0cd"},
		},
		{
			// no terminal
			"[cujgel - 5920c9b1-6a3f-4a7d-8566-e6067aaeaf01",
			Seat{Terminal: "", Mark: MarkLeft, Topic: "cujgel", ResumeID: "5920c9b1-6a3f-4a7d-8566-e6067aaeaf01"},
		},
		{
			// no id
			"rio]garden-salon",
			Seat{Terminal: "rio", Mark: MarkRight, Topic: "garden-salon", ResumeID: ""},
		},
		{
			// double space before the id
			"iterm[jeddnet -  f44fd423-1cc0-4f6d-8bd1-a793d69facf4",
			Seat{Terminal: "iterm", Mark: MarkLeft, Topic: "jeddnet", ResumeID: "f44fd423-1cc0-4f6d-8bd1-a793d69facf4"},
		},
		{
			// plain number, no bracket at all
			"28",
			Seat{Topic: "28"},
		},
		{
			// bare topic, no bracket -- the note's grey-area drift
			"grey-area",
			Seat{Topic: "grey-area"},
		},
		{
			// bare topic, no bracket
			"kimifork",
			Seat{Topic: "kimifork"},
		},
	}
	for _, c := range cases {
		got := ParseSeat(c.name)
		if got != c.want {
			t.Errorf("ParseSeat(%q) = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestClassifyPane(t *testing.T) {
	cases := []struct {
		cmd         string
		wantRuntime Runtime
		wantVersion string
	}{
		{"2.1.258", RuntimeClaude, "2.1.258"},
		{"codex", RuntimeCodex, ""},
		{"kimi", RuntimeKimi, ""},
		{"zsh", RuntimeShell, ""},
		{"python3.11", RuntimeOther, ""},
		{"node", RuntimeOther, ""},
	}
	for _, c := range cases {
		gotRuntime, gotVersion := ClassifyPane(c.cmd)
		if gotRuntime != c.wantRuntime || gotVersion != c.wantVersion {
			t.Errorf("ClassifyPane(%q) = (%q, %q), want (%q, %q)",
				c.cmd, gotRuntime, gotVersion, c.wantRuntime, c.wantVersion)
		}
	}
}

// TestAttributeRootLaunchedThreadLandsOnItsGardens is WI-4's attribution
// contract for a thread with no resolvable pane path (launched at the
// estate root): the top garden is always credited, and any other garden
// clearing the 20% floor joins it. The plan's own illustrative numbers
// (A 80 / B 15 / C 5) put B at a 15% share, one point under the stated 20%
// floor, so this fixture uses A 70 / B 25 / C 5 instead -- the same shape
// (a dominant garden, a real secondary, a noise garden) with B actually
// clearing the floor the rule describes. Logged in the plan's Question
// ledger.
func TestAttributeRootLaunchedThreadLandsOnItsGardens(t *testing.T) {
	projects := []Project{
		{Name: "A", Root: "/e/a"},
		{Name: "B", Root: "/e/b"},
		{Name: "C", Root: "/e/c"},
	}
	th := Thread{
		Session: "estate-thread",
		Path:    "/e", // the estate root itself -- no project's path resolves it
		Gardens: []GardenHit{
			{Root: "/e/a", Name: "A", Mentions: 70},
			{Root: "/e/b", Name: "B", Mentions: 25},
			{Root: "/e/c", Name: "C", Mentions: 5},
		},
	}
	byRoot := Attribute([]Thread{th}, projects, 0.2)
	if _, ok := byRoot["/e/a"]; !ok {
		t.Fatal("top garden A must always be credited")
	}
	if _, ok := byRoot["/e/b"]; !ok {
		t.Fatal("B clears the 20% floor and must be credited")
	}
	if _, ok := byRoot["/e/c"]; ok {
		t.Fatal("C is under the 20% floor and must not be credited")
	}
}

// TestAttributeKeepsPanePathFirst: a thread whose pane cwd resolves inside a
// garden is attributed there, full stop -- transcript content is never
// consulted, even when it talks entirely about a different garden.
func TestAttributeKeepsPanePathFirst(t *testing.T) {
	projects := []Project{
		{Name: "A", Root: "/e/a"},
		{Name: "B", Root: "/e/b"},
	}
	th := Thread{
		Session: "in-a",
		Path:    "/e/a",
		Gardens: []GardenHit{{Root: "/e/b", Name: "B", Mentions: 100}},
	}
	byRoot := Attribute([]Thread{th}, projects, 0.2)
	if _, ok := byRoot["/e/a"]; !ok {
		t.Fatal("pane-path resolution must win over transcript content")
	}
	if _, ok := byRoot["/e/b"]; ok {
		t.Fatal("a pane-path-resolved thread must not also fall through to garden mentions")
	}
}

// TestThreadSetSessionsPreservesCountAndTarget: Sessions() must carry the
// count and ordering ByRoot already has (Attribute's contract: most recent
// first), so enter's target (SessionSet.Target, list[0]) is the thread with
// the newest LastTurn.
func TestThreadSetSessionsPreservesCountAndTarget(t *testing.T) {
	older := Thread{Session: "older", Path: "/e/a", Activity: 10, LastTurn: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)}
	newer := Thread{Session: "newer", Path: "/e/a", Activity: 5, LastTurn: time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)}
	stray := Thread{Session: "stray", Path: "/nowhere"}
	ts := ThreadSet{
		Threads: []Thread{older, newer, stray},
		ByRoot:  map[string][]Thread{"/e/a": {newer, older}}, // most recent first, as Attribute produces
	}
	set := ts.Sessions()
	if n := set.Count("/e/a"); n != 2 {
		t.Fatalf("Count = %d, want 2", n)
	}
	target, ok := set.Target("/e/a")
	if !ok || target.Name != "newer" {
		t.Fatalf("Target = %+v ok=%v, want newer", target, ok)
	}
	if set.Total != 3 || set.Resolved != 2 {
		t.Fatalf("Total/Resolved = %d/%d, want 3/2", set.Total, set.Resolved)
	}
	if len(set.Unresolved) != 1 || set.Unresolved[0] != "stray" {
		t.Fatalf("Unresolved = %v, want [stray]", set.Unresolved)
	}
}

// TestReadThreadsEmitsEveryThread: non-claude and no-id threads are emitted
// with no transcript lookup, and a claude thread whose transcript cannot be
// found still comes through -- with Err set, never dropped.
func TestReadThreadsEmitsEveryThread(t *testing.T) {
	root := t.TempDir()
	id := "aaaa1111-bbbb-2222-cccc-333344445555"
	transcriptPath := filepath.Join(root, "-e-a", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(transcriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"assistant","timestamp":"2026-09-01T00:00:00Z","message":"hi"}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	projects := []Project{{Name: "a", Root: "/e/a"}}

	sessions := []TmuxSession{
		{Name: "iterm[a - " + id, Path: "/e/a", Command: "2.1.258"},
		{Name: "iterm[gone - deadbeef-does-not-exist", Path: "/e", Command: "2.1.258"},
		{Name: "28", Path: "/e", Command: "zsh"},
		{Name: "rio]garden-salon", Path: "/e", Command: "codex"},
	}
	var mu sync.Mutex
	got := map[string]Thread{}
	ReadThreads(context.Background(), sessions, projects, nil, root, func(th Thread) {
		mu.Lock()
		got[th.Session] = th
		mu.Unlock()
	})
	if len(got) != len(sessions) {
		t.Fatalf("ReadThreads emitted %d of %d -- a dropped thread is a silent coverage gap", len(got), len(sessions))
	}
	present := got["iterm[a - "+id]
	if present.Err != nil || present.LastTurn.IsZero() {
		t.Fatalf("thread with a real transcript should resolve cleanly: %+v", present)
	}
	missing := got["iterm[gone - deadbeef-does-not-exist"]
	if missing.Err == nil {
		t.Fatal("a claude thread whose transcript cannot be found must carry Err, not be silently dropped")
	}
	shell := got["28"]
	if shell.Runtime != RuntimeShell || shell.Err != nil {
		t.Fatalf("idle shell thread must be emitted with no lookup: %+v", shell)
	}
	codexThread := got["rio]garden-salon"]
	if codexThread.Runtime != RuntimeCodex || codexThread.Err != nil {
		t.Fatalf("codex thread must be emitted with no lookup: %+v", codexThread)
	}
}
