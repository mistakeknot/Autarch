package tui

import (
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

func TestSelectedIndexFromID(t *testing.T) {
	items := []Item{
		{Type: ItemTypeGroup, Group: &Group{Name: "draft"}},
		{Type: ItemTypePRD, Summary: &specs.Summary{ID: "PRD-1"}},
		{Type: ItemTypePRD, Summary: &specs.Summary{ID: "PRD-2"}},
	}
	idx := selectedIndexFromID(items, "PRD-2")
	if idx != 2 {
		t.Fatalf("expected index 2, got %d", idx)
	}
	idx = selectedIndexFromID(items, "missing")
	if idx != 0 {
		t.Fatalf("expected default index 0, got %d", idx)
	}
}
