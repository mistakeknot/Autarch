package colony

import (
	"path/filepath"
	"testing"
)

func TestParseWorktrees(t *testing.T) {
	input := []byte(`worktree /repo
HEAD 123456
branch refs/heads/main

worktree /repo/feature
HEAD 999999
branch refs/heads/feature

worktree /repo/detached
HEAD 888888
detached
`)

	worktrees := parseWorktrees(input)
	if len(worktrees) != 3 {
		t.Fatalf("expected 3 worktrees, got %d", len(worktrees))
	}

	if worktrees[0].Path != "/repo" || worktrees[0].Branch != "main" {
		t.Fatalf("unexpected first worktree: %+v", worktrees[0])
	}
	if worktrees[1].Path != "/repo/feature" || worktrees[1].Branch != "feature" {
		t.Fatalf("unexpected second worktree: %+v", worktrees[1])
	}
	if worktrees[2].Path != "/repo/detached" || worktrees[2].Branch != "" {
		t.Fatalf("unexpected third worktree: %+v", worktrees[2])
	}
}

func TestSameOrChild(t *testing.T) {
	root := filepath.Join("/tmp", "repo")
	if !sameOrChild(root, root) {
		t.Fatal("expected root to match itself")
	}
	child := filepath.Join(root, "subdir")
	if !sameOrChild(root, child) {
		t.Fatal("expected child path to match root")
	}
	other := filepath.Join("/tmp", "repo-other")
	if sameOrChild(root, other) {
		t.Fatal("expected other path to not match root")
	}
}
