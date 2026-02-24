package project

import (
	"os"
	"path/filepath"
)

func Init(projectDir string) error {
	dirs := []string{
		filepath.Join(projectDir, ".coldwine"),
		filepath.Join(projectDir, ".coldwine", "specs"),
		filepath.Join(projectDir, ".coldwine", "sessions"),
		filepath.Join(projectDir, ".coldwine", "plan"),
		filepath.Join(projectDir, ".coldwine", "attachments"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
