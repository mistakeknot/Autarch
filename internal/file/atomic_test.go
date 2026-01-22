package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtomicWriteFileCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")
	if err := AtomicWriteFile(path, []byte("id: T1\n"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected output file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Fatalf("expected temp file removed, found %s", entry.Name())
		}
	}
}
