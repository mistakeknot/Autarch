//go:build windows

package homeask

import "os"

func openRootRunScript(path string) (*os.File, error) { return os.Open(path) }
