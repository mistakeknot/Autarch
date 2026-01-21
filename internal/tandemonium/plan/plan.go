package plan

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Run(in io.Reader, planDir string) error {
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		return err
	}
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
		return nil
	}
	vision := ""
	mvp := ""
	if scanner.Scan() {
		vision = scanner.Text()
	}
	if scanner.Scan() {
		mvp = scanner.Text()
	}
	if err := os.WriteFile(filepath.Join(planDir, "vision.md"), []byte(vision+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(planDir, "mvp.md"), []byte(mvp+"\n"), 0o644); err != nil {
		return err
	}
	return nil
}
