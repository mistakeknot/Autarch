package specs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindByID(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "TAND-001.yaml"), []byte("id: TAND-001\nfiles_to_modify:\n  - a.txt\n"), 0o644)
	summaries, _ := LoadSummaries(dir)
	s, ok := FindByID(summaries, "TAND-001")
	if !ok || len(s.FilesToModify) != 1 {
		t.Fatal("expected match")
	}
}
