package claude

import (
	"context"
	"fmt"
	"strings"

	"github.com/mistakeknot/autarch/pkg/agenttargets"
)

// EventType represents the kind of streaming event from Claude CLI.
// Deprecated: Use agenttargets.StreamEventType instead.
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
// Deprecated: Use agenttargets.StreamEvent instead.
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
//
// This is now a thin shim over agenttargets.Dispatch().
func RunStreaming(ctx context.Context, cwd string, args []string) (<-chan StreamEvent, error) {
	// Extract prompt and build config from args.
	cfg, prompt := configFromArgs(args)

	handle, err := agenttargets.Dispatch(ctx, cfg, cwd, prompt)
	if err != nil {
		return nil, err
	}

	events := make(chan StreamEvent)
	go func() {
		defer close(events)
		for event := range handle.Events {
			converted := convertEvent(event)
			select {
			case events <- converted:
			case <-ctx.Done():
				return
			}
		}
	}()

	return events, nil
}

// Run executes Claude and returns the final result text.
// This is a convenience wrapper around RunStreaming that blocks until complete.
//
// This is now a thin shim over agenttargets.Dispatch().
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

// configFromArgs extracts a DispatchConfig and prompt from raw CLI args.
// This handles the common patterns used by callers: -p <prompt>, --resume, --model, etc.
func configFromArgs(args []string) (agenttargets.DispatchConfig, string) {
	cfg := agenttargets.DispatchConfig{
		PreferredBackend: agenttargets.BackendSubscriptionCLI,
		PreferredAgent:   "claude",
	}

	var prompt string
	var extraArgs []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p":
			if i+1 < len(args) {
				prompt = args[i+1]
				i++
			}
		case "--resume":
			if i+1 < len(args) {
				cfg.SessionID = args[i+1]
				i++
			}
		case "--model":
			if i+1 < len(args) {
				cfg.Model = args[i+1]
				i++
			}
		case "--output-format":
			// Skip — Dispatch always uses stream-json.
			if i+1 < len(args) {
				i++
			}
		case "--verbose":
			cfg.Verbose = true
		case "--print":
			cfg.Print = true
		case "--dangerously-skip-permissions":
			cfg.Sandbox = "danger-full-access"
		default:
			// Check for --key=value forms.
			if strings.HasPrefix(args[i], "--output-format=") {
				continue
			}
			extraArgs = append(extraArgs, args[i])
		}
	}

	cfg.ExtraArgs = extraArgs
	return cfg, prompt
}

// convertEvent converts an agenttargets.StreamEvent to a claude.StreamEvent.
func convertEvent(e agenttargets.StreamEvent) StreamEvent {
	var t EventType
	switch e.Type {
	case agenttargets.StreamText:
		t = EventText
	case agenttargets.StreamThinkingStart:
		t = EventThinkingStart
	case agenttargets.StreamThinking:
		t = EventThinking
	case agenttargets.StreamThinkingEnd:
		t = EventThinkingEnd
	case agenttargets.StreamToolUse:
		t = EventToolUse
	case agenttargets.StreamResult:
		t = EventResult
	case agenttargets.StreamError:
		t = EventError
	case agenttargets.StreamSessionID:
		t = EventSessionID
	}
	return StreamEvent{
		Type:      t,
		Text:      e.Text,
		ToolName:  e.ToolName,
		ToolInput: e.ToolInput,
		SessionID: e.SessionID,
		IsError:   e.IsError,
	}
}
