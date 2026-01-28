package tui

import (
	"strings"
	"testing"
)

func TestChatPanelHidesSystemRoleLabel(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(60, 20)
	panel.AddMessage("system", "Welcome")

	view := panel.View()
	if strings.Contains(view, "System:") {
		t.Fatalf("expected System label to be hidden, got %q", view)
	}
	if !strings.Contains(view, "Welcome") {
		t.Fatalf("expected system content to be rendered")
	}
}
