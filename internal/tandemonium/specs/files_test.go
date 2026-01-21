package specs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFilesToModify(t *testing.T) {
	dir := t.TempDir()
	spec := []byte("id: TAND-001\nfiles_to_modify:\n  - a.txt\n  - b/c.txt\n")
	if err := os.WriteFile(filepath.Join(dir, "TAND-001.yaml"), spec, 0o644); err != nil {
		t.Fatal(err)
	}
	summaries, _ := LoadSummaries(dir)
	if len(summaries[0].FilesToModify) != 2 {
		t.Fatal("expected files_to_modify")
	}
}
