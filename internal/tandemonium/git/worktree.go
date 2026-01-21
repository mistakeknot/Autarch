package git

import "os/exec"

func CreateWorktree(repoDir, path, branch string) error {
    cmd := exec.Command("git", "worktree", "add", "-b", branch, path)
    cmd.Dir = repoDir
    return cmd.Run()
}
