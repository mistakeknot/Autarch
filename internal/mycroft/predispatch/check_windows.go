package predispatch

import "os/exec"

// Windows retains CommandContext's default direct-process cancellation.
func setProcessGroup(cmd *exec.Cmd) {}
