package daemon

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GitStatus represents the git state of a project directory.
type GitStatus struct {
	Branch        string
	Dirty         bool
	CommitsAhead  int
	CommitsBehind int
}

// GetGitStatus returns the git status for a directory.
// Returns nil if the directory is not a git repository.
func GetGitStatus(dir string) *GitStatus {
	// Check if it's a git repo
	gitDir := filepath.Join(dir, ".git")
	if !isDir(gitDir) {
		return nil
	}

	status := &GitStatus{}

	// Get current branch
	branchCmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	if output, err := branchCmd.Output(); err == nil {
		status.Branch = strings.TrimSpace(string(output))
	}

	// Check for uncommitted changes (dirty)
	dirtyCmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	if output, err := dirtyCmd.Output(); err == nil {
		status.Dirty = len(strings.TrimSpace(string(output))) > 0
	}

	// Get ahead/behind counts relative to upstream
	// Use @{u} which refers to the upstream branch
	revListCmd := exec.Command("git", "-C", dir, "rev-list", "--left-right", "--count", "HEAD...@{u}")
	if output, err := revListCmd.Output(); err == nil {
		parts := strings.Fields(strings.TrimSpace(string(output)))
		if len(parts) >= 2 {
			status.CommitsAhead, _ = strconv.Atoi(parts[0])
			status.CommitsBehind, _ = strconv.Atoi(parts[1])
		}
	}

	return status
}

// UpdateSessionGitStatus updates the git status fields on a session.
func UpdateSessionGitStatus(s *Session) {
	if s.ProjectPath == "" {
		return
	}

	status := GetGitStatus(s.ProjectPath)
	if status == nil {
		return
	}

	s.GitBranch = status.Branch
	s.GitDirty = status.Dirty
	s.CommitsAhead = status.CommitsAhead
	s.CommitsBehind = status.CommitsBehind
}

// isDir checks if a path is a directory.
func isDir(path string) bool {
	cmd := exec.Command("test", "-d", path)
	return cmd.Run() == nil
}
