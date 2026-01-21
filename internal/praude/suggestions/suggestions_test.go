package suggestions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

func TestCreateSuggestionTemplateIncludesSections(t *testing.T) {
	dir := t.TempDir()
	path, err := Create(dir, "PRD-001", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, "## Summary") {
		t.Fatalf("expected summary section")
	}
	if !strings.Contains(content, "## Requirements") {
		t.Fatalf("expected requirements section")
	}
	if !strings.Contains(content, "## Critical User Journeys") {
		t.Fatalf("expected cuj section")
	}
}

func TestApplySuggestionUpdatesSummary(t *testing.T) {
	root := t.TempDir()
	specPath := filepath.Join(root, "PRD-001.yaml")
	specRaw := []byte("id: \"PRD-001\"\ntitle: \"Title\"\nsummary: \"Old\"\n")
	if err := os.WriteFile(specPath, specRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	sugg := Suggestion{Summary: "New summary"}
	if err := Apply(specPath, sugg); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "summary: New summary") {
		t.Fatalf("expected updated summary")
	}
	res, err := specs.Validate(updated, specs.ValidationOptions{Mode: specs.ValidationSoft, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("expected no errors")
	}
}

func TestLoadLatestSuggestionParsesSummary(t *testing.T) {
	dir := t.TempDir()
	path, err := Create(dir, "PRD-001", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(raw), "suggestion: \"\"", "suggestion: \"Updated summary\"", 1)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	sugg, latest, err := LoadLatest(dir, "PRD-001")
	if err != nil {
		t.Fatal(err)
	}
	if latest == "" {
		t.Fatalf("expected latest path")
	}
	if sugg.Summary != "Updated summary" {
		t.Fatalf("expected summary parsed")
	}
}

func TestLoadLatestSuggestionParsesRequirementsAndCUJ(t *testing.T) {
	dir := t.TempDir()
	path, err := Create(dir, "PRD-001", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	_ = path
	sugg, latest, err := LoadLatest(dir, "PRD-001")
	if err != nil {
		t.Fatal(err)
	}
	if latest == "" {
		t.Fatalf("expected latest path")
	}
	if len(sugg.Requirements) == 0 {
		t.Fatalf("expected requirements parsed")
	}
	if len(sugg.CriticalUserJourneys) == 0 {
		t.Fatalf("expected cuj parsed")
	}
}

func TestParseReadyIgnoresPendingSections(t *testing.T) {
	path, err := Create(t.TempDir(), "PRD-001", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ready := ParseReady(raw)
	if ready.Summary != "" {
		t.Fatalf("expected pending summary ignored")
	}
	if len(ready.Requirements) != 0 {
		t.Fatalf("expected pending requirements ignored")
	}
}

func TestParseReadyParsesReadySections(t *testing.T) {
	path, err := Create(t.TempDir(), "PRD-001", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(raw), "status: pending", "status: ready", -1)
	updated = strings.Replace(updated, "suggestion: \"\"", "suggestion: \"Updated summary\"", 1)
	updated = strings.Replace(updated, "\"REQ-001: Add requirement\"", "\"REQ-002: Agent requirement\"", 1)
	ready := ParseReady([]byte(updated))
	if ready.Summary != "Updated summary" {
		t.Fatalf("expected ready summary parsed")
	}
	if len(ready.Requirements) != 1 || ready.Requirements[0] != "REQ-002: Agent requirement" {
		t.Fatalf("expected ready requirements parsed")
	}
}
