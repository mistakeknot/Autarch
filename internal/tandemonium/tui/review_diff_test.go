package tui

import (
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
)

func TestReviewDiffLoadsFiles(t *testing.T) {
	m := NewModel()
	m.ReviewDiffLoader = func(taskID string) (ReviewDiffState, error) {
		return ReviewDiffState{Files: []string{"a.txt"}}, nil
	}
	m.ViewMode = ViewReview
	m.ReviewQueue = []string{"T1"}
	m.SelectedReview = 0
	m.handleReviewDiff()
	if len(m.ReviewDiff.Files) != 1 {
		t.Fatalf("expected diff files")
	}
}

func TestReviewDiffViewRendersHeader(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewShowDiffs = true
	m.ReviewDiff = ReviewDiffState{Files: []string{"a.txt"}, Current: 0, Lines: []string{"@@ -1 +1 @@"}}
	out := m.View()
	if !strings.Contains(out, "REVIEW DIFF") {
		t.Fatalf("expected diff header")
	}
}

func TestReviewDiffNextPrev(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewShowDiffs = true
	m.ReviewDiff = ReviewDiffState{Files: []string{"a.txt", "b.txt"}, Current: 0}
	m.handleReviewDiffKey("j")
	if m.ReviewDiff.Current != 1 {
		t.Fatalf("expected next file")
	}
	m.handleReviewDiffKey("k")
	if m.ReviewDiff.Current != 0 {
		t.Fatalf("expected prev file")
	}
}

func TestBuildReviewDiffState(t *testing.T) {
	state, err := buildReviewDiffState("main", "feature", []string{"a.txt"}, func(path string) ([]string, error) {
		return []string{"@@ -1 +1 @@"}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Files) != 1 || state.Files[0] != "a.txt" {
		t.Fatalf("unexpected files: %v", state.Files)
	}
	if len(state.Lines) == 0 || state.Lines[0] != "@@ -1 +1 @@" {
		t.Fatalf("unexpected lines: %v", state.Lines)
	}
	if state.BaseBranch != "main" || state.TaskBranch != "feature" {
		t.Fatalf("unexpected branches")
	}
}

func TestBuildReviewDiffStateLoadsCurrentOnly(t *testing.T) {
	loaded := 0
	_, err := buildReviewDiffState("main", "feature", []string{"a.txt", "b.txt"}, func(path string) ([]string, error) {
		loaded++
		return []string{"@@ -1 +1 @@"}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != 1 {
		t.Fatalf("expected 1 diff load, got %d", loaded)
	}
}

type fakeRunnerBranch struct{ out string }

func (f *fakeRunnerBranch) Run(name string, args ...string) (string, error) { return f.out, nil }

func TestCurrentBranch(t *testing.T) {
	branch, err := currentBranch(&fakeRunnerBranch{out: "main\n"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected main, got %q", branch)
	}
}

func TestReviewBaseBranchUsesConfig(t *testing.T) {
	cfg := config.Config{
		Review: config.ReviewConfig{TargetBranch: "develop"},
	}
	branch, err := reviewBaseBranch(cfg, &fakeRunnerBranch{out: "main\n"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "develop" {
		t.Fatalf("expected develop, got %q", branch)
	}
}
