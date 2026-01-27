package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/autarch/internal/coldwine/epics"
)

func TestParseAndValidateEpicsAutoFixesFixableErrors(t *testing.T) {
	planDir := t.TempDir()
	// bad status is fixable — should auto-repair and succeed
	raw := []byte("epics:\n- id: EPIC-001\n  title: X\n  status: bogus\n  priority: p1\n")
	list, err := parseAndValidateEpics(raw, planDir)
	if err != nil {
		t.Fatalf("expected auto-fix to succeed, got: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(list))
	}
	if list[0].Status != epics.StatusTodo {
		t.Errorf("expected auto-fixed status 'todo', got %q", list[0].Status)
	}
}

func TestParseAndValidateEpicsWritesReportOnFatalError(t *testing.T) {
	planDir := t.TempDir()
	// missing title is fatal — cannot be auto-fixed
	raw := []byte("epics:\n- id: EPIC-001\n  title: \"\"\n  status: todo\n  priority: p1\n")
	_, err := parseAndValidateEpics(raw, planDir)
	if err == nil {
		t.Fatalf("expected error for missing title")
	}
	var valErr *InitValidationError
	if e, ok := err.(*InitValidationError); ok {
		valErr = e
	} else {
		t.Fatalf("expected *InitValidationError, got %T", err)
	}
	if !epics.HasFatalErrors(valErr.Errors) {
		t.Error("expected fatal errors")
	}
	if _, err := os.Stat(filepath.Join(planDir, "init-epics-output.yaml")); err != nil {
		t.Fatalf("expected output file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(planDir, "init-epics-errors.txt")); err != nil {
		t.Fatalf("expected error report: %v", err)
	}
}
