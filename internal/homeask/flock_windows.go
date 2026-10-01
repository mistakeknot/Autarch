//go:build windows

package homeask

import "errors"

// lockFile is not available on Windows: filing runs on the dev servers, not there.
func lockFile(string) (func(), error) {
	return nil, errors.New("needs-mk filing is not supported on Windows")
}
