package agent

import "testing"

func TestDetectCompletion(t *testing.T) {
	state := DetectState("Done. All tests pass.")
	if state != "done" {
		t.Fatalf("expected done, got %s", state)
	}
}

func TestDetectBlocked(t *testing.T) {
	state := DetectState("Blocked: waiting on user")
	if state != "blocked" {
		t.Fatalf("expected blocked, got %s", state)
	}
}
