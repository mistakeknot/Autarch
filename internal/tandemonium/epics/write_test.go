package epics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteEpicsCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	epics := []Epic{
		{
			ID:       "EPIC-001",
			Title:    "Auth",
			Summary:  "User auth and sessions",
			Status:   StatusTodo,
			Priority: PriorityP1,
			AcceptanceCriteria: []string{
				"Login works",
			},
			Risks: []string{
				"OAuth latency",
			},
			Estimates: "M",
			Stories: []Story{
				{
					ID:        "EPIC-001-S01",
					Title:     "Login form",
					Summary:   "Email/password flow",
					Status:    StatusTodo,
					Priority:  PriorityP1,
					Estimates: "S",
				},
			},
		},
	}

	if err := WriteEpics(dir, epics, WriteOptions{Existing: ExistingOverwrite}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "EPIC-001.yaml")); err != nil {
		t.Fatalf("expected epic file: %v", err)
	}
}

func TestWriteEpicsUsesEstimatesKey(t *testing.T) {
	dir := t.TempDir()
	epic := Epic{
		ID:        "EPIC-002",
		Title:     "Scope",
		Summary:   "Define scope",
		Status:    StatusTodo,
		Priority:  PriorityP2,
		Estimates: "M",
	}
	if err := WriteEpics(dir, []Epic{epic}, WriteOptions{Existing: ExistingOverwrite}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "EPIC-002.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "estimates:") {
		t.Fatalf("expected estimates key in yaml")
	}
}
