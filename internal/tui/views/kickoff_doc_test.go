package views

import (
	"strings"
	"testing"
)

func TestKickoffDocPanelIncludesAutarchCopy(t *testing.T) {
	v := NewKickoffView()
	v.docPanel.SetSize(80, 20)

	view := v.docPanel.View()
	expected := "Autarch is a platform for a suite of agentic tools"

	if !strings.Contains(view, expected) {
		t.Fatalf("expected doc panel to include kickoff copy, got %q", view)
	}
}
