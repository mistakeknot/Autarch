package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ErrNotInitialized = errors.New("not a Coldwine project")

func FindRoot(start string) (string, error) {
	cur := start
	for {
		// Check .coldwine first, then legacy .tandemonium
		cand := filepath.Join(cur, ".coldwine")
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cur, nil
		}
		legacy := filepath.Join(cur, ".tandemonium")
		if st, err := os.Stat(legacy); err == nil && st.IsDir() {
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
	return filepath.Join(root, ".coldwine", "state.db")
}

func SpecsDir(root string) string {
	return filepath.Join(root, ".coldwine", "specs")
}

func SessionsDir(root string) string {
	return filepath.Join(root, ".coldwine", "sessions")
}

func AttachmentsDir(root string) string {
	return filepath.Join(root, ".coldwine", "attachments")
}

func WorktreesDir(root string) string {
	return filepath.Join(root, ".coldwine", "worktrees")
}

var taskIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

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
	return SafePath(SpecsDir(root), id+".yaml")
}

func SafePath(base, name string) (string, error) {
	if base == "" {
		return "", errors.New("base path required")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("path traversal attempt: %q", name)
	}
	full := filepath.Join(base, name)
	rel, err := filepath.Rel(base, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path traversal attempt: %q", name)
	}
	return full, nil
}
