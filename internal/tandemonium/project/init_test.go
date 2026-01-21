package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitProjectCreatesLayout(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	want := []string{
		".tandemonium",
		".tandemonium/specs",
		".tandemonium/sessions",
		".tandemonium/plan",
	}
	for _, p := range want {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
}
