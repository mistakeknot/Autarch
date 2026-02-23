package tui

import "strings"

// StreamBuffer owns in-flight agent content during a single streaming turn.
// It accumulates TextDelta chunks and finalizes into a ChatMessage when the
// turn ends. Between turns, the ChatPanel holds a nil *StreamBuffer.
type StreamBuffer struct {
	content strings.Builder
	state   ChatState
}

// NewStreamBuffer creates a buffer in the Thinking state.
func NewStreamBuffer() *StreamBuffer {
	return &StreamBuffer{state: ChatThinking}
}

// Append adds incremental text content (from a TextDelta event).
func (b *StreamBuffer) Append(text string) {
	b.content.WriteString(text)
}

// SetState updates the streaming state (e.g. Thinking → Streaming).
func (b *StreamBuffer) SetState(state ChatState) {
	b.state = state
}

// State returns the current streaming state.
func (b *StreamBuffer) State() ChatState {
	return b.state
}

// String returns the accumulated text content.
func (b *StreamBuffer) String() string {
	return b.content.String()
}

// Len returns the byte length of accumulated content.
func (b *StreamBuffer) Len() int {
	return b.content.Len()
}

// Finalize flushes the buffer into a ChatMessage and resets it.
// Returns the finalized message with role "agent".
func (b *StreamBuffer) Finalize() ChatMessage {
	msg := ChatMessage{
		Role:    "agent",
		Content: b.content.String(),
	}
	b.Reset()
	return msg
}

// Reset clears the buffer for reuse.
func (b *StreamBuffer) Reset() {
	b.content.Reset()
	b.state = ChatIdle
}
