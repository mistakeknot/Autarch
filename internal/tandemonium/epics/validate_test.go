package epics

import (
	"os"
	"strings"
	"testing"
)

func TestValidateEpicsReportsErrors(t *testing.T) {
	epics := []Epic{
		{
			ID:       "EPIC-1",
			Title:    "Auth",
			Status:   Status("bogus"),
			Priority: Priority("p9"),
			Stories: []Story{
				{ID: "EPIC-002-S01", Title: "Bad story", Status: StatusTodo, Priority: PriorityP1},
			},
		},
	}

	errList := Validate(epics)
	if len(errList) == 0 {
		t.Fatalf("expected validation errors")
	}
}

func TestWriteValidationReport(t *testing.T) {
	dir := t.TempDir()
	errList := []ValidationError{{Path: "epics[0].id", Message: "invalid epic id"}}
	outPath, errPath, err := WriteValidationReport(dir, []byte("raw"), errList)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output file: %v", err)
	}
	if _, err := os.Stat(errPath); err != nil {
		t.Fatalf("expected error file: %v", err)
	}
	raw, err := os.ReadFile(errPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "epics[0].id") {
		t.Fatalf("expected error path in report")
	}
}
