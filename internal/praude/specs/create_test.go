package specs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateTemplateSpec(t *testing.T) {
	root := t.TempDir()
	specsDir := filepath.Join(root, ".praude", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := CreateTemplate(specsDir, time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !bytes.Contains(raw, []byte("id: \"PRD-001\"")) {
		t.Fatalf("expected PRD-001 id")
	}
	if !bytes.Contains(raw, []byte("strategic_context:")) {
		t.Fatalf("expected full schema")
	}
}

func TestTemplateIncludesCUJsAndEvidenceSections(t *testing.T) {
	root := t.TempDir()
	specsDir := filepath.Join(root, ".praude", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := CreateTemplate(specsDir, time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !bytes.Contains(raw, []byte("critical_user_journeys:")) {
		t.Fatalf("expected cuj section")
	}
	if !bytes.Contains(raw, []byte("Maintenance")) {
		t.Fatalf("expected maintenance cuj")
	}
	if !bytes.Contains(raw, []byte("market_research:")) {
		t.Fatalf("expected market research section")
	}
	if !bytes.Contains(raw, []byte("competitive_landscape:")) {
		t.Fatalf("expected competitive landscape section")
	}
	if !bytes.Contains(raw, []byte("priority: \"high\"")) || !bytes.Contains(raw, []byte("priority: \"low\"")) {
		t.Fatalf("expected cuj priorities as strings")
	}
	if !bytes.Contains(raw, []byte("REQ-001")) {
		t.Fatalf("expected requirement ids in template")
	}
}
