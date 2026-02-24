## Goal
Add multi-turn conversation support to all ChatHandlers using Claude CLI's `-c` (continue) flag. After the first message in a tab, subsequent messages continue the same conversation instead of starting a new one. Also add a `/new` global command to reset the conversation.

## Phase 1: Explore
Before making any changes, investigate:
- Read `pkg/claude/run.go` — understand RunStreaming, EventSessionID, the rawMessage struct
- Read `pkg/tui/chatstream.go` — see StreamDone, the sealed StreamMsg interface
- Read `pkg/tui/claude_handler.go` — the generic ClaudeChatHandler
- Read `internal/tui/views/gurgeh_chat_handler.go` — the spec-aware handler
- Read `internal/tui/views/bigend_chat_handler.go` — the stub handler (if it exists)
- Read `pkg/tui/chatpanel.go` — look at handleStreamChunk for StreamDone, and how SubmitInput works
- Read `pkg/tui/command_picker.go` — look at GlobalCommands() around line 310 for where /new goes
- Print a brief exploration summary before proceeding

## Phase 2: Implement

### 2a. Add SessionID to StreamDone (pkg/tui/chatstream.go)

Add a `SessionID` field to `StreamDone`:
```go
type StreamDone struct {
	FinishReason string
	SessionID    string // Claude CLI session ID for multi-turn
}
```

### 2b. Capture SessionID in event mapping (pkg/tui/claude_handler.go)

In `ClaudeChatHandler.HandleMessage()`, the goroutine that maps claude events:

1. Add a local `var sessionID string` before the event loop
2. When `event.Type == claude.EventSessionID`, capture: `sessionID = event.SessionID` and `continue` (don't emit a StreamMsg)
3. When emitting `StreamDone` for EventResult (non-error), include the sessionID:
   ```go
   msg = StreamDone{FinishReason: "stop", SessionID: sessionID}
   ```

Also add multi-turn support to ClaudeChatHandler:
1. Add `SessionID string` and `Continue bool` fields to ClaudeChatHandler struct
2. In HandleMessage, when building args: if `h.Continue && h.SessionID != ""`, prepend `"--resume", h.SessionID` before `-p`. If `h.Continue && h.SessionID == ""`, prepend just `"-c"` before `-p`.
3. **Do not change** the ExtraArgs handling — multi-turn args go before ExtraArgs.

### 2c. Capture SessionID in GurgehChatHandler (internal/tui/views/gurgeh_chat_handler.go)

Same pattern as 2b:
1. Add `sessionID string` field (protected by existing mu mutex)
2. In the goroutine: capture EventSessionID into a local var, include in StreamDone
3. Add `Continue bool` field. In HandleMessage, build args with `-c` or `--resume` logic.

### 2d. Do the same for bigend_chat_handler.go, coldwine_chat_handler.go, pollard_chat_handler.go

Each stub handler gets the same sessionID capture + continue logic.

### 2e. ChatPanel captures SessionID and sets Continue on handler (pkg/tui/chatpanel.go)

1. Add `MultiTurnHandler` interface in chatpanel.go:
```go
// MultiTurnHandler is optionally implemented by ChatHandlers that support multi-turn.
type MultiTurnHandler interface {
	SetContinue(cont bool, sessionID string)
	ResetSession()
}
```

2. In `handleStreamChunk`, when handling `StreamDone`:
```go
case StreamDone:
	p.chatState = ChatIdle
	p.SetStatus("")
	// Enable multi-turn if handler supports it
	if e.SessionID != "" {
		if mth, ok := p.handler.(MultiTurnHandler); ok {
			mth.SetContinue(true, e.SessionID)
		}
	}
	// ... rest of existing StreamDone handling
```

3. Add `ResetSession()` method on ChatPanel:
```go
func (p *ChatPanel) ResetSession() {
	if mth, ok := p.handler.(MultiTurnHandler); ok {
		mth.ResetSession()
	}
	// Optionally clear chat history:
	p.messages = nil
	p.scroll = 0
}
```

### 2f. Implement MultiTurnHandler on all handlers

For **ClaudeChatHandler** (pkg/tui/claude_handler.go):
```go
func (h *ClaudeChatHandler) SetContinue(cont bool, sessionID string) {
	h.Continue = cont
	h.SessionID = sessionID
}

func (h *ClaudeChatHandler) ResetSession() {
	h.Continue = false
	h.SessionID = ""
}
```

For **GurgehChatHandler** (internal/tui/views/gurgeh_chat_handler.go):
```go
func (h *GurgehChatHandler) SetContinue(cont bool, sessionID string) {
	h.mu.Lock()
	h.continue_ = cont  // use continue_ since continue is a keyword
	h.sessionID = sessionID
	h.mu.Unlock()
}

func (h *GurgehChatHandler) ResetSession() {
	h.mu.Lock()
	h.continue_ = false
	h.sessionID = ""
	h.mu.Unlock()
}
```

Same pattern for Bigend/Coldwine/Pollard handlers, using their local field naming convention.

### 2g. Add /new global command (pkg/tui/command_picker.go)

In `GlobalCommands()`, add this entry:
```go
{Command: "new", Aliases: []string{"clear"}, Description: "New conversation", Category: "chat"},
```

Note: `/n` alias is already taken by KickoffCommands. Use `/clear` as the alias instead.

### 2h. Handle /new in ChatPanel (pkg/tui/chatpanel.go)

In the `SubmitInput()` method, there's a section that handles slash commands. Add handling for `/new`:

Look at how SubmitInput currently dispatches slash commands — it returns a `tea.Cmd` from the command picker for the view to handle. The `/new` command should be handled locally in ChatPanel before returning to the view:

In `SubmitInput()`, after detecting a slash command, check if it's `/new` before delegating to the command picker:
```go
if trimmed == "/new" || trimmed == "/clear" {
	p.ResetSession()
	return nil  // handled locally, no need to propagate
}
```

## Phase 3: Verify
After implementing, run ALL of these and report results:
1. Build: `GOCACHE=/tmp/go-build-cache go build ./cmd/...`
2. Vet: `GOCACHE=/tmp/go-build-cache go vet ./pkg/tui/... ./pkg/claude/... ./internal/tui/views/...`
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
- Modify these files:
  - `pkg/tui/chatstream.go` — add SessionID to StreamDone
  - `pkg/tui/claude_handler.go` — add multi-turn fields + SetContinue/ResetSession
  - `pkg/tui/chatpanel.go` — add MultiTurnHandler interface, capture SessionID, handle /new
  - `pkg/tui/command_picker.go` — add /new command
  - `internal/tui/views/gurgeh_chat_handler.go` — add multi-turn fields + methods
  - `internal/tui/views/bigend_chat_handler.go` — add multi-turn fields + methods
  - `internal/tui/views/coldwine_chat_handler.go` — add multi-turn fields + methods
  - `internal/tui/views/pollard_chat_handler.go` — add multi-turn fields + methods
- Do NOT modify any view files (gurgeh.go, bigend.go, coldwine.go, pollard.go)
- Do NOT modify unified_app.go
- Do NOT commit or push any changes
- If GOCACHE permission errors occur, use: GOCACHE=/tmp/go-build-cache
- Module path is `github.com/mistakeknot/autarch`
- `continue` is a Go keyword — use `continueSession` or `cont` as field names
