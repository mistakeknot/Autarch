package tui

import "testing"

func TestSharedStateDefaults(t *testing.T) {
	state := NewSharedState()
	if state.Focus != "LIST" {
		t.Fatalf("expected LIST focus")
	}
}
