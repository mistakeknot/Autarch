package specs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSummaries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PRD-001.yaml"), []byte("id: \"PRD-001\"\ntitle: \"A\"\nsummary: \"S\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, _ := LoadSummaries(dir)
	if len(list) != 1 || list[0].ID != "PRD-001" {
		t.Fatalf("expected summary")
	}
}

func TestLoadSpec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "PRD-001.yaml")
	raw := []byte("id: \"PRD-001\"\ntitle: \"A\"\nsummary: \"S\"\nrequirements:\n  - \"REQ-001: R\"\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := LoadSpec(path)
	if err != nil {
		t.Fatal(err)
	}
	if spec.ID != "PRD-001" || len(spec.Requirements) != 1 {
		t.Fatalf("expected full spec")
	}
}
