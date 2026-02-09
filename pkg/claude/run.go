package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// EventType represents the kind of streaming event from Claude CLI.
type EventType int

const (
	EventText          EventType = iota // Assistant text content delta
	EventThinkingStart                  // Extended thinking started
	EventThinking                       // Extended thinking content delta
	EventThinkingEnd                    // Extended thinking ended
	EventToolUse                        // Tool invocation (name + input)
	EventResult                         // Final result text
	EventError                          // Error occurred
	EventSessionID                      // Session ID extracted from init message
)

// StreamEvent is a single event from a Claude CLI streaming session.
type StreamEvent struct {
	Type      EventType
	Text      string         // Content for text/thinking/result/error events
	ToolName  string         // For EventToolUse
	ToolInput map[string]any // For EventToolUse
	SessionID string         // For EventSessionID
	IsError   bool           // For EventResult - whether Claude flagged it as error
}

// RunStreaming executes `claude` with --output-format stream-json and sends
// parsed events to the returned channel. The channel is closed when the
// process exits. Cancel the context to kill the process.
func RunStreaming(ctx context.Context, cwd string, args []string) (<-chan StreamEvent, error) {
	cmdArgs := append([]string(nil), args...)
	if !hasStreamJSONOutputFormat(cmdArgs) {
		cmdArgs = append([]string{"--output-format", "stream-json"}, cmdArgs...)
	}

	cmd := exec.CommandContext(ctx, "claude", cmdArgs...)
	cmd.Dir = cwd

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start claude: %w", err)
	}

	events := make(chan StreamEvent)
	go func() {
		defer close(events)

		emit := func(event StreamEvent) {
			select {
			case events <- event:
			case <-ctx.Done():
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

			var msg rawMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				emit(StreamEvent{
					Type: EventError,
					Text: fmt.Sprintf("failed to parse stream line: %v", err),
				})
				continue
			}

			if msg.Type == "system" && msg.Subtype == "init" && msg.SessionID != "" {
				emit(StreamEvent{
					Type:      EventSessionID,
					SessionID: msg.SessionID,
				})
			}

			if msg.Type == "assistant" && msg.Message != nil {
				for _, block := range msg.Message.Content {
					switch block.Type {
					case "text":
						if block.Text != "" {
							emit(StreamEvent{Type: EventText, Text: block.Text})
						}
					case "thinking_start":
						emit(StreamEvent{Type: EventThinkingStart})
					case "thinking":
						text := block.Thinking
						if text == "" {
							text = block.Text
						}
						emit(StreamEvent{Type: EventThinking, Text: text})
					case "thinking_end":
						emit(StreamEvent{Type: EventThinkingEnd})
					case "tool_use":
						emit(StreamEvent{
							Type:      EventToolUse,
							ToolName:  block.Name,
							ToolInput: block.Input,
						})
					}
				}
			}

			if msg.Type == "result" {
				sawResult = true
				emit(StreamEvent{
					Type:    EventResult,
					Text:    msg.Result,
					IsError: msg.IsError,
				})
			}
		}

		if err := scanner.Err(); err != nil {
			emit(StreamEvent{
				Type: EventError,
				Text: fmt.Sprintf("failed to read stream output: %v", err),
			})
		}

		if err := cmd.Wait(); err != nil && !sawResult {
			emit(StreamEvent{
				Type: EventError,
				Text: fmt.Sprintf("claude failed: %v", err),
			})
		}
	}()

	return events, nil
}

// Run executes Claude and returns the final result text.
// This is a convenience wrapper around RunStreaming that blocks until complete.
func Run(ctx context.Context, cwd string, args []string) (string, error) {
	events, err := RunStreaming(ctx, cwd, args)
	if err != nil {
		return "", err
	}

	var resultText string
	var resultIsError bool
	var sawResult bool
	var firstErr string

	for event := range events {
		switch event.Type {
		case EventResult:
			resultText = event.Text
			resultIsError = event.IsError
			sawResult = true
		case EventError:
			if firstErr == "" {
				firstErr = event.Text
			}
		}
	}

	if resultIsError {
		return "", fmt.Errorf("claude returned error: %s", resultText)
	}
	if sawResult {
		return strings.TrimSpace(resultText), nil
	}
	if firstErr != "" {
		return "", fmt.Errorf("%s", firstErr)
	}

	return strings.TrimSpace(resultText), nil
}

func hasStreamJSONOutputFormat(args []string) bool {
	for i := 0; i < len(args); i++ {
		if args[i] == "--output-format" && i+1 < len(args) && args[i+1] == "stream-json" {
			return true
		}
		if strings.HasPrefix(args[i], "--output-format=") && strings.TrimPrefix(args[i], "--output-format=") == "stream-json" {
			return true
		}
	}
	return false
}

type rawMessage struct {
	Type      string      `json:"type"`
	Subtype   string      `json:"subtype"`
	SessionID string      `json:"session_id"`
	Result    string      `json:"result"`
	IsError   bool        `json:"is_error"`
	Message   *rawContent `json:"message"`
}

type rawContent struct {
	Content []rawBlock `json:"content"`
}

type rawBlock struct {
	Type     string         `json:"type"`
	Text     string         `json:"text"`
	Name     string         `json:"name"`
	Input    map[string]any `json:"input"`
	Thinking string         `json:"thinking"`
}
