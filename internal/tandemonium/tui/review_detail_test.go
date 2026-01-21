package tui

import (
	"strings"
	"testing"
)

func TestReviewDetailRenderIncludesSummary(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewDetail = ReviewDetail{
		TaskID:    "T1",
		Title:     "Example",
		Summary:   "Did the thing.",
		TestsSummary: "PASS 8/8",
		Files:     []ReviewFile{{Path: "src/app.go", Added: 10, Deleted: 2}},
		AcceptanceCriteria: []string{"First", "Second"},
	}
	out := m.View()
	if !strings.Contains(out, "SUMMARY") || !strings.Contains(out, "Did the thing.") {
		t.Fatalf("expected summary content, got %q", out)
	}
	if !strings.Contains(out, "FILES CHANGED") || !strings.Contains(out, "src/app.go") {
		t.Fatalf("expected files content")
	}
}

func TestReviewDetailIncludesStoryDrift(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewDetail = ReviewDetail{StoryDrift: "changed"}
	out := m.View()
	if !strings.Contains(out, "STORY DRIFT") {
		t.Fatalf("expected drift warning")
	}
}

func TestReviewViewShowsAlignment(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewDetail = ReviewDetail{Alignment: "mvp"}
	out := m.View()
	if !strings.Contains(out, "ALIGNMENT") {
		t.Fatalf("expected alignment section")
	}
}

func TestReviewDetailShowsAlignmentLabels(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewDetail = ReviewDetail{Alignment: "mvp"}
	out := m.View()
	if !strings.Contains(out, "Alignment: MVP") {
		t.Fatalf("expected MVP alignment label")
	}
	m.ReviewDetail = ReviewDetail{Alignment: "out"}
	out = m.View()
	if !strings.Contains(out, "Alignment: out of scope") {
		t.Fatalf("expected out-of-scope alignment label")
	}
}
