package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesPraudeLayout(t *testing.T) {
	root := t.TempDir()
	if err := Init(root); err != nil {
		t.Fatal(err)
	}
	mustDir := []string{
		filepath.Join(root, ".praude"),
		filepath.Join(root, ".praude", "specs"),
		filepath.Join(root, ".praude", "research"),
		filepath.Join(root, ".praude", "suggestions"),
		filepath.Join(root, ".praude", "briefs"),
	}
	for _, dir := range mustDir {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			t.Fatalf("missing dir %s", dir)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".praude", "config.toml")); err != nil {
		t.Fatalf("expected config.toml")
	}
}
