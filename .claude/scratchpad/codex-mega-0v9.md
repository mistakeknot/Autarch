## Goal
Wire ChatHandler into dashboard views by:
1. Creating a `ClaudeChatHandler` in `pkg/tui/claude_handler.go` that adapts `pkg/claude.RunStreaming()` to the `ChatHandler` interface
2. Updating dashboard views to forward non-key messages to ChatPanel.Update() and call CancelStream() on Blur

## Phase 1: Explore
Before making any changes, investigate:
- Read `pkg/tui/chatpanel.go` — focus on: SetHandler(), SubmitInput() (the new streaming path), Update() (handles StreamChunkMsg and streamStartedMsg), CancelStream()
- Read `pkg/tui/chatstream.go` — ChatHandler interface, StreamMsg types
- Read `pkg/claude/run.go` — RunStreaming(), StreamEvent types
- Read `internal/tui/views/gurgeh.go` — how FocusChat case calls chatPanel.Update()
- Read `internal/tui/views/coldwine.go` — same pattern
- Read `internal/tui/views/pollard.go` — same pattern
- Read `internal/tui/views/bigend.go` — same pattern
- Check if views already forward non-key messages to chatPanel.Update(). They should be calling `v.chatPanel.Update(msg)` in their general Update() path.
- Print a brief exploration summary before proceeding

## Phase 2: Implement

### Create `pkg/tui/claude_handler.go`

This file bridges `pkg/claude.RunStreaming()` → `ChatHandler` interface:

```go
package tui

import (
    "context"
    "os"

    "github.com/mistakeknot/autarch/pkg/claude"
)

// ClaudeChatHandler implements ChatHandler by running Claude CLI.
type ClaudeChatHandler struct {
    // CWD is the working directory for Claude. Defaults to os.Getwd() if empty.
    CWD string
    // ExtraArgs are additional CLI arguments passed to Claude.
    ExtraArgs []string
}

// HandleMessage sends the user's message to Claude CLI and converts stream events.
func (h *ClaudeChatHandler) HandleMessage(ctx context.Context, userMsg string) (<-chan StreamMsg, error) {
    cwd := h.CWD
    if cwd == "" {
        var err error
        cwd, err = os.Getwd()
        if err != nil {
            return nil, err
        }
    }

    args := make([]string, 0, len(h.ExtraArgs)+2)
    args = append(args, h.ExtraArgs...)
    args = append(args, "-p", userMsg)

    events, err := claude.RunStreaming(ctx, cwd, args)
    if err != nil {
        return nil, err
    }

    out := make(chan StreamMsg, 64)
    go func() {
        defer close(out)
        for event := range events {
            var msg StreamMsg
            switch event.Type {
            case claude.EventText:
                msg = TextDelta{Text: event.Text}
            case claude.EventThinkingStart:
                msg = ReasoningStart{}
            case claude.EventThinking:
                msg = ReasoningDelta{Text: event.Text}
            case claude.EventThinkingEnd:
                msg = ReasoningEnd{}
            case claude.EventToolUse:
                msg = ToolCallStart{Name: event.ToolName}
            case claude.EventResult:
                if event.IsError {
                    msg = StreamError{Err: fmt.Errorf("%s", event.Text)}
                } else {
                    msg = StreamDone{FinishReason: "stop"}
                }
            case claude.EventError:
                msg = StreamError{Err: fmt.Errorf("%s", event.Text)}
            default:
                continue
            }
            select {
            case out <- msg:
            case <-ctx.Done():
                return
            }
        }
    }()

    return out, nil
}
```

Add `"fmt"` to the imports.

### Update dashboard views

For each view (gurgeh.go, coldwine.go, pollard.go, bigend.go), make these changes:

#### In each view's Blur() method, add CancelStream:
```go
func (v *XxxView) Blur() {
    v.chatPanel.CancelStream()
    v.chatPanel.Blur()
}
```

#### Ensure Update() forwards non-key messages to chatPanel:
Check if there's a path where `msg` (including StreamChunkMsg) reaches `chatPanel.Update()`. In most views, the FocusChat case only handles tea.KeyMsg. We need to ensure StreamChunkMsg also reaches chatPanel.

Add this near the top of Update(), before focus-specific handling:
```go
// Forward stream events to chat panel regardless of focus
switch msg.(type) {
case StreamChunkMsg, spinner.TickMsg:
    if v.chatPanel != nil {
        var cmd tea.Cmd
        v.chatPanel, cmd = v.chatPanel.Update(msg)
        return v, cmd
    }
}
```

Wait — the views receive `tea.Msg` through the unified_app dispatcher. The `streamStartedMsg` and `StreamChunkMsg` types are defined in pkg/tui (same package as ChatPanel). The views are in `internal/tui/views/` which imports `pkg/tui`. So the type assertions should work.

**Actually**, `streamStartedMsg` is unexported (lowercase). It will only be accessible within `pkg/tui`. The views can't match on it directly. But they don't need to — they just forward ALL non-key messages to chatPanel.Update() and let ChatPanel handle the type switching internally.

The simplest approach: at the top of each view's Update(), for any non-key message, forward to chatPanel:
```go
// Forward non-key messages to chat panel (handles streaming, spinner)
if _, isKey := msg.(tea.KeyMsg); !isKey {
    if v.chatPanel != nil {
        var cmd tea.Cmd
        v.chatPanel, cmd = v.chatPanel.Update(msg)
        if cmd != nil {
            return v, cmd
        }
    }
}
```

But wait — this would also intercept WindowSizeMsg and other messages the view needs. Better: only forward if ChatPanel is streaming, or if it's a spinner tick:

Actually the simplest correct pattern: forward ALL messages to chatPanel unconditionally, then also handle view-specific ones. ChatPanel's Update() already returns early for messages it doesn't handle (falls through to composer):

```go
// Always let chat panel process messages (streaming events, spinner ticks)
if v.chatPanel != nil {
    var chatCmd tea.Cmd
    v.chatPanel, chatCmd = v.chatPanel.Update(msg)
    // Continue processing — view may also need to handle this msg
    // But if chatPanel returned a command, batch it
    if chatCmd != nil {
        // Handle remaining view logic and batch commands
    }
}
```

Hmm, this gets messy. Let me take a different approach: have each view forward the message to chatPanel in its general `default` case, and also forward specific message types (spinner ticks) early.

**Simplest approach for now**: In each view's Update(), add a block right after WindowSizeMsg handling that checks for streaming-related messages:

```go
// Forward streaming events to chat panel
if v.chatPanel != nil && v.chatPanel.IsStreaming() {
    if _, isKey := msg.(tea.KeyMsg); !isKey {
        if _, isWinSize := msg.(tea.WindowSizeMsg); !isWinSize {
            v.chatPanel, cmd = v.chatPanel.Update(msg)
            return v, cmd
        }
    }
}
```

Wait, this is still fragile. Let me look at how GurgehOnboardingView handles this pattern — it's the existing view with ChatPanel already working in the onboarding flow.

**IMPORTANT**: Read `internal/tui/views/gurgeh_onboarding.go` to see how it forwards messages to chatPanel.

Then follow the exact same pattern for all 4 dashboard views.

## Phase 3: Verify
After implementing, run ALL of these and report results:
1. Build: `GOCACHE=/tmp/go-build-cache go build ./pkg/tui/...`
2. Build: `GOCACHE=/tmp/go-build-cache go build ./internal/tui/views/...`
3. Build all: `GOCACHE=/tmp/go-build-cache go build ./cmd/...`
4. Vet: `GOCACHE=/tmp/go-build-cache go vet ./pkg/tui/... ./internal/tui/views/...`
5. Tests: `GOCACHE=/tmp/go-build-cache go test ./pkg/tui/... -v -short -count=1`
6. Diff: `git diff --stat` (ensure only expected files changed)
7. If build or tests fail: fix the issue and re-verify (up to 2 self-retries)

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
- Create `pkg/tui/claude_handler.go` (new file)
- Modify `internal/tui/views/gurgeh.go`, `coldwine.go`, `pollard.go`, `bigend.go` — only the Blur() method and streaming message forwarding
- Do NOT modify chatpanel.go, chatstream.go, composer.go, or unified_app.go
- Do not reformat, realign, or adjust whitespace in code you didn't functionally change
- Do not add comments, docstrings, or type annotations to unchanged code
- Do not refactor or rename anything not directly related to the task
- Keep the change minimal
- Do NOT commit or push any changes
- If GOCACHE permission errors occur, use: GOCACHE=/tmp/go-build-cache
- Module path is `github.com/mistakeknot/autarch`
