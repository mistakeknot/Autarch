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

// logSink is the state shared by a handler and every handler derived from it
// with WithAttrs or WithGroup: one queue, one batch loop, one program.
// It is always held by pointer, so its lock and closed flag are never copied.
type logSink struct {
	mu      sync.Mutex
	program *tea.Program
	msgChan chan LogMsg
	done    chan struct{}
	closed  atomic.Bool
}

// LogHandler implements slog.Handler with batched message routing to Bubble Tea.
// Logs are buffered in a channel and sent in batches to reduce UI updates.
type LogHandler struct {
	sink   *logSink
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

// NewLogHandler creates a handler that batches logs before sending to the TUI.
// Call SetProgram() after creating the tea.Program to wire up message routing.
func NewLogHandler(level slog.Level) *LogHandler {
	sink := &logSink{
		msgChan: make(chan LogMsg, 256),
		done:    make(chan struct{}),
	}
	go sink.batchLoop()
	return &LogHandler{sink: sink, level: level}
}

// SetProgram wires the handler to a Bubble Tea program for message routing.
// Handlers derived from this one share the program, whenever it is set.
func (h *LogHandler) SetProgram(p *tea.Program) {
	h.sink.mu.Lock()
	defer h.sink.mu.Unlock()
	h.sink.program = p
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
	case h.sink.msgChan <- msg:
	default:
	}
	return nil
}

// batchLoop aggregates log messages and sends them in batches.
// Batches are sent when reaching 10 messages or after 100ms, whichever comes first.
func (s *logSink) batchLoop() {
	const batchSize = 10
	const batchTime = 100 * time.Millisecond

	ticker := time.NewTicker(batchTime)
	defer ticker.Stop()

	batch := make([]LogMsg, 0, batchSize)

	for {
		select {
		case msg := <-s.msgChan:
			batch = append(batch, msg)
			if len(batch) >= batchSize {
				s.sendBatch(batch)
				batch = make([]LogMsg, 0, batchSize) // New slice to avoid data race
			}
		case <-ticker.C:
			if len(batch) > 0 {
				s.sendBatch(batch)
				batch = make([]LogMsg, 0, batchSize)
			}
		case <-s.done:
			// Flush remaining messages on shutdown
			if len(batch) > 0 {
				s.sendBatch(batch)
			}
			return
		}
	}
}

// sendBatch sends a batch of log messages to the Bubble Tea program.
func (s *logSink) sendBatch(batch []LogMsg) {
	s.mu.Lock()
	p := s.program
	s.mu.Unlock()

	if p == nil {
		return
	}

	// Copy batch to avoid race with caller's slice
	entries := make([]LogMsg, len(batch))
	copy(entries, batch)
	p.Send(LogBatchMsg{Entries: entries})
}

// Close shuts down the batch loop. Safe to call multiple times, and on a
// derived handler as well as the one it came from: they share one loop.
func (h *LogHandler) Close() {
	if h.sink.closed.Swap(true) {
		return // Already closed
	}
	close(h.sink.done)
}

// WithAttrs returns a new handler with additional attributes.
// Per slog.Handler contract, this must return a new handler, not mutate self.
func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogHandler{
		sink:   h.sink,
		level:  h.level,
		attrs:  append(slices.Clone(h.attrs), attrs...),
		groups: slices.Clone(h.groups),
	}
}

// WithGroup returns a new handler with an additional group.
// Per slog.Handler contract, this must return a new handler, not mutate self.
func (h *LogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &LogHandler{
		sink:   h.sink,
		level:  h.level,
		attrs:  slices.Clone(h.attrs),
		groups: append(slices.Clone(h.groups), name),
	}
}
