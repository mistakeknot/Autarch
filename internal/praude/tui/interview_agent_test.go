package tui

import "testing"

func TestParseAgentDraftYAML(t *testing.T) {
	raw := []byte("draft: |\n  Line one\n  Line two\n")
	got := parseAgentDraft(raw)
	if got != "Line one\nLine two" {
		t.Fatalf("expected yaml draft, got %q", got)
	}
}

func TestParseAgentDraftFallback(t *testing.T) {
	raw := []byte("Plain draft")
	got := parseAgentDraft(raw)
	if got != "Plain draft" {
		t.Fatalf("expected fallback draft, got %q", got)
	}
}
