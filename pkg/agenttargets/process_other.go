//go:build !linux

package agenttargets

import "os/exec"

// Other platforms retain context cancellation, but abrupt parent death can
// leave an agent running. No portable os/exec parent-death primitive exists.
func startProcess(cmd *exec.Cmd) (func() error, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Wait, nil
}
