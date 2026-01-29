package tui

import "testing"

func TestUnifiedDiffFromStrings(t *testing.T) {
	diff, err := UnifiedDiff("before\n", "after\n", "a.md")
	if err != nil || len(diff) == 0 {
		t.Fatalf("expected diff output")
	}
}
