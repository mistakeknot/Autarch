package tui

import "testing"

func TestFindTestSummary(t *testing.T) {
	log := "start\nRunning tests...\nPASS 8/8\nend\n"
	summary := FindTestSummary(log)
	if summary != "PASS 8/8" {
		t.Fatalf("unexpected summary: %q", summary)
	}
}
