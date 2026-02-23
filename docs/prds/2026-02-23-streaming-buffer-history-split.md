# PRD: Streaming Buffer / History Split Per Agent Panel

**Bead:** iv-26pj
**Date:** 2026-02-23
**Status:** Draft
**Priority:** P2

## Summary

Refactor `ChatPanel` to separate live streaming output from finalized conversation history. Introduces a `StreamBuffer` for in-flight content, a rendered-line cache for history, and scroll-anchoring that respects user intent during streaming.

## Problem

The current `ChatPanel.messages` array mixes finalized and in-progress content. During streaming:
- `renderHistory()` re-renders ALL messages (including expensive glamour markdown) every frame
- `TextDelta` forces `scroll=0`, overriding user scrollback
- No structural boundary between completed and in-flight turns

## Goals

1. **Eliminate redundant rendering** — only re-render the streaming region during active output
2. **Clean turn boundaries** — explicit finalization from StreamBuffer → history
3. **Scroll-anchoring** — `followTail` flag that users control, not forced by TextDelta
4. **API compatibility** — ChatPanel's public interface stays the same

## Non-Goals

- Per-message persistence to disk
- Multi-agent routing (multiple concurrent buffers)
- Rich rendering of reasoning/tool-calls in the buffer
- Collapsible message groups

## Features

### F1: StreamBuffer

A new struct that owns in-flight content during a single agent turn.

**Acceptance criteria:**
- `StreamBuffer.Append(text)` accumulates TextDelta content in a `strings.Builder`
- `StreamBuffer.Finalize()` returns a `ChatMessage` and resets the buffer
- `StreamBuffer.Render(width)` renders only the live content (no history re-rendering)
- Buffer is nil when idle (between turns)

### F2: History Cache

Pre-rendered line cache for finalized messages, invalidated on resize.

**Acceptance criteria:**
- Finalized messages are rendered once through glamour and cached as `[]string` lines
- Cache invalidated when panel width changes
- `renderHistory()` reads from cache instead of re-rendering each frame
- Adding a new message renders only that message, appends to cache

### F3: Scroll-Anchoring

Replace `scroll int` with `scroll int` + `followTail bool` for user-respecting behavior.

**Acceptance criteria:**
- `followTail=true` (default): new content keeps view at bottom
- User scrolls up → `followTail=false`, scroll offset increases
- User presses End/G/scrolls to bottom → `followTail=true`
- TextDelta does NOT force `followTail=true` or reset scroll
- StreamDone does NOT force scroll position change

### F4: Turn Lifecycle Integration

Wire StreamBuffer into the existing stream event handling.

**Acceptance criteria:**
- `SubmitInput()` creates a new StreamBuffer
- `handleStreamChunk(TextDelta)` writes to buffer instead of mutating `messages[]`
- `handleStreamChunk(StreamDone)` calls `buffer.Finalize()` and appends to history
- `handleStreamChunk(StreamError)` finalizes with error content
- `CancelStream()` discards or finalizes partial buffer content

## Technical Approach

### Data Model Change

```
Before:
  messages []ChatMessage  ← mixed live + finalized

After:
  history      []ChatMessage       ← finalized only
  historyLines [][]string          ← pre-rendered cache per message
  historyWidth int                 ← width cache was rendered at
  buffer       *StreamBuffer       ← nil when idle
  followTail   bool                ← true by default
```

### Render Pipeline

```
renderView(height)
├── historyHeight = height - bufferHeight
├── renderHistory(historyHeight)  ← reads from historyLines cache
├── renderBuffer()                ← renders only buffer.content (small, per-frame OK)
└── compose vertically
```

### Migration Path

1. Rename `messages` → `history` (internal only)
2. Add StreamBuffer, wire into handleStreamChunk
3. Add history cache, wire into renderHistory
4. Add followTail logic to scroll methods
5. Update tests

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Visual flash on finalize (plain→markdown) | Medium | Low | Render buffer through glamour too |
| Scroll math bugs with two regions | Medium | Medium | Comprehensive scroll tests |
| Test breakage | Low | Low | Incremental refactor, keep public API |

## Success Metrics

- Zero per-frame glamour calls for finalized messages during streaming
- User scroll position stable during TextDelta events when scrolled up
- All existing chatpanel_test.go tests pass after refactor
