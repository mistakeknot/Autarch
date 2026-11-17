//go:build !linux && !windows

package agenttargets

import "os/exec"

func configureParentDeath(cmd *exec.Cmd) {}
