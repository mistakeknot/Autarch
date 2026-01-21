package tmux

import "testing"

func TestExecRunnerRunsCommand(t *testing.T) {
	runner := &ExecRunner{}
	if err := runner.Run("true"); err != nil {
		t.Fatalf("expected true to succeed: %v", err)
	}
}
