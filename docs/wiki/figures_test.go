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
}
