package git

import (
	"errors"
	"strings"
)

var ErrBranchNotFound = errors.New("branch not found for task")

func ListBranches(r Runner) ([]string, error) {
	out, err := r.Run("git", "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	return ParseNameOnly(out), nil
}

func BranchForTask(r Runner, taskID string) (string, error) {
	branches, err := ListBranches(r)
	if err != nil {
		return "", err
	}
	return matchBranch(branches, taskID)
}

// BranchesForTasks resolves branches for multiple task IDs with a single
// git call. Returns a map of taskID → branch for all matched tasks.
func BranchesForTasks(r Runner, taskIDs []string) (map[string]string, error) {
	branches, err := ListBranches(r)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(taskIDs))
	for _, id := range taskIDs {
		if branch, err := matchBranch(branches, id); err == nil {
			result[id] = branch
		}
	}
	return result, nil
}

func matchBranch(branches []string, taskID string) (string, error) {
	for _, b := range branches {
		if strings.EqualFold(b, taskID) {
			return b, nil
		}
	}
	lowerID := strings.ToLower(taskID)
	for _, b := range branches {
		if strings.Contains(strings.ToLower(b), lowerID) {
			return b, nil
		}
	}
	return "", ErrBranchNotFound
}
