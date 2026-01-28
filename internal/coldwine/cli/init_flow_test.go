package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/pkg/agenttargets"
)

type fakeRunner struct {
	target agenttargets.ResolvedTarget
	policy agenttargets.SafetyPolicy
	work   string
	prompt string
	result agenttargets.RunResult
	err    error
}

func (f *fakeRunner) Run(_ context.Context, target agenttargets.ResolvedTarget, policy agenttargets.SafetyPolicy, workDir string, prompt string) (*agenttargets.RunHandle, error) {
	f.target = target
	f.policy = policy
	f.work = workDir
	f.prompt = prompt
	if f.err != nil {
		return nil, f.err
	}
	done := make(chan struct{})
	handle := &agenttargets.RunHandle{
		ID:     "fake",
		Target: target,
		Policy: policy,
		Done:   done,
	}
	handle.Wait = func() (agenttargets.RunResult, error) {
		close(done)
		return f.result, nil
	}
	return handle, nil
}

func TestRunAgentWithRunnerSuccess(t *testing.T) {
	runner := &fakeRunner{
		result: agenttargets.RunResult{ExitCode: 0, Output: []byte("ok")},
	}
	target := agenttargets.ResolvedTarget{Name: "claude", Command: "claude"}
	out, err := runAgentWithRunner(context.Background(), runner, target, agenttargets.DefaultSafetyPolicy(), "/tmp", "prompt.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("expected output ok, got %q", string(out))
	}
	if runner.prompt != "prompt.md" {
		t.Fatalf("expected prompt path to be passed")
	}
	if runner.policy.Timeout != 30*time.Minute || runner.policy.MaxOutputBytes == 0 || !runner.policy.Sandbox {
		t.Fatalf("unexpected policy: %+v", runner.policy)
	}
}

func TestRunAgentWithRunnerHandlesExitCode(t *testing.T) {
	runner := &fakeRunner{
		result: agenttargets.RunResult{ExitCode: 2, Output: []byte("fail")},
	}
	target := agenttargets.ResolvedTarget{Name: "claude", Command: "claude"}
	_, err := runAgentWithRunner(context.Background(), runner, target, agenttargets.DefaultSafetyPolicy(), "/tmp", "prompt.md")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunAgentWithRunnerHandlesTimeout(t *testing.T) {
	runner := &fakeRunner{
		result: agenttargets.RunResult{ExitCode: 0, TimedOut: true},
	}
	target := agenttargets.ResolvedTarget{Name: "claude", Command: "claude"}
	_, err := runAgentWithRunner(context.Background(), runner, target, agenttargets.DefaultSafetyPolicy(), "/tmp", "prompt.md")
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestRunAgentWithRunnerPropagatesRunError(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	target := agenttargets.ResolvedTarget{Name: "claude", Command: "claude"}
	_, err := runAgentWithRunner(context.Background(), runner, target, agenttargets.DefaultSafetyPolicy(), "/tmp", "prompt.md")
	if err == nil {
		t.Fatalf("expected error")
	}
}
