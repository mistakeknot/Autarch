package git

import "testing"

func TestCommitFilesNoGit(t *testing.T) {
	dir := t.TempDir()
	if err := CommitFiles(dir, []string{"x.txt"}, "msg"); err == nil {
		t.Fatalf("expected error without git")
	}
}
