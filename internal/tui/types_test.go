package tui

import "testing"

func TestCreateSpecSummaryFromAnswers(t *testing.T) {
	answers := map[string]string{
		"vision":       "Build a great app",
		"users":        "Developers",
		"problem":      "Too complex",
		"platform":     "Web",
		"language":     "Go",
		"requirements": "Fast startup\nLow memory\nGood UX",
	}

	spec := CreateSpecSummaryFromAnswers("proj-1", answers, nil)

	if spec.Name != "Build a great app" {
		t.Errorf("Name = %q, want %q", spec.Name, "Build a great app")
	}
	if spec.Vision != "Build a great app" {
		t.Errorf("Vision = %q, want %q", spec.Vision, "Build a great app")
	}
	if spec.Users != "Developers" {
		t.Errorf("Users = %q, want %q", spec.Users, "Developers")
	}
	if spec.Problem != "Too complex" {
		t.Errorf("Problem = %q, want %q", spec.Problem, "Too complex")
	}
	if spec.Platform != "Web" {
		t.Errorf("Platform = %q, want %q", spec.Platform, "Web")
	}
	if spec.Language != "Go" {
		t.Errorf("Language = %q, want %q", spec.Language, "Go")
	}
	if len(spec.Requirements) != 3 {
		t.Fatalf("Requirements count = %d, want 3", len(spec.Requirements))
	}
	if spec.Requirements[0] != "Fast startup" {
		t.Errorf("Requirements[0] = %q, want %q", spec.Requirements[0], "Fast startup")
	}
	if spec.Requirements[1] != "Low memory" {
		t.Errorf("Requirements[1] = %q, want %q", spec.Requirements[1], "Low memory")
	}
	if spec.Requirements[2] != "Good UX" {
		t.Errorf("Requirements[2] = %q, want %q", spec.Requirements[2], "Good UX")
	}
}

func TestCreateSpecSummaryFromAnswersCommaSeparated(t *testing.T) {
	answers := map[string]string{
		"vision":       "App",
		"requirements": "fast, reliable, secure",
	}

	spec := CreateSpecSummaryFromAnswers("proj-2", answers, nil)

	if len(spec.Requirements) != 3 {
		t.Fatalf("Requirements count = %d, want 3", len(spec.Requirements))
	}
	if spec.Requirements[0] != "fast" {
		t.Errorf("Requirements[0] = %q, want %q", spec.Requirements[0], "fast")
	}
	if spec.Requirements[1] != "reliable" {
		t.Errorf("Requirements[1] = %q, want %q", spec.Requirements[1], "reliable")
	}
	if spec.Requirements[2] != "secure" {
		t.Errorf("Requirements[2] = %q, want %q", spec.Requirements[2], "secure")
	}
}

func TestCreateSpecSummaryFromAnswersEmpty(t *testing.T) {
	spec := CreateSpecSummaryFromAnswers("proj-3", map[string]string{}, nil)

	if spec.ProjectID != "proj-3" {
		t.Errorf("ProjectID = %q, want %q", spec.ProjectID, "proj-3")
	}
	if len(spec.Requirements) != 0 {
		t.Errorf("Requirements count = %d, want 0", len(spec.Requirements))
	}
}

func TestCreateSpecSummaryFromAnswersWithDecisions(t *testing.T) {
	answers := map[string]string{
		"vision":   "My project",
		"platform": "CLI",
	}
	decisions := []SpecDecision{
		{Key: "platform", Value: "CLI", Source: "user"},
	}

	spec := CreateSpecSummaryFromAnswers("proj-4", answers, decisions)

	if len(spec.Decisions) != 1 {
		t.Fatalf("Decisions count = %d, want 1", len(spec.Decisions))
	}
	if spec.Decisions[0].Key != "platform" {
		t.Errorf("Decisions[0].Key = %q, want %q", spec.Decisions[0].Key, "platform")
	}
}

func TestCreateSpecSummaryFromAnswersTrailingNewline(t *testing.T) {
	answers := map[string]string{
		"vision":       "App",
		"requirements": "req1\nreq2\n",
	}

	spec := CreateSpecSummaryFromAnswers("proj-5", answers, nil)

	if len(spec.Requirements) != 2 {
		t.Fatalf("Requirements count = %d, want 2", len(spec.Requirements))
	}
	if spec.Requirements[0] != "req1" {
		t.Errorf("Requirements[0] = %q, want %q", spec.Requirements[0], "req1")
	}
	if spec.Requirements[1] != "req2" {
		t.Errorf("Requirements[1] = %q, want %q", spec.Requirements[1], "req2")
	}
}

func TestCreateSpecSummaryFromAnswersWhitespaceRequirements(t *testing.T) {
	answers := map[string]string{
		"vision":       "App",
		"requirements": "  spaced  \n\n  trimmed  ",
	}

	spec := CreateSpecSummaryFromAnswers("proj-6", answers, nil)

	if len(spec.Requirements) != 2 {
		t.Fatalf("Requirements count = %d, want 2", len(spec.Requirements))
	}
	if spec.Requirements[0] != "spaced" {
		t.Errorf("Requirements[0] = %q, want %q", spec.Requirements[0], "spaced")
	}
	if spec.Requirements[1] != "trimmed" {
		t.Errorf("Requirements[1] = %q, want %q", spec.Requirements[1], "trimmed")
	}
}
