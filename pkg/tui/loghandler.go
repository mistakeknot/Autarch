package tui

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// LogMsg represents a single log entry for display in the TUI.
type LogMsg struct {
	Level   slog.Level
	Message string
	Time    time.Time
}

// LogBatchMsg contains multiple log entries for efficient routing to the TUI.
type LogBatchMsg struct {
	Entries []LogMsg
}

// LogHandler implements slog.Handler with batched message routing to Bubble Tea.
// Logs are buffered in a channel and sent in batches to reduce UI updates.
type LogHandler struct {
	mu      sync.Mutex
	program *tea.Program
	level   slog.Level
	attrs   []slog.Attr
	groups  []string
	msgChan chan LogMsg
	done    chan struct{}
	closed  atomic.Bool
}

// NewLogHandler creates a handler that batches logs before sending to the TUI.
// Call SetProgram() after creating the tea.Program to wire up message routing.
func NewLogHandler(level slog.Level) *LogHandler {
	h := &LogHandler{
		level:   level,
		msgChan: make(chan LogMsg, 256),
		done:    make(chan struct{}),
	}
	go h.batchLoop()
	return h
}

// SetProgram wires the handler to a Bubble Tea program for message routing.
func (h *LogHandler) SetProgram(p *tea.Program) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.program = p
}

// Enabled reports whether the handler handles records at the given level.
func (h *LogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle processes a log record by enqueueing it for batched delivery.
func (h *LogHandler) Handle(_ context.Context, r slog.Record) error {
	msg := LogMsg{
		Level:   r.Level,
		Message: r.Message,
		Time:    r.Time,
	}

	// Non-blocking enqueue - drop on overflow to prevent blocking callers
	select {
	case h.msgChan <- msg:
	default:
	}
	return nil
}

// batchLoop aggregates log messages and sends them in batches.
// Batches are sent when reaching 10 messages or after 100ms, whichever comes first.
func (h *LogHandler) batchLoop() {
	const batchSize = 10
	const batchTime = 100 * time.Millisecond

	ticker := time.NewTicker(batchTime)
	defer ticker.Stop()

	batch := make([]LogMsg, 0, batchSize)

	for {
		select {
		case msg := <-h.msgChan:
			batch = append(batch, msg)
			if len(batch) >= batchSize {
				h.sendBatch(batch)
				batch = make([]LogMsg, 0, batchSize) // New slice to avoid data race
			}
		case <-ticker.C:
			if len(batch) > 0 {
				h.sendBatch(batch)
				batch = make([]LogMsg, 0, batchSize)
			}
		case <-h.done:
			// Flush remaining messages on shutdown
			if len(batch) > 0 {
				h.sendBatch(batch)
			}
			return
		}
	}
}

// sendBatch sends a batch of log messages to the Bubble Tea program.
func (h *LogHandler) sendBatch(batch []LogMsg) {
	h.mu.Lock()
	p := h.program
	h.mu.Unlock()

	if p == nil {
		return
	}

	// Copy batch to avoid race with caller's slice
	entries := make([]LogMsg, len(batch))
	copy(entries, batch)
	p.Send(LogBatchMsg{Entries: entries})
}

// Close shuts down the handler's batch loop. Safe to call multiple times.
func (h *LogHandler) Close() {
	if h.closed.Swap(true) {
		return // Already closed
	}
	close(h.done)
}

// WithAttrs returns a new handler with additional attributes.
// Per slog.Handler contract, this must return a new handler, not mutate self.
func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := *h
	h2.attrs = append(slices.Clone(h.attrs), attrs...)
	return &h2
}

// WithGroup returns a new handler with an additional group.
// Per slog.Handler contract, this must return a new handler, not mutate self.
func (h *LogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.groups = append(slices.Clone(h.groups), name)
	return &h2
}
