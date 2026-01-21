package git

import (
	"fmt"
	"os/exec"
)

func CommitFiles(root string, files []string, message string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return err
	}
	args := append([]string{"-C", root, "add"}, files...)
	if err := exec.Command("git", args...).Run(); err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", root, "commit", "-m", message)
	return cmd.Run()
}

func EnsureRepo(root string) error {
	if err := exec.Command("git", "-C", root, "rev-parse", "--git-dir").Run(); err != nil {
		return fmt.Errorf("not a git repo")
	}
	return nil
}
