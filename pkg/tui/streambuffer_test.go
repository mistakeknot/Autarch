package tui

import "testing"

func TestStreamBufferAppend(t *testing.T) {
	b := NewStreamBuffer()
	b.Append("Hello ")
	b.Append("world")
	if got := b.String(); got != "Hello world" {
		t.Fatalf("String() = %q, want %q", got, "Hello world")
	}
	if got := b.Len(); got != 11 {
		t.Fatalf("Len() = %d, want 11", got)
	}
}

func TestStreamBufferFinalize(t *testing.T) {
	b := NewStreamBuffer()
	b.SetState(ChatStreaming)
	b.Append("response text")

	msg := b.Finalize()

	if msg.Role != "agent" {
		t.Fatalf("Role = %q, want %q", msg.Role, "agent")
	}
	if msg.Content != "response text" {
		t.Fatalf("Content = %q, want %q", msg.Content, "response text")
	}
	// Buffer should be reset after finalize
	if b.Len() != 0 {
		t.Fatalf("Len() after Finalize = %d, want 0", b.Len())
	}
	if b.State() != ChatIdle {
		t.Fatalf("State() after Finalize = %v, want ChatIdle", b.State())
	}
}

func TestStreamBufferStateTransitions(t *testing.T) {
	b := NewStreamBuffer()
	if b.State() != ChatThinking {
		t.Fatalf("initial state = %v, want ChatThinking", b.State())
	}

	b.SetState(ChatStreaming)
	if b.State() != ChatStreaming {
		t.Fatalf("state after SetState = %v, want ChatStreaming", b.State())
	}

	b.Append("text")
	msg := b.Finalize()
	if msg.Content != "text" {
		t.Fatalf("Content = %q, want %q", msg.Content, "text")
	}
	if b.State() != ChatIdle {
		t.Fatalf("state after Finalize = %v, want ChatIdle", b.State())
	}
}

func TestStreamBufferReset(t *testing.T) {
	b := NewStreamBuffer()
	b.SetState(ChatStreaming)
	b.Append("some content")
	b.Reset()

	if b.Len() != 0 {
		t.Fatalf("Len() after Reset = %d, want 0", b.Len())
	}
	if b.String() != "" {
		t.Fatalf("String() after Reset = %q, want empty", b.String())
	}
	if b.State() != ChatIdle {
		t.Fatalf("State() after Reset = %v, want ChatIdle", b.State())
	}
}
