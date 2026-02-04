# Inline Logging - Implementation Code Patterns

Reference implementations for each component. Use these as templates.

---

## 1. MessageSender Interface & LogMsg Type

**File: `/root/projects/Autarch/pkg/tui/log.go`**

```go
package tui

import (
    "log/slog"
    "time"

    tea "github.com/charmbracelet/bubbletea"
)

// MessageSender is the minimal interface for sending Bubble Tea messages.
// This abstraction allows TUIHandler to be tested independently of tea.Program.
type MessageSender interface {
    Send(msg tea.Msg)
}

// LogMsg represents a structured log entry for TUI display.
// It carries all information needed to render without re-querying the handler.
type LogMsg struct {
    Level     slog.Level           // Log level (Debug, Info, Warn, Error)
    Message   string               // Main log message
    Timestamp time.Time            // When the log was generated
    Attrs     map[string]any       // Structured attributes from slog.Record
}

// Verify LogMsg implements tea.Msg interface (it does - empty interface)
var _ tea.Msg = (*LogMsg)(nil)
```

---

## 2. TUIHandler - Thread-Safe slog.Handler

**File: `/root/projects/Autarch/pkg/tui/log.go` (continued)**

```go
import (
    "context"
    "sync"
    "log/slog"
    "time"
)

// TUIHandler is a custom slog.Handler that routes log records to a Bubble Tea program.
//
// THREAD-SAFETY:
// - Handle() is called from any goroutine (slog calls may originate anywhere)
// - Protects the sender pointer with a mutex
// - Copies the pointer before use to minimize lock duration
// - Does NOT hold lock during Send() to avoid blocking callers
//
// INITIALIZATION:
// - Create with nil sender: handler := NewTUIHandler(nil)
// - Wire program later: handler.SetSender(program)
// - This allows slog to be configured before program exists
type TUIHandler struct {
    mu     sync.Mutex      // Protects sender pointer only
    sender MessageSender   // nil = drop logs silently
}

// NewTUIHandler creates a handler without a sender.
// Use SetSender() to wire the program once it's initialized.
func NewTUIHandler(sender MessageSender) *TUIHandler {
    return &TUIHandler{
        sender: sender,
    }
}

// SetSender atomically updates the message sender.
// Safe to call from any goroutine after handler creation.
func (h *TUIHandler) SetSender(sender MessageSender) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.sender = sender
}

// Handle implements slog.Handler.
// It converts slog.Record to LogMsg and sends it to the program asynchronously.
//
// IMPORTANT: This method is called from the caller's goroutine.
// We MUST NOT block here (Send is async, no wait).
func (h *TUIHandler) Handle(ctx context.Context, r slog.Record) error {
    // 1. Copy sender pointer under lock (quick operation)
    h.mu.Lock()
    sender := h.sender
    h.mu.Unlock()

    // 2. If no sender yet, drop log silently (before program ready)
    if sender == nil {
        return nil
    }

    // 3. Extract structured attributes from the record
    attrs := make(map[string]any)
    r.Attrs(func(a slog.Attr) bool {
        attrs[a.Key] = a.Value.Any()
        return true  // Continue iteration
    })

    // 4. Create LogMsg (no lock needed for this operation)
    msg := &LogMsg{
        Level:     r.Level,
        Message:   r.Message,
        Timestamp: r.Time,
        Attrs:     attrs,
    }

    // 5. Send to program (non-blocking; program.Send uses buffered channel)
    sender.Send(msg)

    return nil
}

// WithAttrs returns a new handler with additional attributes.
// Required to fully implement slog.Handler interface.
func (h *TUIHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    // For TUI logging, we don't need to implement attr chaining.
    // Return self or a no-op handler.
    return h
}

// WithGroup returns a new handler with a log group.
// Required to fully implement slog.Handler interface.
func (h *TUIHandler) WithGroup(name string) slog.Handler {
    // For TUI logging, we don't need to implement grouping.
    // Return self or a no-op handler.
    return h
}

// Enabled implements slog.Handler.
// Return true if this level should be processed.
func (h *TUIHandler) Enabled(ctx context.Context, level slog.Level) bool {
    // For TUI display, accept all levels.
    // (Filtering can happen in LogPane rendering if needed)
    return true
}
```

---

## 3. LogPane - Circular Buffer with Viewport

**File: `/root/projects/Autarch/pkg/tui/log_pane.go`**

```go
package tui

import (
    "fmt"
    "strings"
    "time"

    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
    "log/slog"
)

// LogPane displays a scrollable log history.
//
// THREAD-SAFETY:
// - SINGLE-THREADED: Only access from app.Update() and app.View()
// - Both are guaranteed serial by Bubble Tea's event loop
// - Do NOT access from other goroutines (no lock protection)
//
// DESIGN:
// - Fixed 500-entry circular buffer (no unbounded growth)
// - O(1) append with wraparound
// - Memory bounded: ~50KB total
type LogPane struct {
    entries    []*LogMsg        // Fixed-size circular buffer
    head       int              // Current write position (0-499)
    count      int              // Actual entries (0-500)
    viewport   viewport.Model   // Handles scrolling and rendering
    width      int              // Pane width
    height     int              // Pane height
}

// NewLogPane creates a new log pane with preallocated buffer.
func NewLogPane() *LogPane {
    return &LogPane{
        entries:  make([]*LogMsg, 500),  // Preallocate
        viewport: viewport.New(80, 20),
    }
}

// Init initializes the pane.
func (p *LogPane) Init() tea.Cmd {
    return nil
}

// Update handles resize and scroll messages.
func (p *LogPane) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd

    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        p.width = msg.Width
        p.height = msg.Height
        p.viewport.Width = msg.Width
        p.viewport.Height = msg.Height

    case LogMsg:
        // Append new log entry to circular buffer
        p.Append(&msg)
        // Update viewport content to scroll to bottom
        p.viewport.GotoBottom()

    case tea.KeyMsg:
        // Allow scrolling in viewport
        p.viewport, cmd = p.viewport.Update(msg)
    }

    return p, cmd
}

// Append adds a log entry to the circular buffer.
// Overwrites oldest entry if buffer is full.
func (p *LogPane) Append(msg *LogMsg) {
    p.entries[p.head] = msg
    p.head = (p.head + 1) % 500

    if p.count < 500 {
        p.count++
    }
}

// View renders the log pane as a string.
func (p *LogPane) View() string {
    // 1. Build content from entries
    var content strings.Builder

    for i := 0; i < p.count; i++ {
        // Access entries in insertion order
        idx := (p.head - p.count + i) % 500
        if idx < 0 {
            idx += 500
        }

        entry := p.entries[idx]
        if entry == nil {
            continue
        }

        // Format entry for display
        line := p.formatEntry(entry)
        content.WriteString(line)
        content.WriteString("\n")
    }

    // 2. Update viewport with content
    p.viewport.SetContent(content.String())

    // 3. Render viewport (handles scrolling)
    return p.viewport.View()
}

// formatEntry converts a LogMsg to a displayable string.
func (p *LogPane) formatEntry(msg *LogMsg) string {
    // Format: "14:23:45 [INFO] message" + attrs
    timeStr := msg.Timestamp.Format("15:04:05")
    levelStr := p.levelString(msg.Level)

    line := fmt.Sprintf("%s [%-5s] %s", timeStr, levelStr, msg.Message)

    // Append structured attributes if present
    if len(msg.Attrs) > 0 {
        line += " "
        for k, v := range msg.Attrs {
            line += fmt.Sprintf("%s=%v ", k, v)
        }
    }

    return line
}

// levelString returns a short string representation of a log level.
func (p *LogPane) levelString(level slog.Level) string {
    switch level {
    case slog.LevelDebug:
        return "DEBUG"
    case slog.LevelInfo:
        return "INFO"
    case slog.LevelWarn:
        return "WARN"
    case slog.LevelError:
        return "ERROR"
    default:
        return "INFO"
    }
}

// Clear removes all entries from the buffer.
func (p *LogPane) Clear() {
    p.entries = make([]*LogMsg, 500)
    p.head = 0
    p.count = 0
}

// Entries returns a snapshot of current log entries (read-only).
// Safe for external inspection; doesn't modify state.
func (p *LogPane) Entries() []*LogMsg {
    result := make([]*LogMsg, p.count)
    for i := 0; i < p.count; i++ {
        idx := (p.head - p.count + i) % 500
        if idx < 0 {
            idx += 500
        }
        result[i] = p.entries[idx]
    }
    return result
}
```

---

## 4. TerminalWriter - Centralized Stdout Coordination

**File: `/root/projects/Autarch/pkg/tui/terminal_writer.go`**

```go
package tui

import (
    "io"
    "sync"
)

// TerminalWriter coordinates all writes to stdout/stderr.
//
// PURPOSE:
// - External libraries (Intermute, etc.) may write directly to stdout
// - Without coordination, output interleaves with TUI rendering
// - This centralizes all writes behind a single mutex
//
// USAGE:
// - Redirect stdlib log: log.SetOutput(termWriter)
// - Redirect fmt.Println: Requires external lib cooperation (document this)
// - Pass to slog if NOT using TUIHandler for stdout logging
type TerminalWriter struct {
    mu     sync.Mutex
    writer io.Writer
}

// NewTerminalWriter creates a coordinated writer for the given output.
func NewTerminalWriter(writer io.Writer) *TerminalWriter {
    return &TerminalWriter{
        writer: writer,
    }
}

// Write implements io.Writer.
// All writes are serialized by mutex to prevent interleaving.
func (w *TerminalWriter) Write(p []byte) (int, error) {
    w.mu.Lock()
    defer w.mu.Unlock()
    return w.writer.Write(p)
}

// WriteString is a convenience method for string output.
func (w *TerminalWriter) WriteString(s string) (int, error) {
    return w.Write([]byte(s))
}
```

---

## 5. Integration in App

**File: `/root/projects/Autarch/internal/tui/app.go` (modifications)**

```go
package tui

import (
    "log/slog"
    tea "github.com/charmbracelet/bubbletea"
    pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// App is the main application model.
type App struct {
    // ... existing fields ...

    logPane      *pkgtui.LogPane
    showLogPane  bool  // Controlled by --inline flag
}

// Update handles all messages.
func (app *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd

    // Route LogMsg to logPane if visible
    if app.showLogPane {
        if logMsg, ok := msg.(*pkgtui.LogMsg); ok {
            _, cmd = app.logPane.Update(*logMsg)
            return app, cmd
        }
    }

    // ... rest of existing update logic ...

    return app, cmd
}

// View renders the app.
func (app *App) View() string {
    // ... existing content ...

    if app.showLogPane {
        // Render log pane in a corner or pane
        logView := app.logPane.View()
        // Combine with main view (layout specific to your design)
        // This depends on your current layout system
    }

    return ""  // Your existing return
}
```

---

## 6. Main Setup - cmd/autarch/main.go

**Key sections to modify:**

```go
package main

import (
    "fmt"
    "log"
    "log/slog"
    "os"

    tea "github.com/charmbracelet/bubbletea"
    pkgtui "github.com/mistakeknot/autarch/pkg/tui"
    "github.com/mistakeknot/autarch/internal/tui"
)

func tuiCmd() *cobra.Command {
    var (
        port        int
        dataDir     string
        skipOnboard bool
        inline      bool  // ADD THIS
    )

    cmd := &cobra.Command{
        Use:   "tui",
        Short: "Launch unified TUI with Intermute backend",
        RunE: func(cmd *cobra.Command, args []string) error {
            // SETUP: Inline logging if requested
            if inline {
                if err := setupInlineLogging(); err != nil {
                    fmt.Fprintf(os.Stderr, "Warning: inline logging setup failed: %v\n", err)
                }
            } else {
                // Normal logging setup (existing code)
                logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
                    Level: slog.LevelError,
                }))
                slog.SetDefault(logger)
            }

            // ... existing setup code ...

            // Create app
            app := tui.NewUnifiedApp(client)

            if inline {
                app.ShowLogPane = true  // Enable log display
            }

            // ... rest of existing code ...
        },
    }

    cmd.Flags().IntVar(&port, "port", 7338, "Intermute server port")
    cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory (default: ~/.autarch)")
    cmd.Flags().BoolVar(&skipOnboard, "skip-onboard", false, "Skip onboarding and go directly to dashboard")
    cmd.Flags().BoolVar(&inline, "inline", false, "Show inline log pane in TUI")  // ADD THIS

    return cmd
}

// setupInlineLogging configures slog to route through TUI.
// IMPORTANT: Program reference is added later via SetSender().
func setupInlineLogging() error {
    // 1. Create handler (with nil sender initially)
    tuiHandler := pkgtui.NewTUIHandler(nil)

    // 2. Configure slog to use handler
    logger := slog.New(tuiHandler)
    slog.SetDefault(logger)

    // 3. Redirect stdlib log to centralized writer
    termWriter := pkgtui.NewTerminalWriter(os.Stdout)
    log.SetOutput(termWriter)

    // 4. Store handler globally so RunUnified can wire program
    // (or pass via context; this depends on your architecture)
    setTUIHandlerGlobal(tuiHandler)

    return nil
}

// Global reference to handler (or use dependency injection)
var globalTUIHandler *pkgtui.TUIHandler

func setTUIHandlerGlobal(h *pkgtui.TUIHandler) {
    globalTUIHandler = h
}

// RunUnified launches the unified TUI with optional inline logging.
func RunUnified(client *autarch.Client, app *tui.UnifiedApp) error {
    // Panic recovery wrapper
    defer func() {
        if r := recover(); r != nil {
            fmt.Fprintf(os.Stderr, "FATAL PANIC: %v\n", r)
            os.Exit(1)
        }
    }()

    // Create program
    p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())

    // Wire TUI handler to program if inline logging enabled
    if globalTUIHandler != nil {
        globalTUIHandler.SetSender(p)
    }

    // Run
    _, err := p.Run()
    return err
}
```

---

## 7. Tests

**File: `/root/projects/Autarch/pkg/tui/log_test.go`**

```go
package tui

import (
    "context"
    "log/slog"
    "sync"
    "testing"
    "time"
)

func TestTUIHandlerThreadSafety(t *testing.T) {
    // Create mock sender that counts calls
    var count int
    var mu sync.Mutex

    mockSender := &testSender{
        sendFn: func(msg interface{}) {
            mu.Lock()
            count++
            mu.Unlock()
        },
    }

    handler := NewTUIHandler(mockSender)

    // Simulate concurrent logging from 10 goroutines
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                record := slog.Record{
                    Level:   slog.LevelInfo,
                    Message: "test",
                    Time:    time.Now(),
                }
                _ = handler.Handle(context.Background(), record)
            }
        }()
    }

    wg.Wait()

    mu.Lock()
    defer mu.Unlock()
    if count != 1000 {
        t.Fatalf("expected 1000 sends, got %d", count)
    }
}

func TestTUIHandlerGracefulDegradation(t *testing.T) {
    // Create handler without sender
    handler := NewTUIHandler(nil)

    // Should not panic
    record := slog.Record{
        Level:   slog.LevelInfo,
        Message: "test",
        Time:    time.Now(),
    }
    err := handler.Handle(context.Background(), record)

    if err != nil {
        t.Fatalf("expected nil error, got %v", err)
    }
}

func TestLogPaneCircularBuffer(t *testing.T) {
    pane := NewLogPane()

    // Add 600 items (exceeds 500 limit)
    for i := 0; i < 600; i++ {
        pane.Append(&LogMsg{
            Message: "log",
            Level:   slog.LevelInfo,
        })
    }

    // Should have exactly 500
    if pane.count != 500 {
        t.Fatalf("expected 500 entries, got %d", pane.count)
    }

    // First 100 should be gone
    entries := pane.Entries()
    if len(entries) != 500 {
        t.Fatalf("expected 500 entries in snapshot, got %d", len(entries))
    }
}

// testSender is a mock MessageSender for testing
type testSender struct {
    sendFn func(msg interface{})
}

func (s *testSender) Send(msg interface{}) {
    if s.sendFn != nil {
        s.sendFn(msg)
    }
}
```

---

## Key Patterns Summary

1. **Handler:** Mutex protects pointer, not operations
2. **LogMsg:** Immutable after creation
3. **LogPane:** Single-threaded (documented)
4. **Setup:** Handler before program, SetSender after
5. **Panic:** Defer recovery at entry point
6. **Testing:** Mock MessageSender for unit tests

