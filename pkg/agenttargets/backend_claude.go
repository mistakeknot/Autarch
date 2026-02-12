package agenttargets

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// claudeRawMessage is the top-level JSON structure from Claude CLI stream-json output.
type claudeRawMessage struct {
	Type      string            `json:"type"`
	Subtype   string            `json:"subtype"`
	SessionID string            `json:"session_id"`
	Result    string            `json:"result"`
	IsError   bool              `json:"is_error"`
	Message   *claudeRawContent `json:"message"`
}

// claudeRawContent holds the message content array.
type claudeRawContent struct {
	Content []claudeRawBlock `json:"content"`
}

// claudeRawBlock represents a single content block (text, thinking, tool_use).
type claudeRawBlock struct {
	Type     string         `json:"type"`
	Text     string         `json:"text"`
	Name     string         `json:"name"`
	Input    map[string]any `json:"input"`
	Thinking string         `json:"thinking"`
}

// ClaudeBackend dispatches agent work via the Claude CLI.
type ClaudeBackend struct{}

// RunStreaming launches a Claude CLI process with stream-json output and returns
// a StreamHandle with parsed events.
func (b *ClaudeBackend) RunStreaming(ctx context.Context, cfg DispatchConfig, workDir, prompt string) (*StreamHandle, error) {
	args := buildClaudeArgs(cfg, prompt)

	runCtx := ctx
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}

	cmd := exec.CommandContext(runCtx, "claude", args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	applyEnv(cmd, cfg)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("claude stdout pipe: %w", err)
	}

	startedAt := time.Now()
	if err := cmd.Start(); err != nil {
		cancel()
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return nil, fmt.Errorf("claude CLI not found: install with 'npm install -g @anthropic-ai/claude-code'")
		}
		return nil, fmt.Errorf("failed to start claude: %w", err)
	}

	events := make(chan StreamEvent)
	doneCh := make(chan struct{})
	id := fmt.Sprintf("claude-%d", startedAt.UnixMilli())

	var once sync.Once
	var result RunResult
	var waitErr error

	handle := &StreamHandle{
		RunHandle: &RunHandle{
			ID: id,
			Target: ResolvedTarget{
				Name:    "claude",
				Command: "claude",
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

			result = RunResult{
				ExitCode: exitCode,
				TimedOut: runCtx.Err() == context.DeadlineExceeded,
				Duration: duration,
			}
			close(doneCh)
		}()

		emit := func(event StreamEvent) {
			event.Backend = "claude"
			select {
			case events <- event:
			case <-runCtx.Done():
			}
		}

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

		sawResult := false
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var msg claudeRawMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				emit(StreamEvent{
					Type: StreamError,
					Text: fmt.Sprintf("failed to parse stream line: %v", err),
				})
				continue
			}

			if msg.Type == "system" && msg.Subtype == "init" && msg.SessionID != "" {
				emit(StreamEvent{
					Type:      StreamSessionID,
					SessionID: msg.SessionID,
				})
			}

			if msg.Type == "assistant" && msg.Message != nil {
				for _, block := range msg.Message.Content {
					switch block.Type {
					case "text":
						if block.Text != "" {
							emit(StreamEvent{Type: StreamText, Text: block.Text})
						}
					case "thinking_start":
						emit(StreamEvent{Type: StreamThinkingStart})
					case "thinking":
						text := block.Thinking
						if text == "" {
							text = block.Text
						}
						emit(StreamEvent{Type: StreamThinking, Text: text})
					case "thinking_end":
						emit(StreamEvent{Type: StreamThinkingEnd})
					case "tool_use":
						emit(StreamEvent{
							Type:      StreamToolUse,
							ToolName:  block.Name,
							ToolInput: block.Input,
						})
					}
				}
			}

			if msg.Type == "result" {
				sawResult = true
				emit(StreamEvent{
					Type:    StreamResult,
					Text:    msg.Result,
					IsError: msg.IsError,
				})
			}
		}

		if err := scanner.Err(); err != nil {
			emit(StreamEvent{
				Type: StreamError,
				Text: fmt.Sprintf("failed to read stream output: %v", err),
			})
		}

		if !sawResult {
			// If no result event was received, the process likely failed
			emit(StreamEvent{
				Type: StreamError,
				Text: "claude process ended without result",
			})
		}
	}()

	return handle, nil
}

// buildClaudeArgs constructs the CLI arguments for a Claude invocation.
func buildClaudeArgs(cfg DispatchConfig, prompt string) []string {
	args := []string{"-p", prompt, "--output-format", "stream-json"}

	if cfg.Verbose {
		args = append(args, "--verbose")
	}
	if cfg.Print {
		args = append(args, "--print")
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.SessionID != "" {
		args = append(args, "--resume", cfg.SessionID)
	}
	if cfg.Sandbox == "danger-full-access" {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, cfg.ExtraArgs...)
	return args
}

// ParseClaudeStreamLine parses a single line of Claude stream-json output into a StreamEvent.
// Returns nil if the line should be skipped (empty or non-parseable).
// Exported for testing.
func ParseClaudeStreamLine(line string) []StreamEvent {
	if line == "" {
		return nil
	}

	var msg claudeRawMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return []StreamEvent{{
			Type:    StreamError,
			Text:    fmt.Sprintf("failed to parse stream line: %v", err),
			Backend: "claude",
		}}
	}

	var events []StreamEvent

	if msg.Type == "system" && msg.Subtype == "init" && msg.SessionID != "" {
		events = append(events, StreamEvent{
			Type:      StreamSessionID,
			SessionID: msg.SessionID,
			Backend:   "claude",
		})
	}

	if msg.Type == "assistant" && msg.Message != nil {
		for _, block := range msg.Message.Content {
			switch block.Type {
			case "text":
				if block.Text != "" {
					events = append(events, StreamEvent{
						Type:    StreamText,
						Text:    block.Text,
						Backend: "claude",
					})
				}
			case "thinking_start":
				events = append(events, StreamEvent{
					Type:    StreamThinkingStart,
					Backend: "claude",
				})
			case "thinking":
				text := block.Thinking
				if text == "" {
					text = block.Text
				}
				events = append(events, StreamEvent{
					Type:    StreamThinking,
					Text:    text,
					Backend: "claude",
				})
			case "thinking_end":
				events = append(events, StreamEvent{
					Type:    StreamThinkingEnd,
					Backend: "claude",
				})
			case "tool_use":
				events = append(events, StreamEvent{
					Type:      StreamToolUse,
					ToolName:  block.Name,
					ToolInput: block.Input,
					Backend:   "claude",
				})
			}
		}
	}

	if msg.Type == "result" {
		events = append(events, StreamEvent{
			Type:    StreamResult,
			Text:    msg.Result,
			IsError: msg.IsError,
			Backend: "claude",
		})
	}

	return events
}
