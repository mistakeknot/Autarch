package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var ErrNotInitialized = errors.New("not a Tandemonium project")

func FindRoot(start string) (string, error) {
	cur := start
	for {
		cand := filepath.Join(cur, ".tandemonium")
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", ErrNotInitialized
		}
		cur = parent
	}
}

func StateDBPath(root string) string {
	return filepath.Join(root, ".tandemonium", "state.db")
}

func SpecsDir(root string) string {
	return filepath.Join(root, ".tandemonium", "specs")
}

func SessionsDir(root string) string {
	return filepath.Join(root, ".tandemonium", "sessions")
}

func AttachmentsDir(root string) string {
	return filepath.Join(root, ".tandemonium", "attachments")
}

func WorktreesDir(root string) string {
	return filepath.Join(root, ".tandemonium", "worktrees")
}

var taskIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func ValidateTaskID(id string) error {
	if !taskIDPattern.MatchString(id) {
		return fmt.Errorf("invalid task id: %q", id)
	}
	return nil
}

func TaskSpecPath(root, id string) (string, error) {
	if err := ValidateTaskID(id); err != nil {
		return "", err
	}
	return filepath.Join(SpecsDir(root), id+".yaml"), nil
}
