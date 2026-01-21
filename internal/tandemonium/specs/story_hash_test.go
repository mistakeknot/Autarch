package specs

import "testing"

func TestStoryHash(t *testing.T) {
	h := StoryHash("As a user, I want X.")
	if len(h) != 8 {
		t.Fatalf("expected 8-char hash, got %q", h)
	}
}
