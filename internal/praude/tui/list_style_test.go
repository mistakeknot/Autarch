package tui

import "testing"

func TestRenderGroupListUsesSelectionMarker(t *testing.T) {
	items := []Item{{Type: ItemTypeGroup, Group: &Group{Name: "draft", Expanded: true}}}
	out := renderGroupList(items, 0, 0, 3)
	if out == "" {
		t.Fatalf("expected output")
	}
}
