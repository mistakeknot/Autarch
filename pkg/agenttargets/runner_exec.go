package agenttargets

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// sandboxArgs maps agent names to the flags that enable sandboxing.
var sandboxArgs = map[string][]string{
	"claude": {"--sandbox"},
	"codex":  {"--sandbox"},
}

// ExecAgentRunner launches agents via os/exec with safety enforcement.
type ExecAgentRunner struct{}

// NewExecAgentRunner returns an ExecAgentRunner.
func NewExecAgentRunner() *ExecAgentRunner {
	return &ExecAgentRunner{}
}

func (r *ExecAgentRunner) Run(ctx context.Context, target ResolvedTarget, policy SafetyPolicy, workDir string, prompt string) (*RunHandle, error) {
	if target.Command == "" {
		return nil, fmt.Errorf("target %q has no command", target.Name)
	}

	// Build context with timeout if specified.
	runCtx := ctx
	var cancel context.CancelFunc
	if policy.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, policy.Timeout)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}

	// Build args: target args + sandbox flags + prompt.
	args := append([]string{}, target.Args...)
	if policy.Sandbox {
		if extra, ok := sandboxArgs[strings.ToLower(target.Name)]; ok {
			args = append(args, extra...)
		}
	}
	if prompt != "" {
		args = append(args, prompt)
	}

	cmd := exec.CommandContext(runCtx, target.Command, args...)
	cmd.Dir = workDir

	// Apply environment.
	if len(target.Env) > 0 {
		for k, v := range target.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Capture output with optional limit.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	startedAt := time.Now()
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start %q: %w", target.Command, err)
	}

	doneCh := make(chan struct{})
	id := fmt.Sprintf("%s-%d", target.Name, startedAt.UnixMilli())

	handle := &RunHandle{
		ID:        id,
		Target:    target,
		Policy:    policy,
		StartedAt: startedAt,
		Done:      doneCh,
		Cancel:    cancel,
	}

	var once sync.Once
	var result RunResult
	var waitErr error

	handle.Wait = func() (RunResult, error) {
		once.Do(func() {
			// Read output from both pipes.
			combined := io.MultiReader(stdoutPipe, stderrPipe)
			var buf bytes.Buffer
			var truncated bool

			if policy.MaxOutputBytes > 0 {
				limited := io.LimitReader(combined, policy.MaxOutputBytes+1)
				n, _ := io.Copy(&buf, limited)
				if n > policy.MaxOutputBytes {
					buf.Truncate(int(policy.MaxOutputBytes))
					truncated = true
				}
				// Drain remaining so process can finish.
				_, _ = io.Copy(io.Discard, io.MultiReader(stdoutPipe, stderrPipe))
			} else {
				_, _ = io.Copy(&buf, combined)
			}

			cmdErr := cmd.Wait()
			duration := time.Since(startedAt)

			exitCode := 0
			if cmdErr != nil {
				if exitErr, ok := cmdErr.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					waitErr = cmdErr
				}
			}

			timedOut := runCtx.Err() == context.DeadlineExceeded

			result = RunResult{
				ExitCode:  exitCode,
				Output:    buf.Bytes(),
				Truncated: truncated,
				TimedOut:  timedOut,
				Duration:  duration,
			}

			close(doneCh)
		})
		return result, waitErr
	}

	return handle, nil
}
