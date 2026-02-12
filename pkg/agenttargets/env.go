package agenttargets

import (
	"fmt"
	"os"
	"os/exec"
)

// Autarch environment variables injected into agent processes.
const (
	EnvAutarchEnabled = "AUTARCH_ENABLED"
	EnvAutarchTool    = "AUTARCH_TOOL"
	EnvAutarchSpecID  = "AUTARCH_SPEC_ID"
	EnvAutarchTaskID  = "AUTARCH_TASK_ID"
)

// applyEnv sets the environment on cmd, merging the parent environment with
// cfg.Env and the AUTARCH_ENABLED=1 marker. cfg.Env values take precedence
// over inherited environment variables.
func applyEnv(cmd *exec.Cmd, cfg DispatchConfig) {
	if len(cfg.Env) == 0 {
		// Still inject AUTARCH_ENABLED so agents know they're orchestrated.
		cmd.Env = append(os.Environ(), EnvAutarchEnabled+"=1")
		return
	}

	env := os.Environ()
	env = append(env, EnvAutarchEnabled+"=1")
	for k, v := range cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
}
