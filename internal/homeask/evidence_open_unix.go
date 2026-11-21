//go:build unix

package homeask

import (
	"os"
	"syscall"
)

// Non-blocking open also protects the interval after Lstat if a regular file
// is replaced with a FIFO. OpenFile supplies close-on-exec; fstat is checked by the caller.
func openEvidenceFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
}
