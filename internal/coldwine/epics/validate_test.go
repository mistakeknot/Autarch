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

func TestValidateErrorsHaveSeverityAndFix(t *testing.T) {
	list := []Epic{
		{ID: "BAD", Title: "Auth", Status: "nope", Priority: "p9"},
	}
	errs := Validate(list)
	for _, e := range errs {
		if e.Severity == "" {
			t.Errorf("error at %s missing severity", e.Path)
		}
		if e.Fix == "" {
			t.Errorf("error at %s missing fix guidance", e.Path)
		}
	}
}

func TestAutoFixRepairsBadIDs(t *testing.T) {
	list := []Epic{
		{
			ID: "bad-id", Title: "Auth", Status: "nope", Priority: "high",
			Stories: []Story{
				{ID: "also-bad", Title: "Login", Status: "xyz", Priority: "low"},
			},
		},
	}
	remaining := AutoFix(list)
	if len(remaining) != 0 {
		t.Fatalf("expected 0 remaining errors after auto-fix, got %d: %v", len(remaining), remaining)
	}
	if list[0].ID != "EPIC-001" {
		t.Errorf("expected EPIC-001, got %s", list[0].ID)
	}
	if list[0].Status != StatusTodo {
		t.Errorf("expected status todo, got %s", list[0].Status)
	}
	if list[0].Stories[0].ID != "EPIC-001-S01" {
		t.Errorf("expected EPIC-001-S01, got %s", list[0].Stories[0].ID)
	}
}

func TestAutoFixCannotFixMissingTitle(t *testing.T) {
	list := []Epic{
		{ID: "EPIC-001", Title: "", Status: StatusTodo, Priority: PriorityP1},
	}
	remaining := AutoFix(list)
	if len(remaining) == 0 {
		t.Fatal("expected remaining errors for missing title")
	}
	if !HasFatalErrors(remaining) {
		t.Error("missing title should be fatal")
	}
}

func TestFormatValidationErrorsIncludesFix(t *testing.T) {
	errs := []ValidationError{
		{Path: "epics[0].id", Message: "invalid", Severity: SeverityFixable, Fix: "use EPIC-NNN"},
	}
	out := FormatValidationErrors(errs)
	if !strings.Contains(out, "[FIXABLE]") {
		t.Error("expected [FIXABLE] prefix")
	}
	if !strings.Contains(out, "fix: use EPIC-NNN") {
		t.Error("expected fix guidance in output")
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
