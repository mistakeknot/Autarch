package compound

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCapture(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "compound-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sol := Solution{
		Module:      "gurgeh",
		Date:        "2026-01-26",
		ProblemType: "validation_error",
		Component:   "spec_parser",
		Symptoms:    []string{"YAML parsing fails on nested arrays"},
		RootCause:   "Missing type assertion for array elements",
		Severity:    "medium",
		Tags:        []string{"yaml", "parsing", "gurgeh"},
	}

	body := `# YAML Nested Array Parsing Failure

## Problem
The spec parser fails when processing nested arrays.

## Solution
Added type assertion before accessing array elements.
`

	path, err := Capture(tmpDir, sol, body)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("file not created at %s", path)
	}

	// Verify file is in correct directory
	expectedDir := filepath.Join(tmpDir, "docs/solutions/gurgeh")
	if !strings.HasPrefix(path, expectedDir) {
		t.Errorf("file in wrong directory: got %s, want prefix %s", path, expectedDir)
	}

	// Verify content contains frontmatter
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if !strings.HasPrefix(string(content), "---\n") {
		t.Error("content missing frontmatter delimiter")
	}

	if !strings.Contains(string(content), "module: gurgeh") {
		t.Error("content missing module in frontmatter")
	}

	if !strings.Contains(string(content), "# YAML Nested Array Parsing Failure") {
		t.Error("content missing body title")
	}
}

func TestCaptureValidation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compound-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name    string
		sol     Solution
		wantErr string
	}{
		{
			name:    "missing module",
			sol:     Solution{ProblemType: "ui_bug", Component: "tui", Severity: "low", Symptoms: []string{"x"}, RootCause: "y"},
			wantErr: "module is required",
		},
		{
			name:    "invalid module",
			sol:     Solution{Module: "invalid", ProblemType: "ui_bug", Component: "tui", Severity: "low", Symptoms: []string{"x"}, RootCause: "y"},
			wantErr: "invalid module",
		},
		{
			name:    "missing severity",
			sol:     Solution{Module: "gurgeh", ProblemType: "ui_bug", Component: "tui", Symptoms: []string{"x"}, RootCause: "y"},
			wantErr: "severity is required",
		},
		{
			name:    "missing symptoms",
			sol:     Solution{Module: "gurgeh", ProblemType: "ui_bug", Component: "tui", Severity: "low", RootCause: "y"},
			wantErr: "at least one symptom is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Capture(tmpDir, tt.sol, "body")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestGenerateFilename(t *testing.T) {
	tests := []struct {
		component string
		date      string
		want      string
	}{
		{"spec_parser", "2026-01-26", "spec-parser-2026-01-26.md"},
		{"TUI Layout", "2026-01-26", "tui-layout-2026-01-26.md"},
		{"api--endpoint", "2026-01-26", "api-endpoint-2026-01-26.md"},
	}

	for _, tt := range tests {
		sol := Solution{Component: tt.component, Date: tt.date}
		got := generateFilename(sol)
		if got != tt.want {
			t.Errorf("generateFilename(%q) = %q, want %q", tt.component, got, tt.want)
		}
	}
}
