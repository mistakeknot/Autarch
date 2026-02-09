package tui

import (
	"context"
	"time"
)

// StreamMsg is the interface for streaming message events.
// Implement one of the concrete types below.
type StreamMsg interface {
	streamMsg() // marker method, unexported
}

// TextDelta is an incremental text content update from the assistant.
type TextDelta struct {
	Text string
}

func (TextDelta) streamMsg() {}

// ReasoningStart signals that extended thinking has begun.
type ReasoningStart struct{}

func (ReasoningStart) streamMsg() {}

// ReasoningDelta is an incremental reasoning/thinking content update.
type ReasoningDelta struct {
	Text string
}

func (ReasoningDelta) streamMsg() {}

// ReasoningEnd signals that extended thinking has completed.
type ReasoningEnd struct {
	Duration time.Duration
}

func (ReasoningEnd) streamMsg() {}

// ToolCallStart signals that a tool invocation has begun.
type ToolCallStart struct {
	ID   string
	Name string
}

func (ToolCallStart) streamMsg() {}

// ToolCallInput is incremental tool input content.
type ToolCallInput struct {
	ID    string
	Input string
}

func (ToolCallInput) streamMsg() {}

// ToolCallResult signals that a tool call has completed.
type ToolCallResult struct {
	ID      string
	Output  string
	IsError bool
}

func (ToolCallResult) streamMsg() {}

// StreamError signals an error during streaming.
type StreamError struct {
	Err error
}

func (StreamError) streamMsg() {}

// StreamDone signals that the stream is complete.
type StreamDone struct {
	FinishReason string
	SessionID    string // Claude CLI session ID for multi-turn continuation
}

func (StreamDone) streamMsg() {}

// ChatState represents the current state of a chat interaction.
type ChatState int

const (
	ChatIdle      ChatState = iota // No active streaming
	ChatThinking                   // Agent is thinking (extended thinking)
	ChatStreaming                  // Agent is producing text output
	ChatError                      // An error occurred
)

// String returns a human-readable label for the chat state.
func (s ChatState) String() string {
	switch s {
	case ChatIdle:
		return ""
	case ChatThinking:
		return "Thinking..."
	case ChatStreaming:
		return "Responding..."
	case ChatError:
		return "Error"
	default:
		return ""
	}
}

// ChatHandler handles user messages and returns a stream of events.
// Implementations should start streaming immediately and close the channel when done.
type ChatHandler interface {
	// HandleMessage sends a user message and returns a channel of streaming events.
	// The returned channel will be closed when the response is complete.
	// Cancel the context to abort the stream.
	HandleMessage(ctx context.Context, userMsg string) (<-chan StreamMsg, error)
}
