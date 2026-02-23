# Streaming Buffer / History Split Per Agent Panel
**Bead:** iv-26pj

**Date:** 2026-02-23
**Bead:** iv-26pj
**Status:** Brainstorm

## Problem Statement

The current `ChatPanel` uses a single `[]ChatMessage` slice for both finalized history and live streaming output. During streaming, `TextDelta` events mutate the last message in-place. `renderHistory()` rebuilds all rendered lines from all messages every frame, then slices by scroll offset.

This creates three problems:

1. **Flicker during live output** — the entire history is re-rendered each frame even though only the streaming region changes. As the conversation grows, re-rendering the full glamour markdown pipeline for every TextDelta gets expensive.

2. **No structural boundary between done and in-flight** — there's no way to tell which messages are finalized vs. being actively streamed. This blocks features like "copy last response" or persistence-to-disk, since you can't distinguish a completed turn from a partial one.

3. **Scroll behavior during streaming is fragile** — `scroll=0` means follow-tail, but scrolling up during streaming races with new TextDelta events that reset `p.scroll = 0`. If a user scrolls up to read earlier output, the next chunk snaps them back to the bottom.

## Current Architecture

```
ChatPanel
├── messages: []ChatMessage          ← flat array, streaming mutates last entry
├── scroll: int                      ← 0=bottom, >0=scrolled up
├── streaming: bool                  ← status flag
├── chatState: ChatState             ← Idle/Thinking/Streaming/Error
├── events: <-chan StreamMsg          ← live event channel
└── renderHistory(height)            ← rebuilds ALL lines every frame
    ├── for each message → render markdown (glamour) or plain text
    ├── append status indicator if streaming
    └── slice by scroll offset
```

Key observations:
- `handleStreamChunk(TextDelta)` appends to `last.Content` and forces `p.scroll = 0`
- `renderHistory` calls `glamour.Render()` on every agent message every frame
- No caching of rendered output for finalized messages
- The streaming indicator (spinner + status) is inlined in the history section

## Design Goals

1. **Separate streaming buffer from finalized history** — two distinct data structures
2. **Only re-render the streaming region during active output** — finalized history is cached
3. **Scroll-anchoring that respects user intent** — if scrolled up, stay scrolled up
4. **Clean turn boundaries** — StreamDone finalizes the buffer into history
5. **Minimal API change** — ChatPanel's public API should remain compatible

## Proposed Architecture

```
ChatPanel
├── history: []ChatMessage           ← finalized messages only
├── historyCache: []string           ← pre-rendered lines (invalidated on resize/new message)
├── currentBuffer: *StreamBuffer     ← nil when idle
│   ├── content: strings.Builder     ← accumulates TextDelta text
│   ├── state: ChatState             ← Thinking/Streaming
│   └── Render(width) string         ← renders only the live region
├── scroll: int                      ← applies to history only
├── followTail: bool                 ← true by default, false when user scrolls up
└── renderView(height)
    ├── if followTail: show bottom of history + full buffer
    ├── if !followTail: show history at scroll offset, buffer hidden or minimized
    └── buffer region is re-rendered each frame (small); history uses cache
```

### StreamBuffer

A small struct that owns the in-flight content:

```go
type StreamBuffer struct {
    content    strings.Builder
    state      ChatState
    role       string      // always "agent" for now
}

func (b *StreamBuffer) Append(text string)          // TextDelta handler
func (b *StreamBuffer) SetState(state ChatState)    // Thinking → Streaming transitions
func (b *StreamBuffer) Render(width int) string     // Render just the live content
func (b *StreamBuffer) Finalize() ChatMessage       // Flush to a ChatMessage, return it
func (b *StreamBuffer) Reset()                      // Clear for next turn
```

### History Cache

Pre-rendered line cache for finalized messages:

```go
type renderedMessage struct {
    lines []string   // pre-rendered at current width
    width int        // width it was rendered at (invalidate on resize)
}
```

On `AddMessage()`, render once and cache. On resize, invalidate all caches. This eliminates the per-frame glamour calls for old messages.

### Scroll-Anchoring

```
followTail = true (default)
├── New TextDelta → stay at bottom (no scroll change needed)
├── User scrolls up → followTail = false, scroll++
├── User presses End/G → followTail = true, scroll = 0
└── StreamDone → do NOT force followTail back to true
    (user might be reading mid-conversation)

followTail = false
├── New TextDelta → no position change (user stays where they are)
├── Buffer grows → doesn't affect history scroll position
└── User scrolls to bottom → followTail = true
```

## Turn Lifecycle

```
1. User sends message
   → AddMessage("user", text) to history
   → currentBuffer = new StreamBuffer{state: ChatThinking}

2. ReasoningStart → buffer.SetState(ChatThinking)
3. TextDelta     → buffer.Append(text); if followTail, re-render buffer region
4. StreamDone    → msg := buffer.Finalize(); AddMessage(msg); currentBuffer = nil
5. StreamError   → same as Done but with error content

Between turns: currentBuffer is nil, only history is rendered
```

## Alternatives Considered

### A. Ring buffer for streaming content
Could use a fixed-size ring buffer for the streaming region to bound memory. Rejected because agent responses are typically small enough that a `strings.Builder` suffices, and truncating mid-stream would lose content.

### B. Virtual viewport with lazy rendering
Instead of pre-rendering and caching, render on-demand only the visible portion. More complex (need to know line heights without rendering) and Bubble Tea's synchronous render model makes this awkward. The cache approach is simpler and effective.

### C. Separate widget for streaming output
Make the streaming buffer a completely separate Bubble Tea component rendered below history. This would be cleaner architecturally but harder to integrate with the existing scroll model and layout calculations in SplitLayout.

## Scope & Non-Goals

**In scope:**
- StreamBuffer struct and lifecycle
- History cache with width-aware invalidation
- followTail scroll-anchoring
- Minimal refactor of renderHistory → split into renderHistory + renderBuffer

**Out of scope (future work):**
- Per-message persistence to disk
- Multi-agent panel routing (multiple buffers)
- Reasoning/tool-call rich rendering in the buffer
- Collapsible message groups

## Risks

1. **Glamour rendering inconsistency** — if we render the buffer with plain text during streaming, then finalize with glamour, there could be a visual "flash" on turn end. Mitigation: render buffer content through glamour too, just don't re-render history.

2. **Scroll math complexity** — two regions (history + buffer) with one scroll offset is tricky. Need clear rules about which region the scroll applies to.

3. **Test coverage** — existing `chatpanel_test.go` tests the flat model. Need to update tests for the split model without breaking existing test patterns.

## Open Questions

1. Should the buffer render markdown incrementally or only plain text during streaming?
2. Should followTail be a per-panel setting or global?
3. When the user is scrolled up and streaming completes, should we show a "new message" indicator?

## Inspiration

- **pi_agent_rust**: Separates live output from conversation history with a clear buffer flush on turn end
- **schmux**: Terminal multiplexer that maintains per-pane scroll buffers independent of active output
