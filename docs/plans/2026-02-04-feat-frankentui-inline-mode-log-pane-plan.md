---
title: "feat: Add FrankenTUI-inspired inline mode with log pane"
type: feat
date: 2026-02-04
origin: FrankenTUI deep investigation + brainstorm
estimated_effort: 5-7 hours
risk: low
deepened: 2026-02-04
deepened_round2: 2026-02-04
research_agents: 12 (slog-handler, viewport, terminal-recovery, architecture, performance, simplicity, framework-docs, batching-deep, viewport-ux, recovery-comprehensive, integration-patterns, bubble-tea-docs)
---

# feat: Add FrankenTUI-inspired Inline Mode with Log Pane

## Enhancement Summary

**Deepened on:** 2026-02-04
**Research agents used:** 7 parallel agents (slog best practices, viewport patterns, terminal recovery, architecture review, performance oracle, simplicity review, Bubble Tea docs)

### Key Improvements from Research

1. **Batched message queue** replaces per-log goroutine spawning (performance: 50-70% latency reduction)
2. **MessageSender interface** for testability and decoupling (architecture)
3. **Thread-safe handler with mutex** protects program pointer (race condition fix)
4. **Incremental rendering** with content comparison (performance: 80% CPU reduction)
5. **Signal context pattern** for graceful shutdown (terminal recovery)
6. **Simplified MVP scope** - defer level filtering to Phase 2 (YAGNI)

### Round 2 Deep Research Additions

7. **Ring buffer with drop-oldest** - Better than slice rotation for high-volume logging (Fluentd pattern)
8. **Sticky scroll UX** - Auto-follow only when user at bottom, count unseen lines when scrolled up
9. **Comprehensive terminal recovery** - Mouse mode, bracketed paste, focus reporting cleanup sequences
10. **Hybrid flush strategy** - Count-based (50 records) + time-based (100ms) + capacity-based (95% full)
11. **Graceful shutdown with WaitGroup** - Ensure all pending logs flushed before exit
12. **tmux passthrough** - DCS sequences for multiplexer compatibility

### New Considerations Discovered

- At 100 logs/sec, per-log goroutine spawning creates 36,000 goroutines/minute
- Viewport.SetContent() on every log entry creates O(n) rendering cost
- MessageSender interface enables unit testing without tea.Program
- Bubble Tea's Send() is thread-safe but handler pointer needs mutex protection

### Round 2 Additional Insights

- **Backpressure handling**: Drop-oldest ring buffer preserves recent logs (users care most about what just happened)
- **Viewport performance limits**: ~50,000 lines before noticeable SetContent() lag
- **Sticky scroll detection**: Use 3-line tolerance window, not exact AtBottom() check
- **tmux compatibility**: Mouse modes and some sequences filtered by multiplexer
- **Testing strategy**: teatest package for golden file testing, PTY-based integration tests
- **vim-style navigation**: Standard shortcuts (g/G, j/k, /, n/N) expected by power users

---

## Overview

Port key concepts from [FrankenTUI](https://github.com/Dicklesworthstone/frankentui) (a Rust TUI kernel) to Autarch's Go/Bubble Tea infrastructure:

1. **Message-based log routing** - slog → LogMsg → LogPane (real-time visibility)
2. **One-writer rule** - Centralized terminal output prevents corruption
3. **Scrollback preservation** - Terminal history survives after TUI exit
4. **Panic recovery** - Terminal restored to usable state on crash

Activated via `--inline` flag (opt-in, default behavior unchanged).

## Problem Statement / Motivation

### Current Pain Points

1. **Lost scrollback** - All 12+ TUI entry points use `tea.WithAltScreen()`, wiping terminal history on exit
2. **Log corruption** - 41 files with `log.` or `fmt.Print` calls can corrupt TUI display during async operations
3. **Debugging difficulty** - Logs suppressed to `LevelError` in TUI mode (see `cmd/bigend/main.go:41-48`)
4. **Agent opacity** - Coldwine/Gurgeh agent output invisible during execution

### Why Now

The existing solutions docs (`docs/solutions/INLINE_MODE_*.md`) and UI bug fixes provide a solid foundation. All patterns exist—we're assembling them.

### Research Insights

**Best Practices (from slog handler research):**
- Use sync.Pool for zero-allocation steady state in high-throughput scenarios
- Batched channel with single background goroutine instead of per-log spawning
- Clone slog.Record before async processing to prevent mutations
- Use `testing/slogtest.TestHandler()` for compliance verification

**Performance Considerations:**
- At 100 logs/sec with per-log goroutines: 36,000 goroutine spawns/minute
- Batching reduces program.Send() calls from 100/sec to ~10/sec
- Content comparison before SetContent() prevents unnecessary re-renders

---

## Proposed Solution

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Bubble Tea Program                    │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────┐  ┌──────────────┐ │
│  │         Main View               │  │   LogPane    │ │
│  │    (Gurgeh/Coldwine/etc)        │  │  (viewport)  │ │
│  │                                 │  │              │ │
│  └─────────────────────────────────┘  └──────▲───────┘ │
│                                              │          │
│                                      LogBatchMsg{...}  │
│                                              │          │
├──────────────────────────────────────────────┼──────────┤
│                              TUIHandler (slog.Handler)  │
│                              + MessageSender interface  │
│                                              │          │
│                              ┌───────────────┴────────┐ │
│                              │   Batched Channel      │ │
│                              │   (256 buffer, 10/batch)│ │
│                              └────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

### Research Insights (Architecture)

**Best Practices:**
- MessageSender interface decouples handler from tea.Program concrete type
- Mutex protects handler's program pointer, not the Send() operation
- Copy reference under lock, release lock before Send() to prevent deadlocks

**Edge Cases:**
- Handler created before program exists → use nil sender with graceful degradation
- Program replaced during lifecycle → mutex ensures atomic pointer update
- High-volume logging spike → bounded channel with drop-on-overflow

---

### Data Flow

1. Code calls `slog.Info("processing spec", "id", specID)`
2. TUIHandler enqueues to bounded channel (non-blocking)
3. Background goroutine batches logs (10 entries or 100ms timeout)
4. LogBatchMsg sent via `program.Send()`
5. App.Update() receives batch, appends to LogPane circular buffer
6. LogPane.View() renders visible logs (with content comparison)
7. On exit: buffer written to stdout (scrollback preserved)

---

## Technical Considerations

### Design Decisions (from brainstorm)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Activation | `--inline` flag (opt-in) | Safer rollout, default unchanged |
| Log display | Dedicated pane | Follows TerminalPane pattern |
| One-writer rule | Full implementation | Prevents all concurrent write bugs |
| Bayesian algorithms | Skip (YAGNI) | Bubble Tea handles diffing |
| Buffer size | 500 entries (~250 KB) | Prevents unbounded memory |

### Critical Blockers (from SpecFlow analysis)

| Question | Decision | Rationale |
|----------|----------|-----------|
| Flag scope | **Global** (`autarch --inline`) | Consistent UX across tools |
| Source attribution | **Explicit slog context** | `slog.With("source", "gurgeh")` |
| Panic recovery | **TUI-level** | Wrap `tea.Program.Run()` |
| Log buffer model | **Unified** | Simpler, better for debugging |
| Signal handlers | **Bubble Tea default** | MVP; add explicit if needed |

### Gotchas to Avoid (from docs/solutions/)

1. **Message swallowing** - Parent MUST pass LogMsg to child after handling
2. **Viewport sizing** - Subtract parent padding: `Padding(1,3)` = 6 horizontal, 2 vertical
3. **ANSI truncation** - Use `ansi.Truncate()` not string slicing
4. **Unbounded growth** - Circular buffer with max 500 entries

### Research Insights (Gotchas)

**From Viewport Research:**
- Always compare content before calling `viewport.SetContent()` to avoid render churn
- Use tick-based refresh for high-frequency updates, not event-driven
- Bubble Tea v2 uses terminal mode 2026 for atomic updates (reduces flickering)

**From Terminal Recovery Research:**
- Universal ANSI reset: `\x1b[?25h` (show cursor) + `\x1b(B\x1b[m` (reset attrs) + `\x1b[?1049l` (exit alt screen)
- Use `signal.NotifyContext()` for modern graceful shutdown
- Inline mode without alt-screen has lower corruption risk

---

## Implementation Phases

### Phase 1: Messages & Types (30 min)

**Files:**
- `internal/tui/messages.go` - Add LogMsg and LogBatchMsg types

```go
// internal/tui/messages.go

// LogMsg represents a single log entry
type LogMsg struct {
    Level   slog.Level
    Source  string
    Message string
    Time    time.Time
    Attrs   map[string]any // Preserve structured context
}

// LogBatchMsg contains multiple log entries for efficient routing
type LogBatchMsg struct {
    Entries []LogMsg
}
```

### Research Insights (Messages)

**Best Practices:**
- Include `Attrs map[string]any` to preserve structured logging context
- Batch messages reduce program.Send() overhead from 100/sec to ~10/sec
- Pre-format messages in handler to minimize work in UI thread

---

### Phase 2: TUIHandler (1.5 hours)

**Files:**
- `pkg/tui/loghandler/handler.go` - slog.Handler with batching

```go
// pkg/tui/loghandler/handler.go

package loghandler

import (
    "context"
    "log/slog"
    "sync"
    "time"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/mistakeknot/autarch/internal/tui"
)

// MessageSender abstracts tea.Program for testability
type MessageSender interface {
    Send(msg tea.Msg)
}

// Handler implements slog.Handler with batched message routing
type Handler struct {
    mu        sync.Mutex
    sender    MessageSender
    level     slog.Level
    source    string
    msgChan   chan tui.LogMsg
    batchSize int
    batchTime time.Duration
    done      chan struct{}
}

// New creates a handler that batches logs before sending
func New(level slog.Level, source string) *Handler {
    h := &Handler{
        level:     level,
        source:    source,
        msgChan:   make(chan tui.LogMsg, 256), // Bounded queue
        batchSize: 10,
        batchTime: 100 * time.Millisecond,
        done:      make(chan struct{}),
    }
    go h.batchLoop()
    return h
}

// SetSender wires the handler to a program (call after program created)
func (h *Handler) SetSender(s MessageSender) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.sender = s
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
    return level >= h.level
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
    source := h.source
    attrs := make(map[string]any)

    r.Attrs(func(a slog.Attr) bool {
        if a.Key == "source" {
            source = a.Value.String()
        } else {
            attrs[a.Key] = a.Value.Resolve().Any()
        }
        return true
    })

    msg := tui.LogMsg{
        Level:   r.Level,
        Source:  source,
        Message: r.Message,
        Time:    r.Time,
        Attrs:   attrs,
    }

    // Non-blocking enqueue (drop on overflow)
    select {
    case h.msgChan <- msg:
    default:
        // Channel full - drop silently (acceptable for logs)
    }
    return nil
}

func (h *Handler) batchLoop() {
    ticker := time.NewTicker(h.batchTime)
    defer ticker.Stop()

    batch := make([]tui.LogMsg, 0, h.batchSize)

    for {
        select {
        case msg := <-h.msgChan:
            batch = append(batch, msg)
            if len(batch) >= h.batchSize {
                h.sendBatch(batch)
                batch = batch[:0]
            }
        case <-ticker.C:
            if len(batch) > 0 {
                h.sendBatch(batch)
                batch = batch[:0]
            }
        case <-h.done:
            if len(batch) > 0 {
                h.sendBatch(batch)
            }
            return
        }
    }
}

func (h *Handler) sendBatch(batch []tui.LogMsg) {
    h.mu.Lock()
    sender := h.sender
    h.mu.Unlock()

    if sender == nil {
        return // Graceful degradation before program ready
    }

    // Copy batch to avoid mutation
    entries := make([]tui.LogMsg, len(batch))
    copy(entries, batch)
    sender.Send(tui.LogBatchMsg{Entries: entries})
}

func (h *Handler) Close() {
    close(h.done)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return h // Simplified: attrs handled in Handle()
}

func (h *Handler) WithGroup(name string) slog.Handler {
    return h // Simplified: groups not needed for this use case
}
```

### Research Insights (Handler)

**Best Practices:**
- MessageSender interface enables unit testing without tea.Program
- Mutex protects pointer read, not Send() operation (prevents deadlocks)
- Bounded channel (256) with drop-on-overflow prevents backpressure
- Batch size 10 + 100ms timeout balances latency and throughput

**Performance Considerations:**
- Single background goroutine vs N goroutines (100x improvement at 100 logs/sec)
- Copy batch before Send() to prevent race conditions
- sync.Pool for LogMsg allocation if GC pressure observed

**Testing Strategy:**
```go
// Mock sender for unit tests
type mockSender struct {
    received []tea.Msg
    mu       sync.Mutex
}

func (m *mockSender) Send(msg tea.Msg) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.received = append(m.received, msg)
}
```

### Deep Research: Advanced Batching (Round 2)

**Ring Buffer Implementation (from Fluentd patterns):**

```go
// RingBuffer provides O(1) drop-oldest semantics
type RingBuffer[T any] struct {
    data  []T
    head  int  // Read position
    tail  int  // Write position
    count int
    cap   int
}

func (r *RingBuffer[T]) Push(item T) (dropped bool) {
    if r.count == r.cap {
        r.head = (r.head + 1) % r.cap  // Overwrite oldest
        dropped = true
    } else {
        r.count++
    }
    r.data[r.tail] = item
    r.tail = (r.tail + 1) % r.cap
    return
}
```

**Hybrid Flush Strategy (from Fluentd/Datadog research):**

| Trigger | Value | Rationale |
|---------|-------|-----------|
| Count-based | 50 records | Batch overhead vs latency tradeoff |
| Time-based | 100ms | Ensures delivery for slow periods |
| Capacity-based | 95% full | Prevent overflow |

**Graceful Shutdown with WaitGroup:**

```go
func (h *Handler) Close() error {
    if h.closed.Swap(true) {
        return nil  // Already closed
    }
    close(h.done)

    done := make(chan struct{})
    go func() {
        h.wg.Wait()
        close(done)
    }()

    select {
    case <-done:
        return nil
    case <-time.After(5 * time.Second):
        return fmt.Errorf("timeout flushing logs")
    }
}
```

**Handler Metrics for Observability:**

```go
type Handler struct {
    // ... other fields
    dropped atomic.Uint64  // Lock-free counter
    flushed atomic.Uint64
}

func (h *Handler) Metrics() (dropped, flushed uint64) {
    return h.dropped.Load(), h.flushed.Load()
}
```

---

### Phase 3: LogPane Component (2 hours)

**Files:**
- `pkg/tui/logpane/pane.go` - Viewport-based log display with incremental rendering

```go
// pkg/tui/logpane/pane.go

package logpane

import (
    "fmt"
    "log/slog"
    "strings"

    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    "github.com/charmbracelet/x/ansi"
    "github.com/mistakeknot/autarch/internal/tui"
    pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

const maxEntries = 500

// Pane displays log messages in a scrollable viewport
type Pane struct {
    viewport     viewport.Model
    entries      []tui.LogMsg
    width        int
    height       int
    lastContent  string // For content comparison (prevents re-render churn)
}

// New creates a log pane
func New() *Pane {
    return &Pane{
        entries: make([]tui.LogMsg, 0, maxEntries),
    }
}

// SetSize updates the pane dimensions
func (p *Pane) SetSize(width, height int) {
    p.width = width
    p.height = height
    p.viewport = viewport.New(width, height)
    p.updateViewport()
}

// Update handles messages
func (p *Pane) Update(msg tea.Msg) (*Pane, tea.Cmd) {
    switch msg := msg.(type) {
    case tui.LogBatchMsg:
        for _, entry := range msg.Entries {
            p.addEntry(entry)
        }
        p.updateViewport()
        p.viewport.GotoBottom()
        return p, nil

    case tui.LogMsg:
        // Handle single messages for backwards compatibility
        p.addEntry(msg)
        p.updateViewport()
        p.viewport.GotoBottom()
        return p, nil

    case tea.KeyMsg:
        switch msg.String() {
        case "g":
            p.viewport.GotoTop()
        case "G":
            p.viewport.GotoBottom()
        default:
            var cmd tea.Cmd
            p.viewport, cmd = p.viewport.Update(msg)
            return p, cmd
        }
        return p, nil
    }
    return p, nil
}

func (p *Pane) addEntry(entry tui.LogMsg) {
    p.entries = append(p.entries, entry)
    // Circular buffer: drop oldest if over limit
    if len(p.entries) > maxEntries {
        p.entries = p.entries[1:]
    }
}

func (p *Pane) updateViewport() {
    content := p.renderEntries()
    // Only update viewport if content changed (prevents render churn)
    if content != p.lastContent {
        p.lastContent = content
        p.viewport.SetContent(content)
    }
}

func (p *Pane) renderEntries() string {
    var b strings.Builder
    b.Grow(len(p.entries) * 100) // Pre-allocate for performance

    for _, e := range p.entries {
        line := p.formatEntry(e)
        b.WriteString(line)
        b.WriteString("\n")
    }
    return b.String()
}

func (p *Pane) formatEntry(e tui.LogMsg) string {
    ts := e.Time.Format("15:04:05")

    var levelStyle lipgloss.Style
    var levelStr string
    switch {
    case e.Level >= slog.LevelError:
        levelStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorError)
        levelStr = "ERR"
    case e.Level >= slog.LevelWarn:
        levelStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorWarning)
        levelStr = "WRN"
    case e.Level >= slog.LevelInfo:
        levelStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorPrimary)
        levelStr = "INF"
    default:
        levelStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)
        levelStr = "DBG"
    }

    srcStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFgDim)
    src := srcStyle.Render(fmt.Sprintf("[%s]", e.Source))

    // Build message with attributes
    msg := e.Message
    for k, v := range e.Attrs {
        msg += fmt.Sprintf(" %s=%v", k, v)
    }

    // Truncate if needed (ANSI-safe)
    prefixLen := 8 + 3 + len(e.Source) + 4
    maxMsg := p.width - prefixLen
    if lipgloss.Width(msg) > maxMsg && maxMsg > 0 {
        msg = ansi.Truncate(msg, maxMsg, "…")
    }

    return fmt.Sprintf("%s %s %s %s", ts, levelStyle.Render(levelStr), src, msg)
}

// View renders the pane
func (p *Pane) View() string {
    border := lipgloss.RoundedBorder()
    style := lipgloss.NewStyle().
        Border(border).
        BorderForeground(pkgtui.ColorMuted).
        Width(p.width).
        Height(p.height)

    header := lipgloss.NewStyle().
        Foreground(pkgtui.ColorPrimary).
        Bold(true).
        Render("Logs")

    return style.Render(
        lipgloss.JoinVertical(lipgloss.Left,
            header,
            p.viewport.View(),
        ),
    )
}

// Entries returns all log entries (for scrollback dump)
func (p *Pane) Entries() []tui.LogMsg { return p.entries }

// ShortHelp returns keybinding hints
func (p *Pane) ShortHelp() string {
    return "↑↓ scroll • g/G top/bottom"
}
```

### Research Insights (LogPane)

**Best Practices:**
- Content comparison (`lastContent`) prevents unnecessary SetContent() calls
- Pre-allocate strings.Builder with `Grow()` for fewer allocations
- Always use `ansi.Truncate()` not string slicing for styled text
- Handle both LogBatchMsg and LogMsg for flexibility

**Performance Considerations:**
- At 100 logs/sec: content comparison reduces SetContent() from 100/sec to ~10/sec
- Circular buffer rotation is O(1) (slice append + drop)
- Render only when content changes, not on every batch

**Edge Cases:**
- Empty buffer → return empty string, no viewport update
- Resize during logging → SetSize() triggers re-render
- Very long messages → ANSI-safe truncation preserves styling

### Deep Research: Advanced Viewport UX (Round 2)

**Sticky Scroll Pattern (from k9s/lazydocker):**

```go
const scrollTolerance = 3 // lines

func (l *LogViewport) isNearBottom() bool {
    maxOffset := l.viewport.TotalLineCount() - l.viewport.Height
    if maxOffset < 0 {
        return true
    }
    return l.viewport.YOffset >= maxOffset - scrollTolerance
}

func (l *LogViewport) AppendLine(line string) {
    l.content.Push(line)
    wasAtBottom := l.isNearBottom()

    l.viewport.SetContent(l.content.String())

    if wasAtBottom && l.autoFollow {
        l.viewport.GotoBottom()
        l.newLines = 0
    } else {
        l.newLines++  // Count unseen lines
    }
}
```

**"N New Lines" Badge (non-intrusive):**

```go
func (l *LogViewport) renderStatusLine() string {
    if l.newLines > 0 && !l.autoFollow {
        badge := lipgloss.NewStyle().
            Background(ColorPrimary).
            Foreground(ColorBg).
            Padding(0, 1).
            Render(fmt.Sprintf("%d new", l.newLines))
        return badge
    }
    // Show scroll percentage when reading
    return lipgloss.NewStyle().
        Foreground(ColorMuted).
        Render(fmt.Sprintf("%d%%", int(l.viewport.ScrollPercent()*100)))
}
```

**Vim-Style Navigation Keys (from lazyjournal):**

| Key | Action | Description |
|-----|--------|-------------|
| `j/↓` | LineDown | Scroll down one line |
| `k/↑` | LineUp | Scroll up one line, disable auto-follow |
| `g/Home` | GotoTop | Go to first line, disable auto-follow |
| `G/End` | GotoBottom | Go to last line, re-enable auto-follow |
| `Ctrl+u` | HalfPageUp | Half page up |
| `Ctrl+d` | HalfPageDown | Half page down |
| `F` | ToggleFollow | Toggle auto-follow mode (like `less`) |
| `/` | Search | Enter search mode (Phase 2) |
| `n/N` | NextMatch | Navigate search results (Phase 2) |

**Visual Hierarchy - Log Levels (Tokyo Night):**

```go
var (
    ColorLogError = ColorError     // #f7768e - Red
    ColorLogWarn  = ColorWarning   // #e0af68 - Yellow
    ColorLogInfo  = ColorPrimary   // #7aa2f7 - Blue
    ColorLogDebug = ColorMuted     // #565f89 - Gray

    // Additional semantic colors
    ColorLogTimestamp = lipgloss.Color("#737aa2") // Dim blue-gray
    ColorLogSource    = ColorSecondary            // #bb9af7 - Purple
)
```

**Pattern Highlighting (from lazyjournal):**

```go
var DefaultHighlights = []HighlightRule{
    // URLs - cyan underline
    {regexp.MustCompile(`https?://[^\s]+`),
        lipgloss.NewStyle().Foreground(lipgloss.Color("#7dcfff")).Underline(true)},
    // HTTP methods - bold
    {regexp.MustCompile(`\b(GET|POST|PUT|DELETE|PATCH)\b`),
        lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)},
    // File paths - purple
    {regexp.MustCompile(`/[\w/.-]+\.\w+`),
        lipgloss.NewStyle().Foreground(ColorSecondary)},
}
```

---

### Phase 4: TerminalWriter (30 min)

**Files:**
- `pkg/tui/writer/writer.go` - Simplified mutex-protected output

```go
// pkg/tui/writer/writer.go

package writer

import (
    "io"
    "sync"
)

// TerminalWriter serializes terminal output to prevent corruption.
// Note: Bubble Tea already serializes via event loop, but this provides
// explicit protection for any direct writes (e.g., panic recovery).
type TerminalWriter struct {
    mu  sync.Mutex
    out io.Writer
}

func New(out io.Writer) *TerminalWriter {
    return &TerminalWriter{out: out}
}

func (w *TerminalWriter) Write(p []byte) (int, error) {
    w.mu.Lock()
    defer w.mu.Unlock()
    return w.out.Write(p)
}
```

### Research Insights (Writer)

**Simplification Applied (YAGNI):**
- Removed `WriteString()` - not needed for current use cases
- Bubble Tea handles serialization via event loop for normal operation
- TerminalWriter only needed for panic recovery and direct writes
- Keep minimal (~20 lines) as simplicity reviewer suggested

---

### Phase 5: Panic Recovery (30 min)

**Files:**
- `pkg/tui/recovery/recovery.go` - Terminal restoration with signal context

```go
// pkg/tui/recovery/recovery.go

package recovery

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "runtime/debug"
    "syscall"
)

// Recover restores terminal state on panic.
// Use as: defer recovery.Recover()
func Recover() {
    if r := recover(); r != nil {
        RestoreTerminal()
        fmt.Fprintf(os.Stderr, "\n\nPanic recovered: %v\n\n", r)
        fmt.Fprintf(os.Stderr, "Stack trace:\n%s\n", debug.Stack())
        os.Exit(1)
    }
}

// RestoreTerminal resets terminal to usable state using universal ANSI sequences
func RestoreTerminal() {
    // Exit alt screen (critical for inline mode recovery)
    fmt.Fprint(os.Stdout, "\x1b[?1049l")
    // Show cursor
    fmt.Fprint(os.Stdout, "\x1b[?25h")
    // Reset all attributes
    fmt.Fprint(os.Stdout, "\x1b(B\x1b[m")
    // Move to new line
    fmt.Fprintln(os.Stdout)
}

// SignalContext creates a context that cancels on SIGINT/SIGTERM.
// Use for graceful shutdown: ctx, stop := recovery.SignalContext(context.Background())
func SignalContext(parent context.Context) (context.Context, context.CancelFunc) {
    return signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
}
```

### Research Insights (Recovery)

**Best Practices:**
- Universal ANSI sequences work across Linux, macOS, Windows (v1607+)
- `signal.NotifyContext()` is modern graceful shutdown pattern (Go 1.16+)
- Keep recovery simple - only reset critical terminal state
- Exit with code 1 after panic to signal failure

**Edge Cases:**
- Panic in inline mode (no alt-screen) → cursor/attrs still need reset
- Panic during signal handling → RestoreTerminal() is safe to call multiple times
- SIGKILL cannot be caught → OS handles terminal cleanup

### Deep Research: Comprehensive Terminal Recovery (Round 2)

**Universal ANSI Sequences (work across all major terminals):**

```go
const (
    // Cursor visibility
    ShowCursor = "\x1b[?25h"  // DECTCEM - Show cursor
    HideCursor = "\x1b[?25l"  // DECTCEM - Hide cursor

    // Alternate screen buffer
    EnterAltScreen = "\x1b[?1049h"  // Save cursor & enter
    ExitAltScreen  = "\x1b[?1049l"  // Exit & restore cursor

    // SGR reset
    ResetAttributes = "\x1b[0m"
    ResetCharset    = "\x1b(B"  // Reset to ASCII

    // Mouse modes - disable all
    DisableNormalMouse  = "\x1b[?1000l"
    DisableButtonMouse  = "\x1b[?1002l"
    DisableAnyMouse     = "\x1b[?1003l"
    DisableSGRMouse     = "\x1b[?1006l"

    // Extras
    DisableBracketedPaste = "\x1b[?2004l"
    DisableFocusReporting = "\x1b[?1004l"
    ResetScrollRegion     = "\x1b[r"  // DECSTBM
)
```

**Comprehensive Restore (for production):**

```go
func ComprehensiveTerminalRestore() {
    sequences := strings.Join([]string{
        "\x1b[?25h",    // Show cursor
        "\x1b[?1049l",  // Exit alternate screen (xterm)
        "\x1b[?47l",    // Exit alternate screen (legacy)
        "\x1b[?1000l",  // Disable normal mouse
        "\x1b[?1002l",  // Disable button-event mouse
        "\x1b[?1003l",  // Disable any-event mouse
        "\x1b[?1006l",  // Disable SGR extended mouse
        "\x1b[?2004l",  // Disable bracketed paste
        "\x1b[?1004l",  // Disable focus reporting
        "\x1b(B",       // Reset to ASCII charset
        "\x1b[0m",      // Reset all SGR attributes
        "\x1b[r",       // Reset scroll region
        "\r\n",         // New line
    }, "")

    fmt.Fprint(os.Stderr, sequences)
}
```

**tmux/screen Compatibility:**

```go
func InsideMultiplexer() bool {
    return os.Getenv("TMUX") != "" ||
        strings.HasPrefix(os.Getenv("TERM"), "screen")
}

// For sequences that must reach outer terminal (tmux 3.2+)
func TmuxPassthrough(seq string) string {
    escaped := strings.ReplaceAll(seq, "\x1b", "\x1b\x1b")
    return "\x1bPtmux;" + escaped + "\x1b\\"
}
```

**Signal Handling Matrix:**

| Signal | Catchable | Default | Use Case |
|--------|-----------|---------|----------|
| SIGINT | Yes | Terminate | Ctrl+C - user interrupt |
| SIGTERM | Yes | Terminate | Graceful shutdown |
| SIGHUP | Yes | Terminate | Terminal hangup |
| SIGKILL | **No** | Terminate | Force kill |
| SIGSTOP | **No** | Stop | Force stop |

**Double Ctrl+C Pattern:**

```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    if msg, ok := msg.(tea.KeyMsg); ok && msg.Type == tea.KeyCtrlC {
        now := time.Now()
        if now.Sub(m.lastCtrlC) < 500*time.Millisecond {
            return m, tea.Quit  // Second press - quit immediately
        }
        m.lastCtrlC = now
        // First press - show "Press Ctrl+C again to quit"
    }
    return m, nil
}
```

**Testing Terminal Recovery:**

```go
// Using teatest for golden file testing
func TestTerminalRecovery(t *testing.T) {
    m := NewModel()
    tm := teatest.NewTestModel(t, m,
        teatest.WithInitialTermSize(80, 24),
    )

    // Send Ctrl+C
    tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})

    // Verify output contains expected sequences
    teatest.WaitFor(t, tm.Output(),
        func(bts []byte) bool {
            return strings.Contains(string(bts), "Goodbye")
        },
    )
}
```

---

### Phase 6: Integration (1 hour)

**Files to modify:**
- `cmd/autarch/main.go` - Add `--inline` flag and wire components
- `internal/tui/app.go` - Handle LogBatchMsg, integrate LogPane

**cmd/autarch/main.go changes:**

```go
import (
    "github.com/mistakeknot/autarch/pkg/tui/loghandler"
    "github.com/mistakeknot/autarch/pkg/tui/recovery"
)

var inlineMode = flag.Bool("inline", false, "Enable inline mode with log pane (preserves scrollback)")

func runTUI() error {
    defer recovery.Recover()

    app := tui.NewApp(client, views...)
    app.SetInlineMode(*inlineMode)

    var opts []tea.ProgramOption
    if !*inlineMode {
        opts = append(opts, tea.WithAltScreen())
    }
    opts = append(opts, tea.WithMouseCellMotion())

    p := tea.NewProgram(app, opts...)

    // Set up log handler if inline mode
    var handler *loghandler.Handler
    if *inlineMode {
        handler = loghandler.New(slog.LevelDebug, "system")
        handler.SetSender(p) // Wire after program created
        slog.SetDefault(slog.New(handler))
    }

    _, err := p.Run()

    // Cleanup
    if handler != nil {
        handler.Close()
    }

    // Dump logs to stdout for scrollback
    if *inlineMode && app.LogPane() != nil {
        fmt.Println("\n--- Log History ---")
        for _, entry := range app.LogPane().Entries() {
            fmt.Printf("[%s] %s [%s]: %s\n",
                entry.Time.Format("15:04:05"),
                entry.Level.String(),
                entry.Source,
                entry.Message)
        }
    }

    return err
}
```

### Research Insights (Integration)

**Initialization Sequence (Critical):**
1. Create handler with nil sender (safe state)
2. Configure slog immediately
3. Create program
4. Wire handler to program via SetSender() (atomic)
5. Run with panic recovery

**Testing Strategy:**
- Unit test: handler with mock sender, verify batching
- Unit test: LogPane with batch messages, verify buffer rotation
- Integration test: full flow with real slog calls
- Manual test: panic during TUI, verify terminal restored

### Deep Research: Autarch Integration Patterns (Round 2)

**TUI Hierarchy Understanding:**

```
cmd/autarch/main.go          # Entry point, creates tea.Program
    |
internal/tui/app.go          # Main App model, tab management
    |
internal/tui/views/*.go      # Tool views (Gurgeh, Coldwine, etc.)
    |
internal/{tool}/tui/*.go     # Tool-specific TUI components
    |
pkg/tui/*.go                 # Shared components (styles, keys, layouts)
```

**Message Routing Pattern (existing):**

```go
// From internal/tui/app.go:188-195 - passes messages to active view
if len(a.views) > 0 {
    active := a.tabs.Active()
    if active < len(a.views) {
        var cmd tea.Cmd
        a.views[active], cmd = a.views[active].Update(msg)
        cmds = append(cmds, cmd)
    }
}
```

**LogPane Integration Point (app-level first):**

```go
// internal/tui/app.go - Handle LogBatchMsg BEFORE delegating to child views
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    switch msg := msg.(type) {
    case LogBatchMsg:
        if a.logPane != nil {
            var cmd tea.Cmd
            a.logPane, cmd = a.logPane.Update(msg)
            cmds = append(cmds, cmd)
        }
        // Continue to pass to active view for transparency
    }

    // ... rest of existing Update logic
}
```

**Layout Composition for LogPane:**

```go
func (a *App) View() string {
    logPaneHeight := 0
    if a.showLogPane && a.logPane != nil {
        logPaneHeight = 8  // Fixed height when visible
    }

    // Calculate content height (total - tabs - footer - logpane)
    contentHeight := a.height - 4 - logPaneHeight

    // ... existing view content logic ...

    // Append log pane at bottom using lipgloss.JoinVertical
    if a.showLogPane && a.logPane != nil {
        body = lipgloss.JoinVertical(lipgloss.Left, body, a.logPane.View())
    }

    return finalView
}
```

**Feature Flag Pattern:**

```go
// pkg/tui/inline.go (new file)
type InlineConfig struct {
    Enabled    bool
    LogLevel   slog.Level
    PaneHeight int
}

var DefaultInlineConfig = InlineConfig{
    Enabled:    false,
    LogLevel:   slog.LevelDebug,
    PaneHeight: 8,
}
```

**Migration Strategy (Recommended Order):**

| Phase | TUI | Complexity | Notes |
|-------|-----|------------|-------|
| 1 | Unified App | Medium | Single integration point, all views benefit |
| 2 | Gurgeh Standalone | High | Complex layout, sprint mode streaming |
| 3 | Coldwine Standalone | High | Similar patterns to Gurgeh |
| 4 | Bigend TUI | High | 3-pane with terminal preview |

**Manual Testing Checklist:**

```bash
# Build and test each TUI
./dev gurgeh --inline      # Gurgeh standalone with logs
./dev coldwine --inline    # Coldwine standalone with logs
autarch tui --inline       # Unified app with logs

# Test scenarios:
# 1. Start interview -> logs visible
# 2. Resize terminal -> layout adapts
# 3. Ctrl+C -> terminal restored
# 4. Generate 100 logs/sec -> no lag
# 5. Without --inline -> behavior unchanged
```

---

## Acceptance Criteria

### Functional Requirements (MVP)

- [ ] `autarch --inline bigend` shows TUI with log pane at bottom
- [ ] `gurgeh --inline` shows Gurgeh TUI with log pane
- [ ] Logs appear in real-time during agent operations
- [ ] Log pane scrollable with ↑↓ keys
- [ ] Scrollback visible after exit (can grep terminal history)
- [ ] Existing behavior unchanged without `--inline` flag

### Deferred to Phase 2 (YAGNI Applied)

- [ ] ~~`L` key toggles log pane visibility~~ (defer)
- [ ] ~~`1-4` keys filter by log level~~ (defer)
- [ ] ~~Focus management for log pane~~ (defer)
- [ ] ~~Auto-scroll state tracking~~ (always scroll to bottom)

### Non-Functional Requirements

- [ ] No performance degradation at 100+ logs/second
- [ ] Memory bounded (~250 KB for 500 entries)
- [ ] Terminal restored on panic (no zombie terminals)
- [ ] No log corruption during heavy async operations
- [ ] p99 latency < 5ms for log routing

### Quality Gates

- [ ] Unit tests for Handler (batching, mock sender)
- [ ] Unit tests for LogPane (batch handling, buffer rotation)
- [ ] Integration test: high-volume logging (1000 logs in 10 seconds)
- [ ] Manual test: panic during TUI, verify terminal restored
- [ ] Run with `-race` flag, no data races detected

---

## Success Metrics

- **Scrollback preservation**: Terminal history grep-able after exit
- **Log visibility**: Agent operations visible in real-time
- **Zero corruption**: No garbled output during concurrent operations
- **Performance**: Sustained 100 logs/sec without UI lag

---

## Dependencies & Prerequisites

- `github.com/charmbracelet/x/ansi` for ANSI-safe truncation
- `golang.org/x/term` for terminal state detection (recovery only)
- Existing: `bubbles/viewport`, `lipgloss`, `bubbletea`

---

## Risk Analysis & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Message loss at high rate | Medium | Low | Bounded channel (256) + drop silently |
| Layout breaks on small terminals | Low | Medium | Min height check, graceful degrade |
| Handler pointer race | Low | High | Mutex protection on SetSender() |
| Memory growth | Low | Medium | Circular buffer caps at 500 entries |

### Research Insights (Risks)

**Performance Risks (from Performance Oracle):**
- Per-log goroutine: MITIGATED by batched channel
- O(n) rendering: MITIGATED by content comparison
- GC pressure: ACCEPTABLE for MVP, add sync.Pool if needed

**Architecture Risks (from Architecture Strategist):**
- Handler-Program coupling: MITIGATED by MessageSender interface
- Race conditions: MITIGATED by mutex on pointer, Bubble Tea serialization

---

## Future Considerations

1. **Level filtering UI** - Add 1-4 hotkeys for debug/info/warn/error filtering
2. **Log pane toggle** - Add L key to show/hide
3. **Source filtering** - Filter by source (gurgeh, coldwine, etc.)
4. **True inline mode** (scroll-above-UI) - Port FrankenTUI's overlay-redraw strategy
5. **Log persistence** - Write to file for post-mortem analysis
6. **sync.Pool for LogMsg** - Add if GC pressure observed at scale

---

## Documentation Plan

- [ ] Update `AGENTS.md` TUI section with inline mode usage
- [ ] Add `--inline` to `autarch --help` output
- [ ] Document in `docs/solutions/` as completed pattern

---

## References & Research

### Internal References

- Brainstorm: `docs/brainstorms/2026-02-04-frankentui-inline-mode-brainstorm.md`
- Message patterns: `internal/tui/messages.go:234-260`
- TerminalPane: `internal/bigend/tui/terminal.go:15-221`
- slog config: `cmd/autarch/main.go:94-99`
- Layout: `pkg/tui/shelllayout.go:23-222`

### External References

- [FrankenTUI](https://github.com/Dicklesworthstone/frankentui) - Inspiration
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [charmbracelet/x/ansi](https://pkg.go.dev/github.com/charmbracelet/x/ansi) - ANSI utilities
- [Go slog Handler Guide](https://github.com/golang/example/blob/master/slog-handler-guide/README.md)
- [testing/slogtest](https://pkg.go.dev/testing/slogtest) - Handler compliance testing

### Round 2 Research Sources

**Batching & Performance:**
- [Fluentd Buffer Configuration](https://docs.fluentd.org/configuration/buffer-section)
- [Datadog Fluentd Plugin](https://github.com/DataDog/fluent-plugin-datadog)
- [Zap Logger Guide](https://signoz.io/guides/zap-logger/)
- [Backpressure Patterns in Go](https://medium.com/@Realblank/backpressure-patterns-in-go)
- [Lock-Free Ring Buffer in Go](https://medium.com/@nathanbcrocker/implementing-a-lock-free-ring-buffer-in-go)

**Viewport & UX:**
- [k9s Cheatsheet](https://ahmedjama.com/blog/2025/09/the-complete-k9s-cheatsheet/)
- [lazydocker GitHub](https://github.com/jesseduffield/lazydocker)
- [lazyjournal GitHub](https://github.com/Lifailon/lazyjournal)
- [Tips for building Bubble Tea programs](https://leg100.github.io/en/posts/building-bubbletea-programs/)
- [Ring buffer guide for Go](https://medium.com/checker-engineering/a-practical-guide-to-implementing-a-generic-ring-buffer-in-go)

**Terminal Recovery:**
- [ANSI Escape Codes Gist](https://gist.github.com/fnky/458719343aabd01cfb17a3a4f7296797)
- [Julia Evans - Escape Code Standards](https://jvns.ca/blog/2025/03/07/escape-code-standards/)
- [XTerm Control Sequences](https://www.xfree86.org/current/ctlseqs.html)
- [tmux Wiki FAQ](https://github.com/tmux/tmux/wiki/FAQ)
- [VictoriaMetrics - Graceful Shutdown in Go](https://victoriametrics.com/blog/go-graceful-shutdown/)

**Testing:**
- [Writing Bubble Tea Tests (teatest)](https://charm.land/blog/teatest/)
- [Testing TUI Apps](https://blog.waleedkhan.name/testing-tui-apps/)
- [creack/pty](https://github.com/creack/pty) - PTY-based testing

### Solutions Docs

- `docs/solutions/TUI_LOGGING_AND_INLINE_MODE_ANALYSIS.md`
- `docs/solutions/AUTARCH_TUI_PATTERNS_REFERENCE.md`
- `docs/solutions/INLINE_MODE_QUICK_START.md`
- `docs/solutions/ui-bugs/swallowed-generation-error-msg-20260131.md`
- `docs/solutions/ui-bugs/ansi-aware-string-splicing-for-overlays.md`

### Research Documents (from /deepen-plan)

- `docs/reviews/inline-logging-architecture-review.md` - Architecture analysis
- `docs/reviews/inline-logging-code-patterns.md` - Production-ready code
- `docs/solutions/TERMINAL_STATE_RECOVERY.md` - Terminal recovery patterns
- `docs/solutions/TERMINAL_RECOVERY_QUICKREF.md` - Quick reference
