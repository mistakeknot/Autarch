# Inline Mode Architecture Sketch

**Quick Reference:** How to add inline logging to Autarch TUI without breaking terminal state

---

## Current Flow (TUI Mode)

```
Main Process
├── cmd/{tool}/main.go
│   ├── slog suppressed (LevelError)
│   ├── tea.NewProgram(..., tea.WithAltScreen())
│   └── p.Run() [blocks until Ctrl+C]
│
└── Bubble Tea Event Loop
    ├── Input: User keypresses
    ├── Update: Process messages
    │   └── Agent/scan async callbacks emit messages
    └── View: Render to alternate screen buffer
```

**Issue:** Async output from agents/scans is lost (slog suppressed)

---

## Proposed Flow (Inline Mode)

```
Main Process
├── Setup
│   ├── Initialize LogHandler with message sender
│   ├── Replace slog default with LogHandler
│   └── Add panic recovery with terminal reset
│
├── Tea.Program Creation
│   └── tea.NewProgram(app, tea.WithAltScreen())
│
└── Tea Event Loop
    ├── Input: User keypresses
    ├── Update: Process messages
    │   ├── Agent/scan emit LogMsg
    │   ├── LogMsg routed to Views
    │   └── Views update log state
    └── View: Render main view + inline log pane
        ├── Top: Current tool view (Gurgeh/Coldwine/Pollard)
        └── Bottom: Log pane (scrollable, filtered)
```

---

## Component Changes

### 1. Message Types (internal/tui/messages.go)

**Add:**
```go
type LogMsg struct {
    Level   string    // "debug" | "info" | "warn" | "error"
    Source  string    // "agent" | "scan" | "system" | "coldwine" | etc.
    Message string
    Time    time.Time
}

type LogFilterToggleMsg struct {
    Level string // Filter by level
}

type LogClearMsg struct{}
```

---

### 2. Log Handler (pkg/tui/loghandler/)

**New File:** `pkg/tui/loghandler/handler.go`

```go
package loghandler

import (
    "context"
    "log/slog"
    tea "github.com/charmbracelet/bubbletea"
)

type Handler struct {
    sendMsg func(tea.Msg) // Bubble Tea message queue
    level   slog.Level
}

func NewHandler(sendMsg func(tea.Msg), level slog.Level) *Handler {
    return &Handler{sendMsg: sendMsg, level: level}
}

// Handle implements slog.Handler interface
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
    if r.Level < h.level {
        return nil
    }

    msg := LogMsg{
        Level:   r.Level.String(),
        Source:  extractSource(r),  // From context or logger
        Message: r.Message,
        Time:    r.Time,
    }

    // Non-blocking send to Bubble Tea
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        h.sendMsg(msg)
        return nil
    }
}
```

---

### 3. Log Pane Component (pkg/tui/logpane/)

**New File:** `pkg/tui/logpane/pane.go`

```go
package logpane

type LogEntry struct {
    Level   string
    Source  string
    Message string
    Time    time.Time
}

type LogPane struct {
    viewport    viewport.Model
    entries     []LogEntry
    maxEntries  int
    filterLevel string
    width       int
    height      int
    focused     bool
}

func New() *LogPane {
    return &LogPane{
        maxEntries:  500,    // Circular buffer
        filterLevel: "info", // Default filter
    }
}

func (p *LogPane) Update(msg tea.Msg) (*LogPane, tea.Cmd) {
    switch msg := msg.(type) {
    case LogMsg:
        if p.shouldShow(msg) {
            p.entries = append(p.entries, LogEntry{
                Level:   msg.Level,
                Source:  msg.Source,
                Message: msg.Message,
                Time:    msg.Time,
            })
            // Keep circular buffer
            if len(p.entries) > p.maxEntries {
                p.entries = p.entries[1:]
            }
            p.updateViewport()
        }
    case tea.KeyMsg:
        if p.focused {
            // Scroll, filter, clear commands
        }
    }
    return p, nil
}

func (p *LogPane) View() string {
    // Render logs with color coding
    // Level-based styling: error=red, warn=yellow, info=white
}
```

---

### 4. Layout Integration (pkg/tui/layouts/)

**Existing:** `shelllayout.go` - Sidebar + main content

**New:** Three-pane split option
```
┌─────────────────────────────────────────┐
│ Sidebar | Main View (Gurgeh/Coldwine)  │
├─────────────────────────────────────────┤
│ Log Pane (filter: info | warn | error)  │
│ • [agent] Generating requirements...    │
│ • [scan] Found 23 files                 │
│ • [system] Sprint phase advanced        │
└─────────────────────────────────────────┘
```

**Option 1: Always-on log pane**
- Fixed height (5-10 lines)
- Collapsible with hotkey
- Used in all tools

**Option 2: Tool-specific**
- Gurgeh/Coldwine: Always shows
- Bigend: Toggle with 'L' key
- Pollard: Inline in detail pane

---

### 5. App Integration (internal/tui/app.go)

**Changes to `Run()` function:**

```go
func Run(client *autarch.Client, views ...View) error {
    // Create log channel and pane
    logMsgChan := make(chan tea.Msg, 100)
    logPane := logpane.New()

    // Create custom slog handler
    handler := loghandler.NewHandler(
        func(msg tea.Msg) {
            select {
            case logMsgChan <- msg:
            default: // Buffer full, drop oldest
            }
        },
        slog.LevelInfo,
    )
    slog.SetDefault(slog.New(handler))

    // Create app with log pane
    app := NewApp(client, views...)
    app.SetLogPane(logPane)

    // Wrap program with terminal recovery
    p := tea.NewProgram(app, tea.WithAltScreen())

    // Run with cleanup
    if _, err := p.Run(); err != nil {
        // Terminal already restored by Bubble Tea
        return err
    }

    return nil
}
```

---

### 6. Terminal Safety (pkg/tui/recovery/)

**New File:** `pkg/tui/recovery/recovery.go`

```go
package recovery

import (
    "fmt"
    "os"
)

// RestoreTerminal resets terminal to normal mode
func RestoreTerminal() {
    // Exit alternate screen mode
    fmt.Fprint(os.Stderr, "\x1b[?1049l")
    // Restore cursor visibility
    fmt.Fprint(os.Stderr, "\x1b[?25h")
}

// Recover wraps main with panic recovery
func Recover() {
    if err := recover(); err != nil {
        RestoreTerminal()
        fmt.Fprintf(os.Stderr, "PANIC: %v\n", err)
        os.Exit(1)
    }
}

// In main:
// defer recovery.Recover()
// ... run TUI ...
```

---

## Signal Handling

**Update entry points** (`cmd/autarch/main.go`, `cmd/bigend/main.go`):

```go
func runTUI() error {
    // Setup signal handling with terminal restore
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    go func() {
        <-sigChan
        // Terminal restore handled by Bubble Tea
        // But explicit signal handling possible
        os.Exit(0)
    }()

    // Run TUI
    return tui.Run(client, views...)
}
```

---

## Data Flow Diagram

```
Agent/Scan Process
    ↓
Call slog.Info("...")
    ↓
LogHandler.Handle()
    ↓
Create LogMsg
    ↓
Send to tea.Cmd channel
    ↓
Tea.Program.Update() processes LogMsg
    ↓
LogPane.Update(LogMsg) appends entry
    ↓
App.View() renders:
    ├─ Sidebar
    ├─ Tool view (Gurgeh/Coldwine)
    └─ LogPane (bottom strip)
    ↓
Terminal display
```

---

## File Changes Summary

| File | Change | Rationale |
|------|--------|-----------|
| `internal/tui/messages.go` | Add LogMsg | Standard message pattern |
| `pkg/tui/loghandler/handler.go` | NEW | Route slog → Bubble Tea |
| `pkg/tui/logpane/pane.go` | NEW | Display logs inline |
| `pkg/tui/recovery/recovery.go` | NEW | Terminal safety net |
| `internal/tui/app.go` | Update Run() | Wire LogPane, handlers |
| `pkg/tui/layout.go` | Add 3-pane option | Support log pane layout |
| `cmd/autarch/main.go` | Update tuiCmd() | Add signal handlers |
| `cmd/bigend/main.go` | Update runTUI() | Add signal handlers |

---

## Testing Strategy

### Unit Tests
- [ ] LogHandler passes slog.Record → LogMsg conversion
- [ ] LogPane filters by level and source
- [ ] Circular buffer doesn't grow unbounded

### Integration Tests
- [ ] Gurgeh agent output appears in log pane
- [ ] Coldwine scan progress shows inline
- [ ] Log pane scrolling doesn't interfere with main view
- [ ] Terminal properly restored on Ctrl+C

### Manual Testing
1. Run `./dev gurgeh` → Start interview → Check log pane
2. Run `./dev coldwine init` → Verify progress inline
3. Ctrl+C mid-interview → Verify terminal usable after exit
4. Simulate panic → Verify terminal restoration works

---

## Rollout Plan

### Phase 1: Infrastructure (Week 1)
- Implement LogMsg + LogHandler
- Add LogPane component
- Wire into app.go without breaking existing views

### Phase 2: Testing (Week 2)
- Unit tests for handler + pane
- Integration tests with Gurgeh
- Manual testing and debugging

### Phase 3: Hardening (Week 3)
- Signal handlers + panic recovery
- Performance profiling (circular buffer impact)
- Documentation

### Phase 4: Polish (Week 4)
- Styling (colors, icons for levels)
- Filtering UI
- Help text updates

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|-----------|
| **Message buffer overflow** | Lost logs | Circular buffer + drop old entries |
| **Panic → broken terminal** | User confusion | Explicit panic recovery + signal handlers |
| **Log pane affects performance** | Laggy TUI | Limit entries, async rendering |
| **Conflicts with existing views** | Breaking changes | Add as optional pane, toggle-able |
| **slog handler errors crash app** | TUI dies | Non-blocking send, error suppression |

---

## Related Documentation

- `docs/tui/SHORTCUTS.md` - Update with log pane hotkeys
- `docs/tui/ARCHITECTURE.md` - Add log subsystem section
- `AGENTS.md` - Update TUI section
