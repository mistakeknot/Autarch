package homeask

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ResolveBBBinary honors the configured CLI before PATH and the user-local install.
func ResolveBBBinary() (string, error) {
	executable := func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0111 != 0)
	}
	if path := os.Getenv("BB_CLI"); path != "" && executable(path) {
		return path, nil
	}
	if path, err := exec.LookPath("bb"); err == nil {
		return path, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".local", "bin", "bb")
		if executable(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("bb executable unavailable: set BB_CLI, add bb to PATH, or install it in the user-local bin directory")
}
