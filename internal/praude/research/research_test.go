package research

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestCreateResearchTemplateIncludesEvidenceAndOSSScan(t *testing.T) {
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
	if !strings.Contains(content, "Evidence refs") {
		t.Fatalf("expected evidence refs placeholder")
	}
	if !strings.Contains(content, "OSS project scan") {
		t.Fatalf("expected OSS project scan section")
	}
	if !strings.Contains(content, "learnings") || !strings.Contains(content, "bootstrapping") || !strings.Contains(content, "insights") {
		t.Fatalf("expected OSS scan details")
	}
}
