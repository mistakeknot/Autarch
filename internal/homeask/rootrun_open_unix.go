//go:build !windows

package homeask

import (
	"os"
	"syscall"
)

func openRootRunScript(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
