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
