//go:build !unix

package homeask

import "os"

// The caller refuses non-regular paths with Lstat and checks the open file with fstat.
func openEvidenceFile(path string) (*os.File, error) { return os.Open(path) }
