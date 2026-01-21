package tui

import "testing"

func TestMarkdownCacheHits(t *testing.T) {
	cache := NewMarkdownCache()
	cache.Set("PRD-001", "hash", "rendered")
	if got, ok := cache.Get("PRD-001", "hash"); !ok || got != "rendered" {
		t.Fatalf("expected cached render")
	}
}
