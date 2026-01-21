package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanRepoHonorsGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\nignored-dir/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "kept.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "ignored-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored-dir", "skip.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ScanRepo(root, Options{MaxBytesTotal: 1024, MaxBytesPerFile: 1024})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	paths := entryPaths(res.Entries)
	if !paths["kept.txt"] {
		t.Fatalf("expected kept.txt")
	}
	if paths["ignored.txt"] {
		t.Fatalf("expected ignored.txt to be excluded")
	}
	if paths["ignored-dir/skip.txt"] {
		t.Fatalf("expected ignored-dir/skip.txt to be excluded")
	}
}

func TestScanRepoSkipsLargeFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(strings.Repeat("a", 50)), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ScanRepo(root, Options{MaxBytesTotal: 1024, MaxBytesPerFile: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Entries) != 0 {
		t.Fatalf("expected big.txt to be skipped")
	}
	if !containsSkip(res.Skipped, "big.txt") {
		t.Fatalf("expected skip reason for big.txt")
	}
}

func TestScanRepoSkipsBinaryFiles(t *testing.T) {
	root := t.TempDir()
	data := []byte{0x00, 0x01, 0x02}
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ScanRepo(root, Options{MaxBytesTotal: 1024, MaxBytesPerFile: 1024})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Entries) != 0 {
		t.Fatalf("expected binary file to be skipped")
	}
	if !containsSkip(res.Skipped, "bin.dat") {
		t.Fatalf("expected skip reason for bin.dat")
	}
}

func TestScanRepoRespectsTotalBytes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "one.txt"), []byte("12345678"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "two.txt"), []byte("ABCDEFGH"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ScanRepo(root, Options{MaxBytesTotal: 10, MaxBytesPerFile: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	paths := entryPaths(res.Entries)
	if len(paths) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(paths))
	}
}

func entryPaths(entries []Entry) map[string]bool {
	out := make(map[string]bool)
	for _, e := range entries {
		out[e.Path] = true
	}
	return out
}

func containsSkip(skipped []string, name string) bool {
	for _, item := range skipped {
		if strings.Contains(item, name) {
			return true
		}
	}
	return false
}
