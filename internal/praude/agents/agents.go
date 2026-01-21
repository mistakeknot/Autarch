package agents

import (
	"fmt"
	"os"
	"os/exec"
)

type Profile struct {
	Command string
	Args    []string
}

var lookPath = exec.LookPath

func Resolve(cfg map[string]Profile, name string) (Profile, error) {
	p, ok := cfg[name]
	if !ok {
		return Profile{}, fmt.Errorf("unknown agent %s", name)
	}
	return p, nil
}

func Launch(p Profile, briefPath string) error {
	return launchWithEnv(p, briefPath, nil)
}

func LaunchSubagent(p Profile, briefPath string) error {
	return launchWithEnv(p, briefPath, []string{"PRAUDE_SUBAGENT=1"})
}

func launchWithEnv(p Profile, briefPath string, extraEnv []string) error {
	if _, err := lookPath(p.Command); err != nil {
		return err
	}
	cmd := buildCommand(p, briefPath, extraEnv)
	return cmd.Start()
}

func buildCommand(p Profile, briefPath string, extraEnv []string) *exec.Cmd {
	args := append([]string{}, p.Args...)
	args = append(args, briefPath)
	cmd := exec.Command(p.Command, args...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	return cmd
}
