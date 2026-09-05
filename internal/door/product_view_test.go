package door

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func productModelFixture(t *testing.T) Model {
	t.Helper()
	root := t.TempDir()
	productFile(t, root, "docs/why.md", productCardFixture)
	productFile(t, root, "docs/roadmap.md", "# Roadmap\n\nGenerated on 2026-02-25\n"+strings.Repeat("A long plan with wide text 漢字. ", 200))
	m := NewProductModel(root)
	m.product = ReadProductBrief(context.Background(), root, func(context.Context, string, string) ProductBacklog {
		return ProductBacklog{Source: ProductSource{State: "read", Path: root}, Label: "reader", Items: []ProductWork{{ID: "reader-1", Title: "Keep context", Status: "in_progress", SpecID: "reader-01"}}}
	})
	m.productLoading = false
	m.width, m.height = 90, 30
	return m
}

func TestProductViewShowsIntentAndExplicitMissingMeasure(t *testing.T) {
	m := productModelFixture(t)
	all := strings.Join(m.productLines(), "\n")
	for _, want := range []string{"Editors reviewing long documents", "declined", "A measured reading trial", "reader-1", "reader-01", "source declarations"} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing %q: %s", want, all)
		}
	}
	m, _ = press(m, "3")
	if !strings.Contains(m.View(), "label reader") || !strings.Contains(m.View(), "spec: reader-01") {
		t.Fatal(m.View())
	}
}

func TestProductViewNavigationRefreshAndGeometry(t *testing.T) {
	m := productModelFixture(t)
	m, _ = press(m, "2")
	if !strings.Contains(m.View(), "2026-02-25") {
		t.Fatal("roadmap's authored date lost")
	}
	first := m.View()
	m, _ = press(m, "G")
	if m.View() == first || m.productOffset == 0 {
		t.Fatal("roadmap did not scroll")
	}
	for _, size := range [][2]int{{30, 12}, {14, 6}, {100, 38}} {
		x, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = x.(Model)
		lines := strings.Split(m.View(), "\n")
		if len(lines) > size[1] {
			t.Fatalf("height overflow: %d > %d", len(lines), size[1])
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("width overflow: %q", line)
			}
		}
	}
	m, cmd := press(m, "r")
	if cmd == nil || !m.productLoading || !strings.Contains(strings.Join(m.productLines(), "\n"), "Reading") {
		t.Fatal("refresh is not visibly pending")
	}
	_, cmd = press(m, "r")
	if cmd != nil {
		t.Fatal("overlapping refresh")
	}
}

func TestProductRowEntryReturnsToRowsWithoutHandoff(t *testing.T) {
	m := catchupFixture()
	m.screen = screenRows
	m, cmd := press(m, "i")
	if m.screen != screenProduct || cmd == nil {
		t.Fatal("project row did not open product reader")
	}
	m, cmd = press(m, "esc")
	if m.screen != screenRows || cmd != nil {
		t.Fatal("back did not preserve estate")
	}
}
