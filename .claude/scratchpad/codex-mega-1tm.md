## Goal
Extend ChatPanel with streaming state management. When a ChatHandler is set and the user submits non-slash text, ChatPanel should start streaming, accumulate response chunks into a growing agent message, update the spinner status, and transition back to idle when done. This is the core streaming integration.

## Phase 1: Explore
Before making any changes, investigate:
- Read `pkg/tui/chatpanel.go` in full — understand the current ChatPanel struct, methods, and flow
- Read `pkg/tui/chatstream.go` — understand StreamMsg types, ChatState, ChatHandler interface
- Read `pkg/tui/composer.go` — understand Composer API (Reset, Value, SetValue)
- Check how Bubble Tea handles context cancellation and goroutine patterns (tea.Cmd with channel reads)
- Print a brief exploration summary before proceeding

## Phase 2: Implement

### Modify `pkg/tui/chatpanel.go`

The goal is to add streaming state management without changing the existing rendering. The spinner and markdown rendering already work from prior beads — this bead wires the event loop.

#### 1. Add new fields to ChatPanel struct:
```go
type ChatPanel struct {
    // ... existing fields ...
    handler     ChatHandler       // Optional: handles non-slash input
    chatState   ChatState         // Current streaming state
    streamCtx   context.Context   // Active stream context
    cancelStream context.CancelFunc // Cancel active stream
}
```

Add `"context"` to imports.

#### 2. Add SetHandler method:
```go
// SetHandler sets the ChatHandler for processing non-slash input.
func (p *ChatPanel) SetHandler(handler ChatHandler) {
    p.handler = handler
}
```

#### 3. Define a tea.Msg for stream events:
```go
// StreamChunkMsg wraps a StreamMsg for delivery via Bubble Tea's message system.
type StreamChunkMsg struct {
    Event StreamMsg
}
```

#### 4. Modify SubmitInput() to handle non-slash text:
Currently SubmitInput() returns nil for non-slash text. Change it to:
```go
func (p *ChatPanel) SubmitInput() tea.Cmd {
    value := strings.TrimSpace(p.Value())
    if value == "" {
        return nil
    }

    if cmd, args, isSlash := ParseSlashCommand(value); isSlash {
        p.ClearComposer()
        return func() tea.Msg {
            return SlashCommandMsg{Command: cmd, Args: args}
        }
    }

    // Non-slash text: send to handler if available
    if p.handler == nil {
        return nil
    }

    p.ClearComposer()
    p.AddMessage("user", value)

    // Start streaming
    ctx, cancel := context.WithCancel(context.Background())
    p.streamCtx = ctx
    p.cancelStream = cancel
    p.chatState = ChatThinking
    p.SetStatus(ChatThinking.String())

    // Start a new "agent" message that will be appended to
    p.messages = append(p.messages, ChatMessage{Role: "agent", Content: ""})

    handler := p.handler
    userMsg := value

    return tea.Batch(
        p.SpinnerTick(),
        func() tea.Msg {
            events, err := handler.HandleMessage(ctx, userMsg)
            if err != nil {
                return StreamChunkMsg{Event: StreamError{Err: err}}
            }
            // Read first event and send it
            event, ok := <-events
            if !ok {
                return StreamChunkMsg{Event: StreamDone{FinishReason: "stop"}}
            }
            return StreamChunkMsg{Event: event}
        },
    )
}
```

Wait — Bubble Tea's model is single-threaded. We can't read from a channel inside a tea.Cmd and send multiple messages. The standard pattern is: read one event from the channel, send it as a tea.Msg, then in Update() issue another tea.Cmd to read the next event.

#### Better approach — use a waitForStreamEvent pattern:

```go
// waitForStreamEvent returns a tea.Cmd that reads the next event from the stream channel.
func waitForStreamEvent(events <-chan StreamMsg) tea.Cmd {
    return func() tea.Msg {
        event, ok := <-events
        if !ok {
            return StreamChunkMsg{Event: StreamDone{FinishReason: "stop"}}
        }
        return StreamChunkMsg{Event: event}
    }
}
```

Then SubmitInput becomes:
```go
func (p *ChatPanel) SubmitInput() tea.Cmd {
    value := strings.TrimSpace(p.Value())
    if value == "" {
        return nil
    }

    if cmd, args, isSlash := ParseSlashCommand(value); isSlash {
        p.ClearComposer()
        return func() tea.Msg {
            return SlashCommandMsg{Command: cmd, Args: args}
        }
    }

    // Non-slash text: send to handler if available
    if p.handler == nil {
        return nil
    }

    p.ClearComposer()
    p.AddMessage("user", value)

    // Start streaming
    ctx, cancel := context.WithCancel(context.Background())
    p.streamCtx = ctx
    p.cancelStream = cancel
    p.chatState = ChatThinking
    p.SetStatus(ChatThinking.String())

    // Add empty agent message as streaming target
    p.messages = append(p.messages, ChatMessage{Role: "agent", Content: ""})

    handler := p.handler
    userMsg := value

    // Start the stream in a goroutine, send first event via tea.Cmd
    return func() tea.Msg {
        events, err := handler.HandleMessage(ctx, userMsg)
        if err != nil {
            return StreamChunkMsg{Event: StreamError{Err: err}}
        }
        // Store events channel for subsequent reads
        // We need to return this somehow...
        // Actually, we need to store the channel on the struct.
        return streamStartMsg{events: events}
    }
}
```

Hmm, this is getting complex. Let me simplify: store the events channel on the ChatPanel struct.

#### Final approach — store events channel:

Add to struct:
```go
    events      <-chan StreamMsg   // Active stream events channel
```

SubmitInput:
```go
func (p *ChatPanel) SubmitInput() tea.Cmd {
    value := strings.TrimSpace(p.Value())
    if value == "" {
        return nil
    }

    if cmd, args, isSlash := ParseSlashCommand(value); isSlash {
        p.ClearComposer()
        return func() tea.Msg {
            return SlashCommandMsg{Command: cmd, Args: args}
        }
    }

    if p.handler == nil {
        return nil
    }

    p.ClearComposer()
    p.AddMessage("user", value)

    ctx, cancel := context.WithCancel(context.Background())
    p.streamCtx = ctx
    p.cancelStream = cancel
    p.chatState = ChatThinking
    p.SetStatus(ChatThinking.String())

    // Add empty agent message as streaming target
    p.messages = append(p.messages, ChatMessage{Role: "agent", Content: ""})

    handler := p.handler
    userMsg := value

    return tea.Batch(
        p.SpinnerTick(),
        func() tea.Msg {
            events, err := handler.HandleMessage(ctx, userMsg)
            if err != nil {
                return StreamChunkMsg{Event: StreamError{Err: err}}
            }
            return streamStartedMsg{events: events}
        },
    )
}
```

Define internal message types:
```go
// streamStartedMsg is sent when the handler returns its event channel.
type streamStartedMsg struct {
    events <-chan StreamMsg
}
```

In Update(), handle the new messages:
```go
// Handle stream started — store channel and start reading
case streamStartedMsg:
    p.events = msg.events
    return p, waitForStreamEvent(p.events)

// Handle stream chunks
case StreamChunkMsg:
    return p.handleStreamChunk(msg.Event)
```

#### 5. Add handleStreamChunk method:
```go
func (p *ChatPanel) handleStreamChunk(event StreamMsg) (*ChatPanel, tea.Cmd) {
    switch e := event.(type) {
    case TextDelta:
        // Append to last message
        if len(p.messages) > 0 {
            last := &p.messages[len(p.messages)-1]
            last.Content += e.Text
        }
        if p.chatState != ChatStreaming {
            p.chatState = ChatStreaming
            p.SetStatus(ChatStreaming.String())
        }
        // Auto-scroll
        p.scroll = 0
        return p, waitForStreamEvent(p.events)

    case ReasoningStart:
        p.chatState = ChatThinking
        p.SetStatus(ChatThinking.String())
        return p, tea.Batch(p.SpinnerTick(), waitForStreamEvent(p.events))

    case ReasoningDelta:
        // Could display thinking content later; for now just continue
        return p, waitForStreamEvent(p.events)

    case ReasoningEnd:
        return p, waitForStreamEvent(p.events)

    case ToolCallStart:
        // Could show tool usage indicator; for now continue
        return p, waitForStreamEvent(p.events)

    case ToolCallInput:
        return p, waitForStreamEvent(p.events)

    case ToolCallResult:
        return p, waitForStreamEvent(p.events)

    case StreamError:
        p.chatState = ChatError
        p.SetStatus("")
        if len(p.messages) > 0 {
            last := &p.messages[len(p.messages)-1]
            if last.Content == "" {
                last.Content = "Error: " + e.Err.Error()
            }
        }
        p.cleanupStream()
        return p, nil

    case StreamDone:
        p.chatState = ChatIdle
        p.SetStatus("")
        // Remove empty agent message if no content was streamed
        if len(p.messages) > 0 {
            last := p.messages[len(p.messages)-1]
            if last.Role == "agent" && strings.TrimSpace(last.Content) == "" {
                p.messages = p.messages[:len(p.messages)-1]
            }
        }
        p.cleanupStream()
        return p, nil

    default:
        return p, waitForStreamEvent(p.events)
    }
}
```

#### 6. Add cleanupStream helper:
```go
func (p *ChatPanel) cleanupStream() {
    if p.cancelStream != nil {
        p.cancelStream()
    }
    p.streamCtx = nil
    p.cancelStream = nil
    p.events = nil
}
```

#### 7. Add CancelStream method (for views to call on tab switch):
```go
// CancelStream cancels any active streaming and returns to idle state.
func (p *ChatPanel) CancelStream() {
    if p.chatState == ChatIdle {
        return
    }
    p.chatState = ChatIdle
    p.SetStatus("")
    p.cleanupStream()
}
```

#### 8. Update the Update() method to handle new message types:

Add these cases in the Update() method, before the keyMsg check:
```go
// Handle stream events
switch msg := msg.(type) {
case streamStartedMsg:
    p.events = msg.events
    return p, waitForStreamEvent(p.events)
case StreamChunkMsg:
    return p.handleStreamChunk(msg.Event)
}
```

Put this AFTER the spinner.TickMsg handler but BEFORE the keyMsg handler.

**IMPORTANT**: The switch on `msg` here is separate from the existing `msg.(type)` checks. Be careful not to shadow the parameter name. The outer function parameter is named `msg tea.Msg`. Use a type switch that doesn't rebind `msg`:

```go
switch typedMsg := msg.(type) {
case streamStartedMsg:
    p.events = typedMsg.events
    return p, waitForStreamEvent(p.events)
case StreamChunkMsg:
    return p.handleStreamChunk(typedMsg.Event)
}
```

## Phase 3: Verify
After implementing, run ALL of these and report results:
1. Build: `GOCACHE=/tmp/go-build-cache go build ./pkg/tui/...`
2. Build all: `GOCACHE=/tmp/go-build-cache go build ./cmd/...`
3. Vet: `GOCACHE=/tmp/go-build-cache go vet ./pkg/tui/...`
4. Tests: `GOCACHE=/tmp/go-build-cache go test ./pkg/tui/... -v -short -count=1`
5. Diff: `git diff --stat` (ensure only pkg/tui/chatpanel.go changed among non-existing-changes)
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
- Only modify `pkg/tui/chatpanel.go`
- Do NOT modify chatstream.go, composer.go, or any other file
- Do not reformat, realign, or adjust whitespace in code you didn't functionally change
- Do not add comments, docstrings, or type annotations to unchanged code
- Do not refactor or rename anything not directly related to the task
- Keep the change minimal
- Do NOT commit or push any changes
- If GOCACHE permission errors occur, use: GOCACHE=/tmp/go-build-cache
- Module path is `github.com/mistakeknot/autarch`
