package tui

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	out, err := renderMarkdown("# Title\n- item", 40)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Title") {
		t.Fatalf("expected rendered output")
	}
}
