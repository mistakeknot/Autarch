package tui

import (
	"strings"
	"testing"
)

func TestFilterHintRenders(t *testing.T) {
	m := NewModel()
	m.FilterMode = "review"
	out := m.View()
	if !strings.Contains(out, "filter: review") {
		t.Fatalf("expected filter hint")
	}
}
