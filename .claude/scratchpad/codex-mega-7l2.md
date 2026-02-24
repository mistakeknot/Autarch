## Goal
Create stub ChatHandler implementations for Bigend, Coldwine, and Pollard dashboard views. Each follows the same pattern as GurgehChatHandler but with simpler context (no spec loading needed yet). Wire each handler into its view.

## Phase 1: Explore
Before making any changes, investigate:
- Read `internal/tui/views/gurgeh_chat_handler.go` — this is the template to follow
- Read `internal/tui/views/bigend.go` — how it creates chatPanel, what data is available
- Read `internal/tui/views/coldwine.go` — same
- Read `internal/tui/views/pollard.go` — same
- Read `pkg/tui/claude_handler.go` — the generic ClaudeChatHandler for reference
- Print a brief exploration summary before proceeding

## Phase 2: Implement

### Create `internal/tui/views/bigend_chat_handler.go`

Minimal handler for Bigend (project overview/mission control):

```go
package views

import (
    "context"
    "fmt"
    "os"

    "github.com/mistakeknot/autarch/pkg/claude"
    pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

type BigendChatHandler struct {
    cwd string
}

func NewBigendChatHandler() *BigendChatHandler {
    cwd, _ := os.Getwd()
    return &BigendChatHandler{cwd: cwd}
}

func (h *BigendChatHandler) HandleMessage(ctx context.Context, userMsg string) (<-chan pkgtui.StreamMsg, error) {
    systemCtx := "You are Bigend, an AI project management assistant. Help the user understand project status, manage tasks, and coordinate multi-agent work."

    args := []string{"--system-prompt", systemCtx, "-p", userMsg}
    events, err := claude.RunStreaming(ctx, h.cwd, args)
    if err != nil {
        return nil, err
    }

    // Same conversion pattern as GurgehChatHandler
    out := make(chan pkgtui.StreamMsg, 64)
    go func() {
        defer close(out)
        for event := range events {
            var msg pkgtui.StreamMsg
            switch event.Type {
            case claude.EventText:
                msg = pkgtui.TextDelta{Text: event.Text}
            case claude.EventThinkingStart:
                msg = pkgtui.ReasoningStart{}
            case claude.EventThinking:
                msg = pkgtui.ReasoningDelta{Text: event.Text}
            case claude.EventThinkingEnd:
                msg = pkgtui.ReasoningEnd{}
            case claude.EventToolUse:
                msg = pkgtui.ToolCallStart{Name: event.ToolName}
            case claude.EventResult:
                if event.IsError {
                    msg = pkgtui.StreamError{Err: fmt.Errorf("%s", event.Text)}
                } else {
                    msg = pkgtui.StreamDone{FinishReason: "stop"}
                }
            case claude.EventError:
                msg = pkgtui.StreamError{Err: fmt.Errorf("%s", event.Text)}
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

### Create `internal/tui/views/coldwine_chat_handler.go`

Same pattern, Coldwine-specific system prompt:
```go
systemCtx := "You are Coldwine, an AI task orchestration assistant. Help the user plan tasks, track progress, manage dependencies, and coordinate human-AI collaboration."
```

### Create `internal/tui/views/pollard_chat_handler.go`

Same pattern, Pollard-specific system prompt:
```go
systemCtx := "You are Pollard, a general-purpose research intelligence assistant. Help the user research topics, analyze competitive landscapes, find academic papers, and synthesize findings across domains."
```

### Wire handlers into views

For each view (bigend.go, coldwine.go, pollard.go):

1. Add `chatHandler` field to the view struct
2. In `NewXxxView()`, create the handler and set it:
```go
v.chatHandler = NewXxxChatHandler()
v.chatPanel.SetHandler(v.chatHandler)
```

That's the only change needed to each view file — the non-key message forwarding and CancelStream are already wired from bead 0v9.

## Phase 3: Verify
After implementing, run ALL of these and report results:
1. Build: `GOCACHE=/tmp/go-build-cache go build ./internal/tui/views/...`
2. Build all: `GOCACHE=/tmp/go-build-cache go build ./cmd/...`
3. Tests: `GOCACHE=/tmp/go-build-cache go test ./pkg/tui/... -v -short -count=1`
4. Diff: `git diff --stat`
5. If build or tests fail: fix the issue and re-verify (up to 2 self-retries)

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
- Create 3 new files: `bigend_chat_handler.go`, `coldwine_chat_handler.go`, `pollard_chat_handler.go` in `internal/tui/views/`
- Modify `bigend.go`, `coldwine.go`, `pollard.go` — only add handler field + wire in constructor
- Do NOT modify gurgeh.go, chatpanel.go, or any pkg/tui/ files
- Keep all handlers minimal — enrich context later
- Do NOT commit or push any changes
- If GOCACHE permission errors occur, use: GOCACHE=/tmp/go-build-cache
- Module path is `github.com/mistakeknot/autarch`
