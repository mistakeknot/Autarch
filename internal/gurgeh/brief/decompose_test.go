package brief

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestBriefJSONMarshal(t *testing.T) {
	b := Brief{
		Title:    "Add user authentication",
		Outcome:  "Users can log in with email and password",
		Criteria: []string{"Login form renders", "Invalid credentials show error", "Valid credentials redirect to dashboard"},
	}

	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded Brief
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Title != b.Title {
		t.Errorf("Title mismatch: got %q, want %q", decoded.Title, b.Title)
	}
	if decoded.Outcome != b.Outcome {
		t.Errorf("Outcome mismatch: got %q, want %q", decoded.Outcome, b.Outcome)
	}
	if len(decoded.Criteria) != len(b.Criteria) {
		t.Errorf("Criteria length mismatch: got %d, want %d", len(decoded.Criteria), len(b.Criteria))
	}
}

func TestBuildPrompt(t *testing.T) {
	spec := &specs.Spec{
		ID:      "PRD-001",
		Title:   "User Authentication",
		Summary: "Add login and signup functionality",
		Requirements: []string{
			"Users can sign up with email",
			"Users can log in with email",
		},
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				ID:    "CUJ-001",
				Title: "User Login",
				Steps: []string{"Visit /login", "Enter credentials", "Click submit"},
			},
		},
		Acceptance: []specs.AcceptanceCriterion{
			{ID: "AC-001", Description: "Login form is accessible"},
			{ID: "AC-002", Description: "Invalid login shows error"},
		},
	}

	prompt := buildPrompt(spec)

	// Verify prompt contains key elements
	checks := []string{
		"PRD-001",
		"User Authentication",
		"Add login and signup functionality",
		"Users can sign up with email",
		"CUJ-001: User Login",
		"AC-001: Login form is accessible",
		`"briefs"`,
	}

	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("prompt missing %q", check)
		}
	}
}

func TestFormatBrief(t *testing.T) {
	b := Brief{
		Title:    "Implement Login Form",
		Outcome:  "Users see a login form at /login",
		Criteria: []string{"Form has email field", "Form has password field"},
	}

	content := formatBrief(b)

	// Verify markdown structure
	checks := []string{
		"# Implement Login Form",
		"## Outcome",
		"Users see a login form at /login",
		"## Acceptance Criteria",
		"- [ ] Form has email field",
		"- [ ] Form has password field",
	}

	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("formatted brief missing %q", check)
		}
	}
}

func TestSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Add User Authentication", "add-user-authentication"},
		{"Fix Bug #123", "fix-bug-123"},
		{"UPPER CASE", "upper-case"},
		{"multiple   spaces", "multiple-spaces"},
		{"", ""},
		{"a-very-long-title-that-exceeds-the-forty-character-limit-by-a-lot", "a-very-long-title-that-exceeds-the-forty"},
	}

	for _, tt := range tests {
		got := slug(tt.input)
		if got != tt.want {
			t.Errorf("slug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain JSON",
			input: `{"briefs": []}`,
			want:  `{"briefs": []}`,
		},
		{
			name: "JSON in markdown fence",
			input: "```json\n{\"briefs\": []}\n```",
			want:  `{"briefs": []}`,
		},
		{
			name: "JSON in plain fence",
			input: "```\n{\"briefs\": []}\n```",
			want:  `{"briefs": []}`,
		},
		{
			name:  "JSON with surrounding text",
			input: "Here is the result:\n{\"briefs\": []}\nThat's all.",
			want:  `{"briefs": []}`,
		},
		{
			name:  "no JSON",
			input: "No JSON here",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSaveBriefs(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gurgeh-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	briefs := []Brief{
		{Title: "First Task", Outcome: "First outcome", Criteria: []string{"Check 1"}},
		{Title: "Second Task", Outcome: "Second outcome", Criteria: []string{"Check 2", "Check 3"}},
	}

	if err := SaveBriefs("PRD-001", briefs); err != nil {
		t.Fatalf("SaveBriefs failed: %v", err)
	}

	// Verify files exist
	briefsDir := filepath.Join(".gurgeh", "briefs", "PRD-001")
	entries, err := os.ReadDir(briefsDir)
	if err != nil {
		t.Fatalf("failed to read briefs dir: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 files, got %d", len(entries))
	}

	// Verify first file content
	content, err := os.ReadFile(filepath.Join(briefsDir, "BRIEF-001-first-task.md"))
	if err != nil {
		t.Fatalf("failed to read first brief: %v", err)
	}

	if !strings.Contains(string(content), "# First Task") {
		t.Error("first brief missing title")
	}
	if !strings.Contains(string(content), "First outcome") {
		t.Error("first brief missing outcome")
	}
}
