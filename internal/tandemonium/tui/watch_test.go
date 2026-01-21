package tui

import "testing"

func TestWatchFiltersPaths(t *testing.T) {
	if !shouldReloadPath(".tandemonium/specs/T1.yaml") {
		t.Fatalf("expected spec path to reload")
	}
	if shouldReloadPath("README.md") {
		t.Fatalf("expected README to be ignored")
	}
}
