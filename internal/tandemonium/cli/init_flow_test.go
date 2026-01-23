package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAndValidateEpicsWritesReportOnError(t *testing.T) {
	planDir := t.TempDir()
	raw := []byte("epics:\n- id: EPIC-001\n  title: X\n  status: bogus\n  priority: p1\n")
	_, err := parseAndValidateEpics(raw, planDir)
	if err == nil {
		t.Fatalf("expected error")
	}
	if _, err := os.Stat(filepath.Join(planDir, "init-epics-output.yaml")); err != nil {
		t.Fatalf("expected output file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(planDir, "init-epics-errors.txt")); err != nil {
		t.Fatalf("expected error report: %v", err)
	}
}
