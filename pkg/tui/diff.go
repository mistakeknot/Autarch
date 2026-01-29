package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// UnifiedDiff computes a unified diff between before and after content.
func UnifiedDiff(before, after, label string) ([]string, error) {
	if label == "" {
		label = "file"
	}
	name := filepath.Base(label)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "file"
	}

	tmpDir, err := os.MkdirTemp("", "autarch-diff-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	beforePath := filepath.Join(tmpDir, name+".before")
	afterPath := filepath.Join(tmpDir, name+".after")

	if err := os.WriteFile(beforePath, []byte(before), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(afterPath, []byte(after), 0o644); err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "diff", "--no-index", "--unified=3", beforePath, afterPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 {
				return nil, fmt.Errorf("git diff failed: %w", err)
			}
		} else {
			return nil, err
		}
	}

	trimmed := strings.TrimRight(string(output), "\n")
	if trimmed == "" {
		return nil, nil
	}

	lines := strings.Split(trimmed, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			lines[i] = fmt.Sprintf("diff --git a/%s b/%s", name, name)
		case strings.HasPrefix(line, "--- "):
			lines[i] = fmt.Sprintf("--- a/%s", name)
		case strings.HasPrefix(line, "+++ "):
			lines[i] = fmt.Sprintf("+++ b/%s", name)
		}
	}

	return lines, nil
}
