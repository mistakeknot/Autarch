package git

import "os/exec"

type Runner interface {
	Run(name string, args ...string) (string, error)
}

type ExecRunner struct{}

func (e *ExecRunner) Run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

func DiffNameOnly(r Runner, rev string) ([]string, error) {
	out, err := r.Run("git", "diff", "--name-only", rev)
	if err != nil {
		return nil, err
	}
	return ParseNameOnly(out), nil
}

func DiffNameOnlyRange(r Runner, base, branch string) ([]string, error) {
	out, err := r.Run("git", "diff", "--name-only", base+".."+branch)
	if err != nil {
		return nil, err
	}
	return ParseNameOnly(out), nil
}
