package tui

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestLogHandler_Enabled(t *testing.T) {
	h := NewLogHandler(slog.LevelInfo)
	defer h.Close()

	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected Info to be enabled at Info level")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("expected Warn to be enabled at Info level")
	}
	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected Debug to be disabled at Info level")
	}
}

func TestLogHandler_Batching(t *testing.T) {
	h := NewLogHandler(slog.LevelDebug)
	defer h.Close()

	// For this test, we'll send 15 messages and verify batching behavior
	// Since we can't easily mock tea.Program (it's a concrete type), we'll test the handler's core logic

	// Test that handler doesn't block when no program is set
	for i := 0; i < 15; i++ {
		r := slog.Record{}
		r.Time = time.Now()
		r.Level = slog.LevelInfo
		r.Message = "test message"
		if err := h.Handle(context.Background(), r); err != nil {
			t.Fatalf("Handle returned error: %v", err)
		}
	}

	// Without a program set, messages should be dropped (no panic)
	// This tests the non-blocking enqueue behavior

	// Close should not panic
	h.Close()

	// Double close should be safe
	h.Close()
}

func TestLogHandler_WithAttrs(t *testing.T) {
	h := NewLogHandler(slog.LevelInfo)
	defer h.Close()

	// WithAttrs should return a new handler
	h2 := h.WithAttrs([]slog.Attr{slog.String("key", "value")})
	if h2 == h {
		t.Error("WithAttrs should return a new handler")
	}

	// Original handler should be unchanged
	if len(h.attrs) != 0 {
		t.Error("original handler should not be modified")
	}

	// New handler should have attrs
	h2Typed, ok := h2.(*LogHandler)
	if !ok {
		t.Fatal("WithAttrs should return *LogHandler")
	}
	if len(h2Typed.attrs) != 1 {
		t.Errorf("expected 1 attr, got %d", len(h2Typed.attrs))
	}
}

func TestLogHandler_WithGroup(t *testing.T) {
	h := NewLogHandler(slog.LevelInfo)
	defer h.Close()

	// WithGroup with empty name should return same handler
	h2 := h.WithGroup("")
	if h2 != h {
		t.Error("WithGroup('') should return same handler")
	}

	// WithGroup with name should return new handler
	h3 := h.WithGroup("mygroup")
	if h3 == h {
		t.Error("WithGroup should return a new handler")
	}

	h3Typed, ok := h3.(*LogHandler)
	if !ok {
		t.Fatal("WithGroup should return *LogHandler")
	}
	if len(h3Typed.groups) != 1 || h3Typed.groups[0] != "mygroup" {
		t.Errorf("expected groups=['mygroup'], got %v", h3Typed.groups)
	}
}

func TestLogHandler_ChannelOverflow(t *testing.T) {
	h := NewLogHandler(slog.LevelDebug)
	defer h.Close()

	// Fill the channel buffer (256) and then some
	// This should not block or panic
	for i := 0; i < 300; i++ {
		r := slog.Record{}
		r.Time = time.Now()
		r.Level = slog.LevelInfo
		r.Message = "overflow test"
		if err := h.Handle(context.Background(), r); err != nil {
			t.Fatalf("Handle returned error: %v", err)
		}
	}
	// If we got here without blocking, the test passes
}

func TestLogHandler_CloseIdempotent(t *testing.T) {
	h := NewLogHandler(slog.LevelDebug)

	// Multiple closes should be safe
	h.Close()
	h.Close()
	h.Close()
	// If we got here without panic, the test passes
}
