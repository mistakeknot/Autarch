---
module: bigend
date: 2026-02-23
problem_type: performance
component: pkg/tui/chatpanel
symptoms:
  - "Flicker during live agent output as full history re-renders each frame"
  - "User scroll position snaps to bottom on every TextDelta during streaming"
  - "No structural boundary between finalized and in-flight messages"
root_cause: "Single flat []ChatMessage array mixed streaming and finalized content. TextDelta mutated the last message in-place and forced scroll=0. renderHistory() called glamour.Render() on every message every frame."
severity: medium
tags: [tui, streaming, performance, scroll, bubble-tea, glamour]
---

# Streaming Buffer / History Split Pattern

## Problem Statement

`ChatPanel` used a single `[]ChatMessage` slice for both finalized history and live streaming output. During streaming, `TextDelta` events mutated the last message in-place via pointer (`last := &p.messages[len(p.messages)-1]`), `renderHistory()` re-ran glamour markdown rendering on ALL messages every frame, and `p.scroll = 0` was forced on every `TextDelta`, overriding any user scrollback.

## Investigation

1. **Profiling the render path**: `renderHistory()` iterated all messages, called `glamour.Render()` per agent message, split into lines, then sliced by scroll offset. For N messages, this was O(N * glamour_cost) per frame during streaming.
2. **Scroll behavior**: The line `p.scroll = 0` in the `TextDelta` handler meant any user who scrolled up to read earlier content was snapped back to the bottom on the next token arrival (~every 50ms).
3. **No turn boundaries**: `Messages()` returned the flat array — callers couldn't distinguish a completed response from a partial one being actively streamed.

## Root Cause

Three separate concerns (in-flight accumulation, finalized rendering, scroll intent) were collapsed into one data structure and one render path.

## Solution

Separate into three distinct mechanisms:

### 1. StreamBuffer — owns in-flight content

```go
// BEFORE:
p.messages = append(p.messages, ChatMessage{Role: "agent", Content: ""})
// ... in TextDelta handler:
last := &p.messages[len(p.messages)-1]
last.Content += e.Text

// AFTER:
p.buffer = NewStreamBuffer()  // created on SubmitInput
// ... in TextDelta handler:
p.buffer.Append(e.Text)       // writes to strings.Builder
// ... on StreamDone:
msg := p.buffer.Finalize()    // flush to ChatMessage
p.AddMessage(msg.Role, msg.Content)
p.buffer = nil
```

`StreamBuffer` is nil between turns. Only non-nil during active streaming.

### 2. History line cache — render once per message

```go
// BEFORE: renderHistory() calls glamour.Render() on every agent message every frame

// AFTER: render at add-time, cache per message
type ChatPanel struct {
    history      []ChatMessage
    historyLines [][]string   // pre-rendered lines per message index
    historyWidth int          // width cache was rendered at
}

func (p *ChatPanel) AddMessage(role, content string) {
    // ... append to history ...
    lines := p.renderMessageLines(msg, contentWidth, prevRole)
    p.historyLines = append(p.historyLines, lines)
}
```

Cache invalidated on resize (`SetSize` clears if width changed). `rebuildHistoryCache()` handles the miss path.

### 3. followTail scroll-anchoring — explicit intent flag

```go
// BEFORE: TextDelta forced p.scroll = 0

// AFTER: separate intent from position
type ChatPanel struct {
    scroll     int   // offset (0 = bottom)
    followTail bool  // true = auto-follow new content
}

func (p *ChatPanel) ScrollUp() {
    p.followTail = false  // user chose to look at older content
    p.scroll++
}

func (p *ChatPanel) ScrollDown() {
    if p.scroll > 0 { p.scroll-- }
    if p.scroll == 0 { p.followTail = true }  // back at bottom
}

// TextDelta handler: does NOT touch scroll or followTail
```

### View composition

```go
func (p *ChatPanel) View() string {
    // Split height: history gets most, buffer gets up to 1/3
    if p.buffer != nil {
        bufferView = p.renderBuffer(p.width)  // re-rendered each frame (small)
        bufferHeight = min(totalHeight/3, height(bufferView))
    }
    historyHeight = totalHeight - bufferHeight
    historyView = p.renderHistory(historyHeight)  // reads from cache
    // compose: [history] [buffer] [separator] [composer]
}
```

## Files Changed

- `pkg/tui/streambuffer.go` — new StreamBuffer type
- `pkg/tui/streambuffer_test.go` — unit tests
- `pkg/tui/chatpanel.go` — refactored history, buffer, followTail, cache
- `pkg/tui/chatpanel_test.go` — 12 new tests

## Prevention

### Detection - Catch Early
- If `renderHistory` ever calls `glamour.Render()` in a loop over all messages, it's the same bug returning
- If any streaming handler writes `p.scroll = 0`, it's overriding user intent

### Best Practices
- **Separate accumulation from display**: mutable in-flight data should live in a dedicated struct, not mixed into the finalized collection
- **Cache rendered output**: if rendering is expensive (glamour, syntax highlighting), render once on add and invalidate on resize
- **Model user intent explicitly**: `followTail` as a boolean is clearer than inferring from `scroll == 0`
- **Buffer nil-ness as state**: `buffer == nil` means "not streaming" — simpler than a boolean flag

### Testing
- `TestChatPanelStreamingDoesNotForceScroll` — verifies scroll/followTail stability during TextDelta
- `TestChatPanelStreamBuffer` — verifies buffer accumulation and finalization
- `TestChatPanelCacheInvalidation` — verifies resize triggers rebuild

## Key Insight

In any TUI that renders streaming content alongside history, **separate the mutable streaming region from the immutable history region**. The streaming buffer is small and cheap to re-render every frame. The history is large and expensive — render it once and cache. The scroll position belongs to the user, not to the streaming pipeline.

## Related

- `docs/brainstorms/2026-02-23-streaming-buffer-history-split.md` — original brainstorm
- `docs/prds/2026-02-23-streaming-buffer-history-split.md` — PRD
- `docs/solutions/patterns/chat-first-tui-design-20260204.md` — earlier ChatPanel design pattern
- Bead: iv-26pj
