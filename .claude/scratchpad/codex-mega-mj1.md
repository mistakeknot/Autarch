## Goal
Create `pkg/tui/chatstream.go` defining StreamMsg types and the ChatHandler interface for streaming chat responses. This is the TUI-layer abstraction that views use to send messages and receive streaming events.

## Phase 1: Explore
Before making any changes, investigate:
- Read `pkg/claude/run.go` — understand the existing `StreamEvent` types from the Claude CLI runner
- Read `pkg/tui/chatpanel.go` — understand the `ChatPanel` struct, `ChatMessage` type, and how `SetStatus`/`IsStreaming`/`SpinnerTick` work
- Read `pkg/tui/colors.go` for any relevant constants
- Check if `pkg/tui/chatstream.go` already exists
- Print a brief exploration summary before proceeding

## Phase 2: Implement

### Create `pkg/tui/chatstream.go`

This file defines the TUI-layer streaming types. These are intentionally separate from `pkg/claude.StreamEvent` (the wire format) — the TUI package should not depend on the subprocess runner.

```go
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
}
func (StreamDone) streamMsg() {}

// ChatState represents the current state of a chat interaction.
type ChatState int

const (
    ChatIdle      ChatState = iota // No active streaming
    ChatThinking                    // Agent is thinking (extended thinking)
    ChatStreaming                   // Agent is producing text output
    ChatError                       // An error occurred
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
```

**IMPORTANT design notes**:
- The `streamMsg()` marker method is unexported, creating a closed type set (only types in this package can implement StreamMsg). This is the Go equivalent of a sealed/discriminated union.
- `ChatState` uses `String()` to provide human-readable labels that can be passed directly to `ChatPanel.SetStatus()`.
- `ChatHandler` returns a channel, not a callback — this matches the `pkg/claude.RunStreaming()` pattern and works naturally with Go's concurrency model.

### That's it — this bead is just the type definitions

Do NOT modify chatpanel.go, composer.go, or any other file. This bead creates a single new file with type definitions and an interface.

## Phase 3: Verify
After implementing, run ALL of these and report results:
1. Build: `GOCACHE=/tmp/go-build-cache go build ./pkg/tui/...`
2. Build all: `GOCACHE=/tmp/go-build-cache go build ./cmd/...`
3. Vet: `GOCACHE=/tmp/go-build-cache go vet ./pkg/tui/...`
4. Tests: `GOCACHE=/tmp/go-build-cache go test ./pkg/tui/... -v -short -count=1`
5. Diff: `git diff --stat` (ensure only expected new file)
6. If build or tests fail: fix the issue and re-verify (up to 2 self-retries)

## Final Report
At the end, print a structured verdict:
```
EXPLORATION: [1-2 sentence summary of what you found]
CHANGES: [list files modified/created with brief description]
BUILD: PASS | FAIL
TESTS: PASS | FAIL [N passed, M failed]
VERDICT: CLEAN | NEEDS_ATTENTION [reason]
```

## Constraints
- Only create `pkg/tui/chatstream.go` (new file)
- Do NOT modify any existing files
- Do not add tests in this bead (types are too simple to test independently)
- Do NOT commit or push any changes
- If GOCACHE permission errors occur, use: GOCACHE=/tmp/go-build-cache
- Module path is `github.com/mistakeknot/autarch`
