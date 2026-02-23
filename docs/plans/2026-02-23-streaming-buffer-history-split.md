# Plan: Streaming Buffer / History Split Per Agent Panel

**Bead:** iv-26pj
**PRD:** `docs/prds/2026-02-23-streaming-buffer-history-split.md`
**Date:** 2026-02-23

## Overview

Refactor `ChatPanel` in `pkg/tui/chatpanel.go` to separate live streaming output from finalized conversation history. Four incremental changes, each independently testable.

## Tasks

### Task 1: Add StreamBuffer struct

**File:** `pkg/tui/streambuffer.go` (new)

Create `StreamBuffer` — a small struct that owns in-flight agent content during a single turn.

```go
type StreamBuffer struct {
    content strings.Builder
    state   ChatState
}

func NewStreamBuffer() *StreamBuffer
func (b *StreamBuffer) Append(text string)
func (b *StreamBuffer) SetState(state ChatState)
func (b *StreamBuffer) State() ChatState
func (b *StreamBuffer) String() string              // current accumulated text
func (b *StreamBuffer) Len() int                    // bytes accumulated
func (b *StreamBuffer) Finalize() ChatMessage       // flush to ChatMessage, reset
func (b *StreamBuffer) Reset()
```

`Finalize()` returns `ChatMessage{Role: "agent", Content: b.content.String()}` and calls `Reset()`.

**Test file:** `pkg/tui/streambuffer_test.go`
- TestStreamBufferAppend: append multiple TextDelta, verify String()
- TestStreamBufferFinalize: verify returns ChatMessage with correct content, resets buffer
- TestStreamBufferStateTransitions: Thinking → Streaming → finalize

**Depends on:** nothing
**Risk:** none — purely additive

---

### Task 2: Add history line cache

**File:** `pkg/tui/chatpanel.go` (edit)

Add a per-message rendered-line cache to avoid re-rendering glamour markdown every frame.

New fields on `ChatPanel`:
```go
historyLines [][]string  // cached rendered lines per message index
historyWidth int         // width the cache was rendered at
```

New method:
```go
func (p *ChatPanel) renderMessageLines(msg ChatMessage, width int) []string
```

This extracts the per-message rendering logic from `renderHistory()` into a standalone function. `renderHistory()` is refactored to:
1. Check if `p.historyWidth != contentWidth` → invalidate all caches
2. For each message, use `p.historyLines[i]` if present, otherwise call `renderMessageLines()` and cache
3. Scroll/slice as before

`AddMessage()` renders the new message immediately and appends to cache.
`ClearMessages()` / `ResetSession()` also clears the cache.
`SetSize()` invalidates the cache if width changed.

**Test additions to `chatpanel_test.go`:**
- TestChatPanelCacheInvalidation: add message, resize, verify re-renders
- TestChatPanelRenderMessages already passes (behavior unchanged)

**Depends on:** nothing (can be done in parallel with Task 1)
**Risk:** low — extracting existing render logic into a cacheable form

---

### Task 3: Wire StreamBuffer into ChatPanel

**File:** `pkg/tui/chatpanel.go` (edit)

Replace the current "mutate last message" streaming approach with StreamBuffer.

Changes:
1. Rename `messages` → `history` (internal field only — `Messages()` public method unchanged)
2. Add `buffer *StreamBuffer` field (nil when idle)
3. Add `followTail bool` field (default true)

**SubmitInput() changes:**
- Instead of appending an empty agent message to `messages`, create `p.buffer = NewStreamBuffer()`
- Still add user message to history

**handleStreamChunk() changes:**
- `TextDelta`: write to `p.buffer.Append(e.Text)` instead of mutating `messages[last]`
  - Do NOT touch `p.scroll` or `p.followTail`
- `ReasoningStart`: `p.buffer.SetState(ChatThinking)`
- `StreamDone`: `msg := p.buffer.Finalize(); p.AddMessage(msg.Role, msg.Content); p.buffer = nil`
- `StreamError`: finalize buffer with error text, set buffer nil

**renderHistory() → renderView() refactor:**
- `renderHistory(height)` only renders from `p.history` + cache (no buffer)
- New `renderBuffer(width)` method renders the buffer's current content + spinner/status
- `View()` composes: if buffer != nil, split height between history and buffer

**Scroll changes:**
- `ScrollUp()`: set `p.followTail = false`, increment scroll
- `ScrollDown()`: decrement scroll; if scroll == 0, set `p.followTail = true`
- `ScrollToBottom()`: set `p.followTail = true`, scroll = 0
- Remove `p.scroll = 0` from TextDelta handler

**Messages() compat:** returns `p.history` copy. If buffer is non-nil and non-empty, append a partial ChatMessage for it (preserves existing behavior where callers see the in-progress message).

**Tests:**
- TestChatPanelStreamBuffer: submit, send TextDelta chunks, verify buffer accumulates, send StreamDone, verify finalized in history
- TestChatPanelFollowTail: scroll up during streaming, verify position stable on TextDelta
- TestChatPanelScrollToBottom: scroll up, then scroll to bottom, verify followTail restored
- Existing tests remain green (public API unchanged)

**Depends on:** Task 1 (StreamBuffer), Task 2 (history cache)
**Risk:** medium — core refactor of the rendering pipeline. Incremental and reversible.

---

### Task 4: Buffer rendering

**File:** `pkg/tui/chatpanel.go` (edit)

Implement `renderBuffer()` to show live streaming content below history.

```go
func (p *ChatPanel) renderBuffer(width, maxHeight int) string
```

Logic:
- If `p.buffer == nil`, return ""
- If `p.buffer.State() == ChatThinking`, show spinner + "Thinking..."
- If `p.buffer.State() == ChatStreaming`, render `p.buffer.String()` through glamour (width-constrained) + spinner + "Responding..."
- Truncate to `maxHeight` lines (show tail)

**View() layout:**
```
totalHeight = p.height - composerHeight - 1 (separator) - selectorHeight
if buffer != nil:
    bufferHeight = min(totalHeight/3, lipgloss.Height(bufferView))
    historyHeight = totalHeight - bufferHeight
else:
    historyHeight = totalHeight
    bufferHeight = 0
```

The buffer gets at most 1/3 of the available space — history dominates. During active streaming with `followTail=true`, the buffer is always visible at the bottom.

When `followTail=false` (user scrolled up), still render the buffer but it naturally occupies the bottom of the viewport. The history region above respects the scroll offset.

**Tests:**
- TestChatPanelBufferRendering: verify buffer region appears during streaming
- TestChatPanelBufferDisappearsOnDone: verify buffer gone after StreamDone

**Depends on:** Task 3
**Risk:** low — rendering addition on top of the refactored structure

## File Summary

| File | Action |
|------|--------|
| `pkg/tui/streambuffer.go` | New |
| `pkg/tui/streambuffer_test.go` | New |
| `pkg/tui/chatpanel.go` | Edit (major refactor) |
| `pkg/tui/chatpanel_test.go` | Edit (add tests, update existing) |

## Verification

```bash
cd apps/autarch && go test -race ./pkg/tui/...
```

All existing tests must pass. New tests cover:
- StreamBuffer lifecycle (append, finalize, reset)
- History cache invalidation on resize
- Scroll-anchoring during streaming
- Buffer rendering and cleanup
