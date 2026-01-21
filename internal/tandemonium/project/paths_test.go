package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".tandemonium"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindRoot(dir)
	if err != nil {
		t.Fatalf("expected root, got error: %v", err)
	}
	if got != dir {
		t.Fatalf("expected %s, got %s", dir, got)
	}
}

func TestTaskSpecPath(t *testing.T) {
	path, err := TaskSpecPath("/tmp/root", "TAND-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join("/tmp/root", ".tandemonium", "specs", "TAND-001.yaml")
	if path != expected {
		t.Fatalf("expected %s, got %s", expected, path)
	}
}

func TestTaskSpecPathRejectsInvalidID(t *testing.T) {
	if _, err := TaskSpecPath("/tmp/root", "../evil"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := TaskSpecPath("/tmp/root", "bad/id"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := TaskSpecPath("/tmp/root", ""); err == nil {
		t.Fatal("expected error")
	}
}
