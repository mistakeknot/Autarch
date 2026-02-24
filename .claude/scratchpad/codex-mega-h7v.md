## Goal
Extract the `runClaude()` function from `internal/gurgeh/exploration/explore.go` into a shared package `pkg/claude/run.go` with a streaming-aware API that returns events on a channel.

## Phase 1: Explore
Before making any changes, investigate:
- Read `internal/gurgeh/exploration/explore.go` — focus on `runClaude()` (around line 576), `streamMessage`, `streamContent`, `contentBlock` types (lines 631-647), and `logToolUse()` (line 651)
- Read `pkg/tui/chatpanel.go` — understand the `ChatMessage` type (line 26) to know what format downstream consumers need
- Check `go.mod` for the module path: `github.com/mistakeknot/autarch`
- Search for any other callers of `runClaude` in the codebase: `grep -r "runClaude" --include="*.go"`
- Check if `pkg/claude/` directory already exists
- Print a brief exploration summary before proceeding

## Phase 2: Implement

### Create `pkg/claude/run.go`

Create a new file `pkg/claude/run.go` with:

1. **Package declaration**: `package claude`

2. **StreamEvent type** — a discriminated union for streaming events:
```go
// EventType represents the kind of streaming event from Claude CLI.
type EventType int

const (
    EventText EventType = iota      // Assistant text content delta
    EventThinkingStart              // Extended thinking started
    EventThinking                   // Extended thinking content delta
    EventThinkingEnd                // Extended thinking ended
    EventToolUse                    // Tool invocation (name + input)
    EventResult                     // Final result text
    EventError                      // Error occurred
    EventSessionID                  // Session ID extracted from init message
)

// StreamEvent is a single event from a Claude CLI streaming session.
type StreamEvent struct {
    Type      EventType
    Text      string            // Content for text/thinking/result events
    ToolName  string            // For EventToolUse
    ToolInput map[string]any    // For EventToolUse
    SessionID string            // For EventSessionID
    IsError   bool              // For EventResult — whether Claude flagged it as error
}
```

3. **RunStreaming function**:
```go
// RunStreaming executes `claude` with --output-format stream-json and sends
// parsed events to the returned channel. The channel is closed when the
// process exits. Cancel the context to kill the process.
func RunStreaming(ctx context.Context, cwd string, args []string) (<-chan StreamEvent, error)
```

Implementation:
- Prepend `--output-format stream-json` to args if not already present (check for it first)
- Use `exec.CommandContext(ctx, "claude", args...)`
- Set `cmd.Dir = cwd`
- Create stdout pipe, start the process
- Launch a goroutine that:
  - Creates a `bufio.Scanner` with 1MB buffer (same as existing)
  - For each JSONL line, parse into a raw struct
  - Emit `EventSessionID` when type is "system" and subtype is "init" and session_id is present
  - Emit `EventText` when type is "assistant" and content blocks contain type "text"
  - Emit `EventThinkingStart`/`EventThinking`/`EventThinkingEnd` for reasoning content
  - Emit `EventToolUse` when content blocks contain type "tool_use"
  - Emit `EventResult` when type is "result"
  - Emit `EventError` on parse failures or process errors
  - Close the channel after `cmd.Wait()` completes
  - If `cmd.Wait()` fails and no EventResult was sent, emit an EventError

4. **Run function** (backward-compatible wrapper):
```go
// Run executes Claude and returns the final result text.
// This is a convenience wrapper around RunStreaming that blocks until complete.
func Run(ctx context.Context, cwd string, args []string) (string, error)
```

Implementation:
- Call `RunStreaming()`, drain the channel collecting only EventResult/EventError
- Return the result text or error, matching the existing `runClaude()` behavior exactly

5. **Raw JSONL types** (internal, lowercase):
```go
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
```

### Update `internal/gurgeh/exploration/explore.go`

- Change `runClaude()` to call `claude.Run()` from the new package
- Remove the local `streamMessage`, `streamContent`, `contentBlock` types (lines 631-647)
- Keep `logToolUse()` in explore.go (it's exploration-specific logging)
- The new `runClaude()` should be a thin wrapper:
```go
func runClaude(ctx context.Context, cwd string, args []string) (string, error) {
    return claude.Run(ctx, cwd, args)
}
```
- OR, if `runClaude` is only called in this file, just inline `claude.Run(ctx, cwd, args)` at the call sites and delete `runClaude` entirely.

**IMPORTANT**: The existing `logToolUse()` function currently logs tool usage during streaming. Since `claude.Run()` is a blocking call that doesn't expose individual events, the tool logging behavior changes. That's acceptable for now — tool logging during streaming will be added back when views use `RunStreaming()` directly.

## Phase 3: Verify
After implementing, run ALL of these and report results:
1. Build: `GOCACHE=/tmp/go-build-cache go build ./cmd/...`
2. Build the specific package: `GOCACHE=/tmp/go-build-cache go build ./pkg/claude/...`
3. Vet: `GOCACHE=/tmp/go-build-cache go vet ./pkg/claude/... ./internal/gurgeh/exploration/...`
4. Tests: `GOCACHE=/tmp/go-build-cache go test ./pkg/claude/... -v -short -count=1`
5. Tests for exploration: `GOCACHE=/tmp/go-build-cache go test ./internal/gurgeh/exploration/... -v -short -count=1 -run "Test" -timeout 30s`
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
- Only modify `pkg/claude/run.go` (new) and `internal/gurgeh/exploration/explore.go`
- Do not reformat, realign, or adjust whitespace in code you didn't functionally change
- Do not add comments, docstrings, or type annotations to unchanged code
- Do not refactor or rename anything not directly related to the task
- Keep the change minimal
- Do NOT commit or push any changes
- If GOCACHE permission errors occur, use: GOCACHE=/tmp/go-build-cache
- Module path is `github.com/mistakeknot/autarch`
