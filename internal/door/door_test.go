package door

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func mkrepo(t *testing.T, root, name string, gitDir bool) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if gitDir {
		if err := os.Mkdir(filepath.Join(path, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		// A .git file (worktree/submodule pointer) still marks a repo.
		if err := os.WriteFile(filepath.Join(path, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestDiscoverProjects(t *testing.T) {
	// Resolve the tmpdir like DiscoverProjects resolves roots, or macOS's
	// /var -> /private/var symlink makes every path comparison lie.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mkrepo(t, root, "beta", true)
	mkrepo(t, root, "alpha", false) // .git file counts
	if err := os.MkdirAll(filepath.Join(root, "not-a-repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".hidden", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a-file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	ps, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("want 2 projects, got %d: %+v", len(ps), ps)
	}
	if ps[0].Name != "alpha" || ps[1].Name != "beta" {
		t.Fatalf("want sorted [alpha beta], got [%s %s]", ps[0].Name, ps[1].Name)
	}
	if want := filepath.Join(root, "beta", "docs", "why.md"); ps[1].CardPath != want {
		t.Fatalf("CardPath = %q, want %q", ps[1].CardPath, want)
	}
	for _, p := range ps {
		if p.Verdict != VerdictUnchecked {
			t.Fatalf("%s starts as %s, want unchecked before any check runs", p.Name, p.Verdict)
		}
	}
}

func TestLoadRankingAbsentIsEmpty(t *testing.T) {
	r, err := LoadRanking(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("absent ranking file must be an empty ranking, got error %v", err)
	}
	if len(r.Funded) != 0 || len(r.Pins) != 0 {
		t.Fatalf("want empty ranking, got %+v", r)
	}
}

func TestLoadRankingMalformedIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "door.yaml")
	if err := os.WriteFile(path, []byte("funded: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRanking(path); err == nil {
		t.Fatal("malformed ranking file must be an error, not an empty ranking -- it would silently reorder the estate")
	}
}

func TestSaveLoadTogglePinRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "door.yaml")
	r := Ranking{Funded: []string{"uncrancher"}}
	if got := r.TogglePin("jawnomicon"); !got {
		t.Fatal("first toggle should pin")
	}
	if err := SaveRanking(path, r); err != nil {
		t.Fatal(err)
	}
	back, err := LoadRanking(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Funded) != 1 || back.Funded[0] != "uncrancher" {
		t.Fatalf("funded lost in round trip: %+v", back)
	}
	if len(back.Pins) != 1 || back.Pins[0] != "jawnomicon" {
		t.Fatalf("pins lost in round trip: %+v", back)
	}
	if got := back.TogglePin("jawnomicon"); got {
		t.Fatal("second toggle should unpin")
	}
	if len(back.Pins) != 0 {
		t.Fatalf("unpin left residue: %+v", back.Pins)
	}
}

func TestApplyFundedOutranksPin(t *testing.T) {
	ps := []Project{{Name: "both"}, {Name: "pinned-only"}}
	r := Ranking{Funded: []string{"both"}, Pins: []string{"both", "pinned-only"}}
	r.Apply(ps)
	if !ps[0].Funded || ps[0].Pinned {
		t.Fatalf("a funded project counts once, in the funded tier: %+v", ps[0])
	}
	if !ps[1].Pinned {
		t.Fatalf("pin lost: %+v", ps[1])
	}
}

// TestRankRuling11 is the ordering contract: funded in file order, then pins
// in file order, then the tail weakest card first, verdict-tie-broken with
// invalid ahead of absent (a lying card is a regression; a missing card is a
// gap), and unchecked grouped last because an unmeasured weakness must not
// be ranked as if it had been measured.
func TestRankRuling11(t *testing.T) {
	ps := []Project{
		{Name: "strong-confirmed", Verdict: VerdictConfirmed, Strength: Strength{Score: 6}},
		{Name: "funded-2", Verdict: VerdictAbsent},
		{Name: "unchecked", Verdict: VerdictUnchecked},
		{Name: "weak-provisional", Verdict: VerdictProvisional, Strength: Strength{Score: 0}},
		{Name: "absent", Verdict: VerdictAbsent},
		{Name: "funded-1", Verdict: VerdictConfirmed, Strength: Strength{Score: 6}},
		{Name: "invalid", Verdict: VerdictInvalid},
		{Name: "pinned", Verdict: VerdictAbsent},
		{Name: "mid-provisional", Verdict: VerdictProvisional, Strength: Strength{Score: 3}},
	}
	r := Ranking{Funded: []string{"funded-1", "funded-2"}, Pins: []string{"pinned"}}
	r.Apply(ps)
	Rank(ps)

	var got []string
	for _, p := range ps {
		got = append(got, p.Name)
	}
	want := []string{
		"funded-1", "funded-2", // file order, regardless of strength
		"pinned",
		"invalid",          // score 0, weakest verdict
		"absent",           // score 0
		"weak-provisional", // score 0
		"mid-provisional",  // score 3
		"strong-confirmed", // score 6
		"unchecked",        // grouped last, never ranked
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("ruling 11 order broken:\n got %v\nwant %v", got, want)
	}
}

// stubChecker writes a fake card-check.py whose verdict depends on the root's
// basename. The stub speaks the real --json contract; the checker's actual
// rules have their own suite in dotfiles, and this suite tests only that the
// door transcribes faithfully.
func stubChecker(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "card-check.py")
	script := `#!/bin/sh
case "$(basename "$1")" in
  confirmed) echo '{"verdict":"confirmed","code":0,"card":"x","reason":"","strength":{"score":6,"of":6,"confirmed":6,"drafted":0,"declined":0}}'; exit 0;;
  provisional) echo '{"verdict":"provisional","code":1,"card":"x","reason":"2 drafted","strength":{"score":2,"of":6,"confirmed":2,"drafted":2,"declined":0}}'; exit 1;;
  invalid) echo '{"verdict":"invalid","code":2,"card":"x","reason":"unsupported card_version","strength":{"score":0,"of":6,"confirmed":0,"drafted":0,"declined":0}}'; exit 2;;
  absent) echo '{"verdict":"absent","code":3,"card":"x","reason":"no card","strength":{"score":0,"of":6,"confirmed":0,"drafted":0,"declined":0}}'; exit 3;;
  garbage) echo 'not json at all'; exit 0;;
  martian) echo '{"verdict":"martian","code":9,"card":"x","reason":"","strength":{"score":0,"of":6,"confirmed":0,"drafted":0,"declined":0}}'; exit 0;;
esac`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckOneTranscribesEveryVerdict(t *testing.T) {
	checker := stubChecker(t)
	for _, want := range []Verdict{VerdictConfirmed, VerdictProvisional, VerdictInvalid, VerdictAbsent} {
		p := CheckOne(context.Background(), checker, Project{Name: string(want), Root: "/tmp/" + string(want)})
		if p.Verdict != want {
			t.Fatalf("verdict %s came back as %s (err=%v)", want, p.Verdict, p.Err)
		}
		if p.Err != nil {
			t.Fatalf("verdict %s should not carry an error: %v", want, p.Err)
		}
	}
	p := CheckOne(context.Background(), checker, Project{Name: "confirmed", Root: "/tmp/confirmed"})
	if p.Strength.Score != 6 || p.Strength.Of != 6 {
		t.Fatalf("strength not transcribed: %+v", p.Strength)
	}
	p = CheckOne(context.Background(), checker, Project{Name: "invalid", Root: "/tmp/invalid"})
	if p.Reason != "unsupported card_version" {
		t.Fatalf("reason not transcribed: %q", p.Reason)
	}
}

func TestCheckOneFailureIsUncheckedNeverAbsent(t *testing.T) {
	checker := stubChecker(t)
	for _, name := range []string{"garbage", "martian"} {
		p := CheckOne(context.Background(), checker, Project{Name: name, Root: "/tmp/" + name})
		if p.Verdict != VerdictUnchecked {
			t.Fatalf("%s: a checker failure must be unchecked, got %s", name, p.Verdict)
		}
		if p.Err == nil {
			t.Fatalf("%s: unchecked must carry its reason", name)
		}
	}
	// A checker binary that does not exist at all is the same story.
	p := CheckOne(context.Background(), filepath.Join(t.TempDir(), "gone"), Project{Name: "x", Root: "/tmp/x"})
	if p.Verdict != VerdictUnchecked || p.Err == nil {
		t.Fatalf("missing checker must be unchecked with a reason, got %s err=%v", p.Verdict, p.Err)
	}
}

func TestCheckAllReportsEveryProject(t *testing.T) {
	checker := stubChecker(t)
	var ps []Project
	for i := 0; i < 30; i++ {
		name := []string{"confirmed", "provisional", "invalid", "absent", "garbage"}[i%5]
		ps = append(ps, Project{Name: name, Root: "/tmp/" + name})
	}
	var mu sync.Mutex
	got := 0
	CheckAll(context.Background(), checker, ps, func(_ int, p Project) {
		mu.Lock()
		got++
		mu.Unlock()
	})
	if got != len(ps) {
		t.Fatalf("CheckAll reported %d of %d -- a dropped project is a silent coverage gap", got, len(ps))
	}
}

// TestRealCheckerContract runs against the deployed card-check.py when it is
// on PATH, so a contract drift (renamed JSON key, changed verdict string)
// fails here instead of on mk's screen. Skipped where dotfiles are not
// installed; the stub tests above still cover the door's own logic.
func TestRealCheckerContract(t *testing.T) {
	checker, err := exec.LookPath(CheckerName)
	if err != nil {
		t.Skipf("%s not on PATH; contract test needs the dotfiles install", CheckerName)
	}
	root := t.TempDir() // no docs/why.md -> absent
	p := CheckOne(context.Background(), checker, Project{Name: "empty", Root: root})
	if p.Verdict != VerdictAbsent {
		t.Fatalf("real checker on empty repo: want absent, got %s (err=%v)", p.Verdict, p.Err)
	}
	if p.Strength.Of == 0 {
		t.Fatalf("real checker emitted no strength block: %+v", p.Strength)
	}
}

func TestViewFourStatesVisiblyDistinct(t *testing.T) {
	ps := []Project{
		{Name: "a", Root: "/a", Verdict: VerdictConfirmed, Strength: Strength{Score: 6, Of: 6}},
		{Name: "b", Root: "/b", Verdict: VerdictProvisional, Strength: Strength{Score: 2, Of: 6}},
		{Name: "c", Root: "/c", Verdict: VerdictInvalid, Reason: "bad version"},
		{Name: "d", Root: "/d", Verdict: VerdictAbsent},
		{Name: "e", Root: "/e", Verdict: VerdictUnchecked},
	}
	m := NewModel(ps, Ranking{}, "", nil, "checker", nil)
	m.width, m.height = 100, 20
	view := m.View()
	for _, label := range []string{"CONFIRMED", "PROVISIONAL", "INVALID", "ABSENT", "UNCHECKED"} {
		if !strings.Contains(view, label) {
			t.Fatalf("view missing state label %s:\n%s", label, view)
		}
	}
	if !strings.Contains(view, "6/6") {
		t.Fatalf("confirmed strength missing from view:\n%s", view)
	}
	if !strings.Contains(view, "bad version") {
		t.Fatalf("invalid reason missing from view:\n%s", view)
	}
	// An unchecked row shows no number: printing 0/6 for a project nobody
	// measured is the empty-result-read-as-zero failure.
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "UNCHECKED") && strings.Contains(line, "0/6") {
			t.Fatalf("unchecked row renders a score it never measured: %q", line)
		}
	}
}

func TestCoverageLineDisclosesUnchecked(t *testing.T) {
	line := coverageLine(Coverage{Total: 82, Absent: 78, Confirmed: 1, Provisional: 2, Unchecked: 1})
	if !strings.Contains(line, "UNCHECKED") {
		t.Fatalf("coverage must name unchecked rows: %q", line)
	}
	quiet := coverageLine(Coverage{Total: 82, Absent: 81, Confirmed: 1})
	if strings.Contains(quiet, "UNCHECKED") {
		t.Fatalf("fully-checked estate should not shout UNCHECKED: %q", quiet)
	}
}

func TestModelReRanksAsResultsArriveAndSelectionFollows(t *testing.T) {
	ps := []Project{
		{Name: "aaa", Root: "/aaa", Verdict: VerdictUnchecked},
		{Name: "bbb", Root: "/bbb", Verdict: VerdictUnchecked},
	}
	m := NewModel(ps, Ranking{}, "", nil, "checker", nil)
	m.width, m.height = 80, 20
	m.selRoot = "/bbb"
	m.moved = true // the user has navigated; selection follows identity

	// aaa comes back confirmed at strength 6; bbb absent. Weakest-first puts
	// bbb on top -- selection must follow the project, not the row number.
	next, _ := m.Update(resultMsg{p: Project{Name: "aaa", Root: "/aaa", Verdict: VerdictConfirmed, Strength: Strength{Score: 6, Of: 6}}})
	m = next.(Model)
	next, _ = m.Update(resultMsg{p: Project{Name: "bbb", Root: "/bbb", Verdict: VerdictAbsent}})
	m = next.(Model)

	if m.projects[0].Name != "bbb" || m.projects[1].Name != "aaa" {
		t.Fatalf("weakest-first re-rank did not happen: %s, %s", m.projects[0].Name, m.projects[1].Name)
	}
	if m.selIndex() != 0 {
		t.Fatalf("selection lost across re-rank: index %d, root %s", m.selIndex(), m.selRoot)
	}
}

// Before the user touches a key, the door shows its point: the weakest row.
// If selection followed a project identity through the initial fill, the view
// would scroll to wherever the alphabetically-first project happened to land
// (observed live: a strong card dragged the viewport to the bottom).
func TestUntouchedSelectionStaysOnWeakestRow(t *testing.T) {
	ps := []Project{
		{Name: "aaa", Root: "/aaa", Verdict: VerdictUnchecked},
		{Name: "bbb", Root: "/bbb", Verdict: VerdictUnchecked},
	}
	m := NewModel(ps, Ranking{}, "", nil, "checker", nil)
	m.width, m.height = 80, 20
	// NewModel selects the first row: aaa. aaa then ranks to the bottom.
	next, _ := m.Update(resultMsg{p: Project{Name: "aaa", Root: "/aaa", Verdict: VerdictConfirmed, Strength: Strength{Score: 6, Of: 6}}})
	m = next.(Model)
	next, _ = m.Update(resultMsg{p: Project{Name: "bbb", Root: "/bbb", Verdict: VerdictAbsent}})
	m = next.(Model)
	if m.selIndex() != 0 || m.projects[0].Name != "bbb" {
		t.Fatalf("untouched door must keep the weakest row selected: sel=%d top=%s", m.selIndex(), m.projects[0].Name)
	}
}
