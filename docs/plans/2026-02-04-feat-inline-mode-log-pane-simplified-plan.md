---
title: "feat: Add inline mode with log pane (Simplified)"
type: feat
date: 2026-02-04
origin: FrankenTUI investigation + brainstorm + reviewer feedback
estimated_effort: 3-4 hours
risk: low
simplified_from: 2026-02-04-feat-frankentui-inline-mode-log-pane-plan.md
reviewers: DHH, Kieran, Simplicity
---

# feat: Add Inline Mode with Log Pane

> **This is the simplified plan.** The original comprehensive plan is at `docs/plans/2026-02-04-feat-frankentui-inline-mode-log-pane-plan.md` for reference.

## Overview

Add `--inline` flag that shows TUI without alt-screen, with a scrollable log pane at bottom. Logs are visible during execution and preserved in terminal scrollback after exit.

**Core components (~160 lines of code):**
1. `LogMsg`/`LogBatchMsg` types
2. slog Handler with batching
3. LogPane viewport component
4. Terminal recovery on panic
5. Wiring in main.go

---

## Problem Statement

1. **Lost scrollback** - All TUI entry points use `tea.WithAltScreen()`, wiping terminal history
2. **Log corruption** - 41 files with `log.`/`fmt.Print` calls can corrupt TUI during async operations
3. **Debugging difficulty** - Logs suppressed in TUI mode
4. **Agent opacity** - Coldwine/Gurgeh agent output invisible during execution

---

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Activation | `--inline` flag (opt-in) | Default behavior unchanged |
| Interface | Direct `*tea.Program` | No MessageSender abstraction (YAGNI) |
| Batching | Count (10) + time (100ms) | Proven pattern, minimal complexity |
| Buffer | Simple slice, 500 max | O(n) rotation is fine at this scale |
| Terminal recovery | 4 ANSI sequences | Covers 99% of cases |

---

## Implementation (3 Phases)

### Phase 1: Log Routing (1 hour)

**File: `pkg/tui/loghandler.go`**

```go
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

// LogMsg represents a single log entry
type LogMsg struct {
    Level   slog.Level
    Message string
    Time    time.Time
}

// LogBatchMsg contains multiple log entries for efficient routing
type LogBatchMsg struct {
    Entries []LogMsg
}

// LogHandler implements slog.Handler with batched message routing
type LogHandler struct {
    mu        sync.Mutex
    program   *tea.Program
    level     slog.Level
    attrs     []slog.Attr
    groups    []string
    msgChan   chan LogMsg
    done      chan struct{}
    closed    atomic.Bool
}

// NewLogHandler creates a handler that batches logs before sending
func NewLogHandler(level slog.Level) *LogHandler {
    h := &LogHandler{
        level:   level,
        msgChan: make(chan LogMsg, 256),
        done:    make(chan struct{}),
    }
    go h.batchLoop()
    return h
}

// SetProgram wires the handler to a Bubble Tea program
func (h *LogHandler) SetProgram(p *tea.Program) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.program = p
}

func (h *LogHandler) Enabled(_ context.Context, level slog.Level) bool {
    return level >= h.level
}

func (h *LogHandler) Handle(ctx context.Context, r slog.Record) error {
    msg := LogMsg{
        Level:   r.Level,
        Message: r.Message,
        Time:    r.Time,
    }

    // Non-blocking enqueue (drop on overflow)
    select {
    case h.msgChan <- msg:
    default:
    }
    return nil
}

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
                batch = make([]LogMsg, 0, batchSize)
            }
        case <-ticker.C:
            if len(batch) > 0 {
                h.sendBatch(batch)
                batch = make([]LogMsg, 0, batchSize)
            }
        case <-h.done:
            if len(batch) > 0 {
                h.sendBatch(batch)
            }
            return
        }
    }
}

func (h *LogHandler) sendBatch(batch []LogMsg) {
    h.mu.Lock()
    p := h.program
    h.mu.Unlock()

    if p == nil {
        return
    }

    entries := make([]LogMsg, len(batch))
    copy(entries, batch)
    p.Send(LogBatchMsg{Entries: entries})
}

func (h *LogHandler) Close() {
    if h.closed.Swap(true) {
        return
    }
    close(h.done)
}

// WithAttrs returns a new handler with additional attributes
func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    h2 := *h
    h2.attrs = append(slices.Clone(h.attrs), attrs...)
    return &h2
}

// WithGroup returns a new handler with an additional group
func (h *LogHandler) WithGroup(name string) slog.Handler {
    if name == "" {
        return h
    }
    h2 := *h
    h2.groups = append(slices.Clone(h.groups), name)
    return &h2
}
```

**Key points:**
- No MessageSender interface - direct `*tea.Program`
- `atomic.Bool` for idempotent Close()
- Allocate new slice after each batch (no reuse race)
- Correct `WithAttrs`/`WithGroup` returning new handlers

---

### Phase 2: Log Display (1 hour)

**File: `pkg/tui/logpane.go`**

```go
package tui

import (
    "fmt"
    "log/slog"
    "strings"

    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

const maxLogEntries = 500

// LogPane displays log messages in a scrollable viewport
type LogPane struct {
    viewport viewport.Model
    entries  []LogMsg
    width    int
    height   int
}

// NewLogPane creates a log pane
func NewLogPane() *LogPane {
    return &LogPane{
        entries: make([]LogMsg, 0, maxLogEntries),
    }
}

// SetSize updates the pane dimensions
func (p *LogPane) SetSize(width, height int) {
    p.width = width
    p.height = height
    p.viewport = viewport.New(width, height-2) // -2 for border
    p.viewport.MouseWheelEnabled = true
    p.updateContent()
}

// Update handles messages
func (p *LogPane) Update(msg tea.Msg) tea.Cmd {
    switch msg := msg.(type) {
    case LogBatchMsg:
        for _, entry := range msg.Entries {
            p.entries = append(p.entries, entry)
        }
        // Simple circular buffer: drop oldest
        if len(p.entries) > maxLogEntries {
            p.entries = p.entries[len(p.entries)-maxLogEntries:]
        }
        p.updateContent()
        p.viewport.GotoBottom()
        return nil

    case tea.KeyMsg:
        switch msg.String() {
        case "g":
            p.viewport.GotoTop()
        case "G":
            p.viewport.GotoBottom()
        default:
            var cmd tea.Cmd
            p.viewport, cmd = p.viewport.Update(msg)
            return cmd
        }
    }
    return nil
}

func (p *LogPane) updateContent() {
    var b strings.Builder
    for _, e := range p.entries {
        b.WriteString(p.formatEntry(e))
        b.WriteString("\n")
    }
    p.viewport.SetContent(b.String())
}

func (p *LogPane) formatEntry(e LogMsg) string {
    ts := e.Time.Format("15:04:05")

    var levelStr string
    var levelColor lipgloss.Color
    switch {
    case e.Level >= slog.LevelError:
        levelStr, levelColor = "ERR", ColorError
    case e.Level >= slog.LevelWarn:
        levelStr, levelColor = "WRN", ColorWarning
    case e.Level >= slog.LevelInfo:
        levelStr, levelColor = "INF", ColorPrimary
    default:
        levelStr, levelColor = "DBG", ColorMuted
    }

    level := lipgloss.NewStyle().Foreground(levelColor).Render(levelStr)
    return fmt.Sprintf("%s %s %s", ts, level, e.Message)
}

// View renders the pane
func (p *LogPane) View() string {
    style := lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(ColorMuted).
        Width(p.width).
        Height(p.height)

    header := lipgloss.NewStyle().
        Foreground(ColorPrimary).
        Bold(true).
        Render("Logs")

    return style.Render(
        lipgloss.JoinVertical(lipgloss.Left, header, p.viewport.View()),
    )
}

// Entries returns all log entries (for scrollback dump on exit)
func (p *LogPane) Entries() []LogMsg { return p.entries }
```

**Key points:**
- Simple slice rotation (O(n) is fine for 500 entries)
- No content comparison optimization (keep it simple)
- Minimal formatting: timestamp + level + message
- `g`/`G` for top/bottom navigation, arrow keys delegated to viewport

---

### Phase 3: Integration (1 hour)

**File: `cmd/autarch/main.go` (modifications)**

```go
import (
    "fmt"
    "os"
    "runtime/debug"

    pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

var inlineMode = flag.Bool("inline", false, "Enable inline mode with log pane (preserves scrollback)")

func runTUI() error {
    // Panic recovery with terminal restore
    defer func() {
        if r := recover(); r != nil {
            restoreTerminal()
            fmt.Fprintf(os.Stderr, "\n\nPanic: %v\n\n", r)
            debug.PrintStack()
            os.Exit(1)
        }
    }()

    app := tui.NewApp(client, views...)
    app.SetInlineMode(*inlineMode)

    var opts []tea.ProgramOption
    if !*inlineMode {
        opts = append(opts, tea.WithAltScreen())
    }
    opts = append(opts, tea.WithMouseCellMotion())

    p := tea.NewProgram(app, opts...)

    // Set up log handler if inline mode
    var handler *pkgtui.LogHandler
    if *inlineMode {
        handler = pkgtui.NewLogHandler(slog.LevelDebug)
        handler.SetProgram(p)
        slog.SetDefault(slog.New(handler))
    }

    _, err := p.Run()

    // Cleanup
    if handler != nil {
        handler.Close()
    }

    // Dump logs for scrollback
    if *inlineMode && app.LogPane() != nil {
        fmt.Println("\n--- Log History ---")
        for _, e := range app.LogPane().Entries() {
            fmt.Printf("[%s] %s: %s\n", e.Time.Format("15:04:05"), e.Level, e.Message)
        }
    }

    return err
}

// restoreTerminal resets terminal to usable state
func restoreTerminal() {
    fmt.Fprint(os.Stdout, "\x1b[?1049l") // Exit alt screen
    fmt.Fprint(os.Stdout, "\x1b[?25h")   // Show cursor
    fmt.Fprint(os.Stdout, "\x1b(B\x1b[m") // Reset attributes
    fmt.Fprintln(os.Stdout)
}
```

**File: `internal/tui/app.go` (modifications)**

```go
type App struct {
    // ... existing fields
    logPane     *pkgtui.LogPane
    showLogPane bool
}

func (a *App) SetInlineMode(enabled bool) {
    a.showLogPane = enabled
    if enabled {
        a.logPane = pkgtui.NewLogPane()
    }
}

func (a *App) LogPane() *pkgtui.LogPane { return a.logPane }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    // Handle window resize for log pane
    if msg, ok := msg.(tea.WindowSizeMsg); ok && a.logPane != nil {
        logHeight := 10 // Fixed height
        a.logPane.SetSize(msg.Width, logHeight)
    }

    // Route log batches to pane
    if _, ok := msg.(pkgtui.LogBatchMsg); ok && a.logPane != nil {
        cmd := a.logPane.Update(msg)
        cmds = append(cmds, cmd)
    }

    // ... existing Update logic ...

    return a, tea.Batch(cmds...)
}

func (a *App) View() string {
    // ... existing view logic ...

    // Append log pane at bottom
    if a.showLogPane && a.logPane != nil {
        content = lipgloss.JoinVertical(lipgloss.Left, content, a.logPane.View())
    }

    return content
}
```

---

## Acceptance Criteria

### MVP (Must Have)

- [x] `autarch --inline tui` shows TUI with log pane at bottom
- [x] Logs appear in real-time during operations
- [x] Log pane scrollable with ↑↓ keys, g/G for top/bottom
- [x] Scrollback visible after exit (can grep terminal history)
- [x] Terminal restored on panic
- [x] Existing behavior unchanged without `--inline` flag

### Deferred (YAGNI)

- Level filtering (1-4 keys)
- Log pane toggle (L key)
- Auto-scroll state tracking
- Pattern highlighting
- tmux passthrough sequences
- Handler metrics
- sync.Pool optimization

---

## Testing Strategy

1. **Unit test handler batching:**
   ```go
   func TestLogHandler_Batching(t *testing.T) {
       h := NewLogHandler(slog.LevelDebug)
       // ... send 15 messages, verify 2 batches
   }
   ```

2. **Unit test pane buffer rotation:**
   ```go
   func TestLogPane_BufferRotation(t *testing.T) {
       p := NewLogPane()
       // ... add 600 entries, verify 500 retained
   }
   ```

3. **Integration test:** Run with `-race` flag

4. **Manual test:**
   - Start interview → logs visible
   - Ctrl+C → terminal restored
   - Without --inline → unchanged behavior

---

## Success Metrics

- Scrollback preservation: Terminal history grep-able after exit
- Log visibility: Operations visible in real-time
- Zero corruption: No garbled output during async operations
- Performance: Sustained 100 logs/sec without lag

---

## References

- Original comprehensive plan: `docs/plans/2026-02-04-feat-frankentui-inline-mode-log-pane-plan.md`
- Brainstorm: `docs/brainstorms/2026-02-04-frankentui-inline-mode-brainstorm.md`
- Existing patterns: `internal/bigend/tui/terminal.go` (TerminalPane)
- Message types: `internal/tui/messages.go`
