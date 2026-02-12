package agenttargets

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CodexBackend dispatches agent work via the Codex CLI.
type CodexBackend struct{}

// RunStreaming launches a Codex CLI process and returns a StreamHandle.
// Codex writes its final output to a file; stderr progress lines are streamed as events.
func (b *CodexBackend) RunStreaming(ctx context.Context, cfg DispatchConfig, workDir, prompt string) (*StreamHandle, error) {
	// Determine output file — use config or create a temp file.
	outputFile := cfg.OutputFile
	if outputFile == "" {
		f, err := os.CreateTemp("", "codex-output-*.md")
		if err != nil {
			return nil, fmt.Errorf("codex temp file: %w", err)
		}
		outputFile = f.Name()
		f.Close()
	} else if !filepath.IsAbs(outputFile) && workDir != "" {
		outputFile = filepath.Join(workDir, outputFile)
	}

	args := buildCodexArgs(cfg, prompt, outputFile)

	runCtx := ctx
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}

	cmd := exec.CommandContext(runCtx, "codex", args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	applyEnv(cmd, cfg)

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("codex stderr pipe: %w", err)
	}

	startedAt := time.Now()
	if err := cmd.Start(); err != nil {
		cancel()
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return nil, fmt.Errorf("codex CLI not found: install with 'npm install -g @openai/codex'")
		}
		return nil, fmt.Errorf("failed to start codex: %w", err)
	}

	events := make(chan StreamEvent)
	doneCh := make(chan struct{})
	id := fmt.Sprintf("codex-%d", startedAt.UnixMilli())

	var once sync.Once
	var result RunResult
	var waitErr error

	handle := &StreamHandle{
		RunHandle: &RunHandle{
			ID: id,
			Target: ResolvedTarget{
				Name:    "codex",
				Command: "codex",
			},
			StartedAt: startedAt,
			Done:      doneCh,
			Cancel:    cancel,
		},
		Events: events,
	}

	handle.RunHandle.Wait = func() (RunResult, error) {
		once.Do(func() {
			<-doneCh
		})
		return result, waitErr
	}

	go func() {
		defer close(events)
		defer func() {
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

			// Read output file for final result.
			var resultText string
			if data, err := os.ReadFile(outputFile); err == nil {
				resultText = strings.TrimSpace(string(data))
			}

			// Clean up temp file if we created it.
			if cfg.OutputFile == "" {
				os.Remove(outputFile)
			}

			if resultText != "" {
				select {
				case events <- StreamEvent{
					Type:    StreamResult,
					Text:    resultText,
					IsError: exitCode != 0,
					Backend: "codex",
				}:
				case <-runCtx.Done():
				}
			} else if exitCode != 0 {
				select {
				case events <- StreamEvent{
					Type:    StreamError,
					Text:    fmt.Sprintf("codex exited with code %d", exitCode),
					Backend: "codex",
				}:
				case <-runCtx.Done():
				}
			}

			result = RunResult{
				ExitCode: exitCode,
				Output:   []byte(resultText),
				TimedOut: runCtx.Err() == context.DeadlineExceeded,
				Duration: duration,
			}
			close(doneCh)
		}()

		// Stream stderr progress lines as StreamText events.
		buf := make([]byte, 4096)
		var line strings.Builder
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				for _, b := range buf[:n] {
					if b == '\n' || b == '\r' {
						if line.Len() > 0 {
							select {
							case events <- StreamEvent{
								Type:    StreamText,
								Text:    line.String(),
								Backend: "codex",
							}:
							case <-runCtx.Done():
								return
							}
							line.Reset()
						}
					} else {
						line.WriteByte(b)
					}
				}
				// Flush partial line too for real-time feedback.
				if line.Len() > 0 {
					select {
					case events <- StreamEvent{
						Type:    StreamText,
						Text:    line.String(),
						Backend: "codex",
					}:
					case <-runCtx.Done():
						return
					}
				}
			}
			if err != nil {
				break
			}
		}
	}()

	return handle, nil
}

// buildCodexArgs constructs the CLI arguments for a Codex invocation.
func buildCodexArgs(cfg DispatchConfig, prompt, outputFile string) []string {
	args := []string{"exec"}

	sandbox := cfg.Sandbox
	if sandbox == "" {
		sandbox = "workspace-write"
	}
	args = append(args, "-s", sandbox)

	if outputFile != "" {
		args = append(args, "-o", outputFile)
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	args = append(args, cfg.ExtraArgs...)
	args = append(args, prompt)
	return args
}
