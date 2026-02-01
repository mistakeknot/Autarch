package views

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/tui"
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

func TestKickoffScanResultUpdatesDocPanel(t *testing.T) {
	v := NewKickoffView()
	v.docPanel.SetSize(80, 30)
	v.chatPanel.SetValue("before")

	_, _ = v.Update(tui.CodebaseScanResultMsg{
		Description:  "Scanned description",
		Vision:       "Vision text",
		Users:        "Users text",
		Problem:      "Problem text",
		Platform:     "Web",
		Language:     "Go",
		Requirements: []string{"Req one", "Req two"},
	})

	if v.chatPanel.Value() != "before" {
		t.Fatalf("expected chat composer unchanged")
	}

	view := v.docPanel.View()
	if !strings.Contains(view, "Vision text") {
		t.Fatalf("expected doc panel to include scan vision")
	}
	if !strings.Contains(view, "Users text") {
		t.Fatalf("expected doc panel to include users")
	}
	if !strings.Contains(view, "Problem text") {
		t.Fatalf("expected doc panel to include problem")
	}
}

func TestKickoffSidebarUsesInterviewSteps(t *testing.T) {
	v := NewKickoffView()
	items := v.SidebarItems()

	if len(items) != 8 {
		t.Fatalf("expected 8 sidebar items, got %d", len(items))
	}
	if items[0].Label != "Vision" ||
		items[1].Label != "Problem" ||
		items[2].Label != "Users" ||
		items[3].Label != "Features + Goals" ||
		items[4].Label != "Requirements" ||
		items[5].Label != "Scope + Assumptions" ||
		items[6].Label != "Critical User Journeys" ||
		items[7].Label != "Acceptance Criteria" {
		t.Fatalf("unexpected sidebar labels: %#v", items)
	}
}

func TestKickoffScanResultHidesTipsAndHeaders(t *testing.T) {
	v := NewKickoffView()
	v.docPanel.SetSize(80, 30)

	_, _ = v.Update(tui.CodebaseScanResultMsg{
		Description: "Scanned description",
	})

	view := v.docPanel.View()
	if strings.Contains(view, "Scan Results") {
		t.Fatalf("did not expect scan results header")
	}
	if strings.Contains(view, "Tips") {
		t.Fatalf("did not expect tips section during scan signoff")
	}
	if strings.Contains(view, "Shortcuts") {
		t.Fatalf("did not expect shortcuts section during scan signoff")
	}
}

func TestKickoffRevertRestoresSnapshot(t *testing.T) {
	v := NewKickoffView()
	v.scanResult = &tui.CodebaseScanResultMsg{Vision: "Old"}
	snapLabel, snap := v.DocumentSnapshot()
	if snapLabel == "" || snap == "" {
		t.Fatalf("expected snapshot")
	}

	v.scanResult.Vision = "New"
	_, _ = v.Update(tui.RevertLastRunMsg{Snapshot: snap})
	if v.scanResult.Vision != "Old" {
		t.Fatalf("expected revert")
	}
}
