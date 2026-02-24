## Goal
Implement GurgehChatHandler — a ChatHandler that sends user messages to Claude CLI with the current spec context prepended as a system-level instruction. Then wire it into GurgehView so typing in the spec browser actually gets an agent response.

## Phase 1: Explore
Before making any changes, investigate:
- Read `pkg/tui/claude_handler.go` — see the existing ClaudeChatHandler pattern
- Read `pkg/tui/chatstream.go` — ChatHandler interface
- Read `pkg/claude/run.go` — RunStreaming(), StreamEvent types
- Read `internal/tui/views/gurgeh.go` — understand the spec browser mode, how specs are loaded, what data is available. Look for: current spec ID, spec store, any method that loads spec data
- Read `internal/gurgeh/specs/spec.go` (or find the Spec type) — understand what fields are available
- Read `internal/gurgeh/specs/store.go` (or equivalent) — understand how to load a spec by ID
- Print a brief exploration summary before proceeding

## Phase 2: Implement

### Create `internal/tui/views/gurgeh_chat_handler.go`

This file lives in the views package (not pkg/tui) because it needs access to the spec store:

```go
package views

import (
    "context"
    "fmt"
    "os"
    "strings"

    "github.com/mistakeknot/autarch/pkg/claude"
    pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// GurgehChatHandler implements ChatHandler with spec-aware context.
type GurgehChatHandler struct {
    specStore   // whatever the spec storage interface is
    currentSpec string // current spec ID
    cwd         string
}

// NewGurgehChatHandler creates a handler for the Gurgeh spec browser.
func NewGurgehChatHandler() *GurgehChatHandler {
    cwd, _ := os.Getwd()
    return &GurgehChatHandler{cwd: cwd}
}

// SetCurrentSpec updates the spec ID for context enrichment.
func (h *GurgehChatHandler) SetCurrentSpec(specID string) {
    h.currentSpec = specID
}

// SetSpecStore sets the spec data source.
// The exact type depends on what's available — look at how GurgehView loads specs.
func (h *GurgehChatHandler) SetSpecStore(store /* type */) {
    h.specStore = store
}

// HandleMessage sends the user's message to Claude with spec context.
func (h *GurgehChatHandler) HandleMessage(ctx context.Context, userMsg string) (<-chan pkgtui.StreamMsg, error) {
    // Build system context
    systemCtx := h.buildSystemContext()

    args := []string{}
    if systemCtx != "" {
        args = append(args, "--system-prompt", systemCtx)
    }
    args = append(args, "-p", userMsg)

    events, err := claude.RunStreaming(ctx, h.cwd, args)
    if err != nil {
        return nil, err
    }

    // Convert claude.StreamEvent → pkgtui.StreamMsg (same pattern as ClaudeChatHandler)
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

func (h *GurgehChatHandler) buildSystemContext() string {
    // If no spec selected, return generic context
    if h.currentSpec == "" {
        return "You are a spec assistant for the Gurgeh PRD tool. Help the user create and refine product specifications."
    }

    // Try to load spec data
    // Look at how GurgehView loads specs to determine the right API
    // For now, use whatever spec loading mechanism is available

    var b strings.Builder
    b.WriteString("You are a spec assistant for the Gurgeh PRD tool.\n")
    b.WriteString(fmt.Sprintf("The user is viewing spec: %s\n", h.currentSpec))
    b.WriteString("Answer questions about this spec. Suggest improvements. Help refine acceptance criteria.\n")
    return b.String()
}
```

**IMPORTANT**: The exact spec loading API depends on what you find in the codebase. Look at:
- How GurgehView stores/accesses the spec list
- What type the spec store uses (is it `specs.Store`? A filesystem path?)
- What fields are available on specs (Title, Status, Summary, etc.)

Adapt the handler to use whatever API exists. If specs have YAML content accessible as a string, include key sections in the system context. If only basic metadata is available, use that.

### Wire handler into GurgehView

In `internal/tui/views/gurgeh.go`:

1. Add a `chatHandler *GurgehChatHandler` field to GurgehView struct
2. In `NewGurgehView()`, create the handler and set it on the chatPanel:
```go
v.chatHandler = NewGurgehChatHandler()
v.chatPanel.SetHandler(v.chatHandler)
```
3. When the spec selection changes (look for where the current spec ID is set/updated in the browser), call:
```go
v.chatHandler.SetCurrentSpec(specID)
```

## Phase 3: Verify
After implementing, run ALL of these and report results:
1. Build: `GOCACHE=/tmp/go-build-cache go build ./internal/tui/views/...`
2. Build all: `GOCACHE=/tmp/go-build-cache go build ./cmd/...`
3. Vet: `GOCACHE=/tmp/go-build-cache go vet ./internal/tui/views/...`
4. Tests: `GOCACHE=/tmp/go-build-cache go test ./pkg/tui/... -v -short -count=1`
5. Diff: `git diff --stat`
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
- Create `internal/tui/views/gurgeh_chat_handler.go` (new file)
- Modify `internal/tui/views/gurgeh.go` (wire handler)
- Do NOT modify pkg/tui/ files
- Do not reformat, realign, or adjust whitespace in code you didn't functionally change
- Keep the change minimal — basic context enrichment is fine, we'll enhance later
- Do NOT commit or push any changes
- If GOCACHE permission errors occur, use: GOCACHE=/tmp/go-build-cache
- Module path is `github.com/mistakeknot/autarch`
