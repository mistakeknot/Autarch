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
	if strings.Contains(view, "Scanned description") {
		t.Fatalf("did not expect description in vision step")
	}
	if strings.Contains(view, "Users text") {
		t.Fatalf("did not expect users in vision step")
	}
	if strings.Contains(view, "Problem text") {
		t.Fatalf("did not expect problem in vision step")
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

func TestKickoffDocPanelShowsEvidenceForVision(t *testing.T) {
	v := NewKickoffView()
	v.docPanel.SetSize(80, 30)

	_, _ = v.Update(tui.CodebaseScanResultMsg{
		PhaseArtifacts: &tui.PhaseArtifacts{
			Vision: &tui.VisionArtifact{
				Summary: "Vision text",
				Evidence: []tui.EvidenceItem{
					{Path: "README.md", Quote: "Autarch"},
				},
				Quality: tui.QualityScores{Clarity: 0.7, Completeness: 0.7, Grounding: 0.7, Consistency: 0.7},
			},
		},
	})

	view := v.docPanel.View()
	if !strings.Contains(view, "Evidence") {
		t.Fatalf("expected evidence section")
	}
	if !strings.Contains(view, "README.md") {
		t.Fatalf("expected evidence path")
	}
	if !strings.Contains(view, "Quality") {
		t.Fatalf("expected quality section")
	}
}

func TestKickoffDocPanelShowsOpenQuestions(t *testing.T) {
	v := NewKickoffView()
	v.docPanel.SetSize(80, 30)

	_, _ = v.Update(tui.CodebaseScanResultMsg{
		PhaseArtifacts: &tui.PhaseArtifacts{
			Vision: &tui.VisionArtifact{
				Summary:       "Vision text",
				OpenQuestions: []string{"What is the primary goal?"},
			},
		},
	})

	view := v.docPanel.View()
	if !strings.Contains(view, "Open Questions") {
		t.Fatalf("expected open questions section")
	}
	if !strings.Contains(view, "What is the primary goal?") {
		t.Fatalf("expected open questions content")
	}
}

func TestKickoffDocPanelShowsResolvedQuestions(t *testing.T) {
	v := NewKickoffView()
	v.docPanel.SetSize(80, 30)

	_, _ = v.Update(tui.CodebaseScanResultMsg{
		PhaseArtifacts: &tui.PhaseArtifacts{
			Vision: &tui.VisionArtifact{
				Summary: "Vision text",
				ResolvedQuestions: []tui.ResolvedQuestion{{
					Question: "What is the goal?",
					Answer:   "Ship an agent suite.",
				}},
			},
		},
	})

	view := v.docPanel.View()
	if !strings.Contains(view, "Resolved Questions") {
		t.Fatalf("expected resolved questions section")
	}
	if !strings.Contains(view, "Ship an agent suite") {
		t.Fatalf("expected resolved question answer")
	}
}

func TestKickoffRescanKeepsResolvedQuestions(t *testing.T) {
	v := NewKickoffView()
	v.scanResult = &tui.CodebaseScanResultMsg{
		PhaseArtifacts: &tui.PhaseArtifacts{
			Vision: &tui.VisionArtifact{
				ResolvedQuestions: []tui.ResolvedQuestion{{Question: "Q1?", Answer: "A1"}},
			},
		},
	}

	msg := tui.CodebaseScanResultMsg{PhaseArtifacts: &tui.PhaseArtifacts{Vision: &tui.VisionArtifact{OpenQuestions: []string{"Q1?", "Q2?"}}}}
	updated := v.applyAcceptedToScanResult(&msg)
	if len(updated.PhaseArtifacts.Vision.ResolvedQuestions) == 0 {
		t.Fatalf("expected resolved questions to carry over")
	}
	for _, q := range updated.PhaseArtifacts.Vision.OpenQuestions {
		if q == "Q1?" {
			t.Fatalf("expected resolved question removed from open list")
		}
	}
}
