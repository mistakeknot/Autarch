package project

import (
	"os"
	"path/filepath"
)

func Init(projectDir string) error {
	dirs := []string{
		filepath.Join(projectDir, ".tandemonium"),
		filepath.Join(projectDir, ".tandemonium", "specs"),
		filepath.Join(projectDir, ".tandemonium", "sessions"),
		filepath.Join(projectDir, ".tandemonium", "plan"),
		filepath.Join(projectDir, ".tandemonium", "attachments"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
