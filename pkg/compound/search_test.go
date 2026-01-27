package compound

import (
	"os"
	"testing"
)

func TestSearch(t *testing.T) {
	// Create temp directory with test solutions
	tmpDir, err := os.MkdirTemp("", "compound-search-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test solutions
	solutions := []struct {
		sol  Solution
		body string
	}{
		{
			sol: Solution{
				Module:      "gurgeh",
				Date:        "2026-01-20",
				ProblemType: "validation_error",
				Component:   "spec_parser",
				Symptoms:    []string{"YAML parsing fails"},
				RootCause:   "Type assertion error",
				Severity:    "medium",
				Tags:        []string{"yaml", "parsing"},
			},
			body: "# YAML Parsing Issue\n\nSolution details here.",
		},
		{
			sol: Solution{
				Module:      "coldwine",
				Date:        "2026-01-25",
				ProblemType: "ui_bug",
				Component:   "task_list",
				Symptoms:    []string{"Tasks not displaying"},
				RootCause:   "State not updated",
				Severity:    "high",
				Tags:        []string{"tui", "state"},
			},
			body: "# Task List Display Bug\n\nTUI state management issue.",
		},
		{
			sol: Solution{
				Module:      "gurgeh",
				Date:        "2026-01-26",
				ProblemType: "ui_bug",
				Component:   "tui_layout",
				Symptoms:    []string{"Layout overflow", "Stray bars"},
				RootCause:   "Dimension mismatch",
				Severity:    "critical",
				Tags:        []string{"tui", "layout", "lipgloss"},
			},
			body: "# TUI Layout Overflow\n\nDimension calculation was wrong.",
		},
	}

	for _, s := range solutions {
		_, err := Capture(tmpDir, s.sol, s.body)
		if err != nil {
			t.Fatalf("Capture: %v", err)
		}
	}

	t.Run("search all", func(t *testing.T) {
		results, err := Search(tmpDir, SearchOptions{})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("got %d results, want 3", len(results))
		}
	})

	t.Run("filter by module", func(t *testing.T) {
		results, err := Search(tmpDir, SearchOptions{Module: "gurgeh"})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d results, want 2", len(results))
		}
	})

	t.Run("filter by tags", func(t *testing.T) {
		results, err := Search(tmpDir, SearchOptions{Tags: []string{"tui"}})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d results, want 2 (coldwine and gurgeh tui bugs)", len(results))
		}
	})

	t.Run("filter by severity", func(t *testing.T) {
		results, err := Search(tmpDir, SearchOptions{Severity: "high"})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d results, want 2 (high and critical)", len(results))
		}
	})

	t.Run("query search", func(t *testing.T) {
		results, err := Search(tmpDir, SearchOptions{Query: "dimension"})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("got %d results, want 1", len(results))
		}
		if len(results) > 0 && results[0].Solution.Component != "tui_layout" {
			t.Errorf("wrong result: got %s, want tui_layout", results[0].Solution.Component)
		}
	})

	t.Run("combined filters", func(t *testing.T) {
		results, err := Search(tmpDir, SearchOptions{
			Module: "gurgeh",
			Tags:   []string{"tui"},
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("got %d results, want 1 (gurgeh tui bug only)", len(results))
		}
	})
}

func TestSearchNonexistentDir(t *testing.T) {
	results, err := Search("/nonexistent/path", SearchOptions{})
	if err != nil {
		t.Fatalf("Search should not error for nonexistent dir: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent dir, got %d", len(results))
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		body string
		want string
	}{
		{"# My Title\n\nContent here", "My Title"},
		{"\n\n# Title After Blank\n\nMore", "Title After Blank"},
		{"No title here", ""},
		{"## Not H1\n\nContent", ""},
	}

	for _, tt := range tests {
		got := extractTitle([]byte(tt.body))
		if got != tt.want {
			t.Errorf("extractTitle(%q) = %q, want %q", tt.body, got, tt.want)
		}
	}
}

func TestSeverityLevel(t *testing.T) {
	tests := []struct {
		severity string
		want     int
	}{
		{"low", 1},
		{"medium", 2},
		{"high", 3},
		{"critical", 4},
		{"unknown", 0},
	}

	for _, tt := range tests {
		got := severityLevel(tt.severity)
		if got != tt.want {
			t.Errorf("severityLevel(%q) = %d, want %d", tt.severity, got, tt.want)
		}
	}
}
