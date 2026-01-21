package git

import (
    "os"
    "os/exec"
    "path/filepath"
    "testing"
)

func TestCreateWorktree(t *testing.T) {
    dir := t.TempDir()
    cmd := exec.Command("git", "init")
    cmd.Dir = dir
    if err := cmd.Run(); err != nil {
        t.Fatalf("git init failed: %v", err)
    }
    // Configure identity and create an initial commit (required for worktrees)
    cmd = exec.Command("git", "config", "user.email", "test@example.com")
    cmd.Dir = dir
    if err := cmd.Run(); err != nil {
        t.Fatalf("git config email failed: %v", err)
    }
    cmd = exec.Command("git", "config", "user.name", "Test User")
    cmd.Dir = dir
    if err := cmd.Run(); err != nil {
        t.Fatalf("git config name failed: %v", err)
    }
    if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test\n"), 0o644); err != nil {
        t.Fatalf("write file failed: %v", err)
    }
    cmd = exec.Command("git", "add", "README.md")
    cmd.Dir = dir
    if err := cmd.Run(); err != nil {
        t.Fatalf("git add failed: %v", err)
    }
    cmd = exec.Command("git", "commit", "-m", "init")
    cmd.Dir = dir
    if err := cmd.Run(); err != nil {
        t.Fatalf("git commit failed: %v", err)
    }

    wtPath := filepath.Join(dir, "wt")
    if err := CreateWorktree(dir, wtPath, "feature/test"); err != nil {
        t.Fatalf("create worktree failed: %v", err)
    }
    if _, err := os.Stat(wtPath); err != nil {
        t.Fatalf("missing worktree: %v", err)
    }
}
