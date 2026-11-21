//go:build !windows

package predispatch

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// CommandContext otherwise kills only the direct child, leaving forked
	// checks running after timeout or parent cancellation.
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(syscall.Signal(0)); errors.Is(err, os.ErrProcessDone) {
			return err
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
