package specs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateQuickSpec(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC)
	path, err := CreateQuickSpec(dir, "Fix login timeout bug", now)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("quick_mode: true")) {
		t.Fatalf("expected quick_mode marker")
	}
	if !bytes.Contains(raw, []byte("status: assigned")) {
		t.Fatalf("expected assigned status")
	}
	if !bytes.Contains(raw, []byte("Quick task")) {
		t.Fatalf("expected quick task summary note")
	}
}

func TestCreateQuickSpecUsesNextID(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "TAND-002.yaml"), []byte("id: TAND-002\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := CreateQuickSpec(dir, "Next task", time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "TAND-003.yaml" {
		t.Fatalf("expected next id TAND-003, got %s", filepath.Base(path))
	}
}
