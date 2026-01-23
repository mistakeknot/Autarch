package tui

import (
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

func TestSearchFiltersList(t *testing.T) {
	state := NewSharedState()
	state.Summaries = []specs.Summary{
		{ID: "PRD-001", Title: "Alpha"},
		{ID: "PRD-002", Title: "Beta"},
	}
	state.Filter = "Alpha"
	items := filterSummaries(state.Summaries, state.Filter)
	if len(items) != 1 {
		t.Fatalf("expected filtered list")
	}
}

func TestSearchModalConsumesKeys(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m = pressKey(m, "/")
		if m.searchOverlay == nil || !m.searchOverlay.Visible() {
			t.Fatalf("expected search overlay visible")
		}
	})
}
