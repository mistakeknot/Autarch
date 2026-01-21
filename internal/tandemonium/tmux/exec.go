package tmux

import "os/exec"

type ExecRunner struct{}

func (e *ExecRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}
