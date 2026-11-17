package agenttargets

import (
	"os/exec"
	"runtime"
	"syscall"
)

func configureParentDeath(cmd *exec.Cmd) { cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL }

// startProcess keeps the creating OS thread alive until Wait completes. Linux
// ties Pdeathsig to that thread, so unlocking immediately after Start is unsafe.
func startProcess(cmd *exec.Cmd) (func() error, error) {
	started := make(chan error, 1)
	wait := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		err := cmd.Start()
		started <- err
		if err != nil {
			return
		}
		<-wait
		done <- cmd.Wait()
	}()
	if err := <-started; err != nil {
		return nil, err
	}
	return func() error { close(wait); return <-done }, nil
}
