// Package wiki checks the figures in docs/wiki/src against the code, so the wiki cannot drift.
package wiki

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var figureRe = regexp.MustCompile(`<!-- figure:([a-z-]+)=([^ ]+?) -->`)

// proseRe finds a marker with the text that follows it, so the written figure can be checked too.
var proseRe = regexp.MustCompile(`<!-- figure:([a-z-]+)=([^ ]+?) -->\s*(\S+)`)

// figures reads every marker in the book as name -> values.
func figures(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	pages, err := filepath.Glob("src/*.md")
	if err != nil || len(pages) == 0 {
		t.Fatalf("no wiki pages: %v", err)
	}
	for _, p := range pages {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range figureRe.FindAllStringSubmatch(string(b), -1) {
			out[m[1]] = append(out[m[1]], m[2])
		}
	}
	return out
}

// TestProseMatchesMarkers: the number written after a marker must be the marker's value, so editing the prose alone fails.
func TestProseMatchesMarkers(t *testing.T) {
	pages, _ := filepath.Glob("src/*.md")
	for _, p := range pages {
		if strings.HasSuffix(p, "introduction.md") {
			continue // its marker is an example inside a code span
		}
		b, _ := os.ReadFile(p)
		for _, m := range proseRe.FindAllStringSubmatch(string(b), -1) {
			if m[1] == "cmd-names" {
				continue // a list; checked below
			}
			if !strings.HasPrefix(m[3], m[2]) {
				t.Errorf("%s: figure %s=%s is followed by %q", p, m[1], m[2], m[3])
			}
		}
	}
	b, _ := os.ReadFile("src/tools.md")
	for _, n := range strings.Split(figures(t)["cmd-names"][0], ",") {
		if !strings.Contains(string(b), "`"+n+"`") {
			t.Errorf("tools.md does not list command %s", n)
		}
	}
}

func dirs(t *testing.T, rel string) []string {
	t.Helper()
	es, err := os.ReadDir(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func expect(t *testing.T, name, want string) {
	t.Helper()
	got := figures(t)[name]
	if len(got) == 0 {
		t.Fatalf("figure %s is not in the wiki", name)
	}
	for _, g := range got {
		if g != want {
			t.Errorf("wiki says %s=%s, code says %s", name, g, want)
		}
	}
}

func TestCmdFigures(t *testing.T) {
	names := dirs(t, "cmd")
	expect(t, "cmd-count", strconv.Itoa(len(names)))
	expect(t, "cmd-names", strings.Join(names, ","))
}

func TestPackageFigures(t *testing.T) {
	expect(t, "internal-count", strconv.Itoa(len(dirs(t, "internal"))))
	expect(t, "pkg-count", strconv.Itoa(len(dirs(t, "pkg"))))
}

func TestHomeAskFigures(t *testing.T) {
	model := read(t, "internal/homeask/model.go")
	m := regexp.MustCompile(`case "instruction", "needs-context", "ruling-only":`).FindString(model)
	if m == "" {
		t.Fatal("option kinds switch not found in model.go")
	}
	expect(t, "option-kinds", strconv.Itoa(strings.Count(m, `"`)/2))
	move := read(t, "internal/homeask/move.go")
	mk := regexp.MustCompile(`kind must be ([a-z, ]+) or ([a-z]+)"`).FindStringSubmatch(move)
	if mk == nil {
		t.Fatal("move kinds message not found in move.go")
	}
	expect(t, "move-kinds", strconv.Itoa(len(strings.Split(mk[1], ","))+1))
}

func TestGhSyncFigures(t *testing.T) {
	ts := read(t, "integrations/bb-plugin-autarch/ghsync.ts")
	block := regexp.MustCompile(`(?s)export const REPO_PROJECTS[^{]*\{(.*?)\n\};`).FindStringSubmatch(ts)
	if block == nil {
		t.Fatal("REPO_PROJECTS not found")
	}
	n := len(regexp.MustCompile(`(?m)^\s*"[^"]+": "[A-Z]+",`).FindAllString(block[1], -1))
	expect(t, "repo-mappings", strconv.Itoa(n))
	// Every repository-to-project pair in the code must be a row of the page's table, and the other way round.
	code := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*"([^"]+)": "([A-Z]+)",`).FindAllStringSubmatch(block[1], -1) {
		code["| `"+m[1]+"` | `"+m[2]+"` |"] = true
	}
	page := read(t, "docs/wiki/src/gh-sync.md")
	rows := regexp.MustCompile("(?m)^\\| `[^`]+` \\| `[A-Z]+` \\|$").FindAllString(page, -1)
	if len(rows) != len(code) {
		t.Errorf("table has %d rows, code has %d mappings", len(rows), len(code))
	}
	for _, r := range rows {
		if !code[r] {
			t.Errorf("table row %s is not in REPO_PROJECTS", r)
		}
	}
}
