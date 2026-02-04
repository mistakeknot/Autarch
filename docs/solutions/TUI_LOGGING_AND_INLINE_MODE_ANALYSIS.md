# TUI Logging & Inline Mode Analysis

**Date:** February 4, 2026
**Context:** Analysis of existing TUI patterns to support inline logging and centralized terminal writing in Autarch

---

## 1. Current Logging Patterns

### slog Usage (Suppressed in TUI Mode)

**Location:** `cmd/bigend/main.go:41-48`, `cmd/autarch/main.go:95-99`

```go
// Existing pattern - logging is suppressed in TUI mode
if *tuiMode {
    logLevel = slog.LevelError
}
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: logLevel,
}))
slog.SetDefault(logger)
```

**Key Finding:**
- Logging is intentionally suppressed in TUI mode to avoid interfering with Bubble Tea's alternate screen
- Default level is `LevelError` in TUI, `LevelInfo` in web/daemon modes
- No custom handlers exist for capturing logs during TUI sessions

### No Custom Log Router
- No existing patterns for routing logs to alternative destinations during TUI sessions
- No message-based logging (like `LogMsg` or `OutputMsg`) in Bubble Tea message system
- Each tool writes directly to `io.Writer` when not in TUI (coldwine: `cmd.OutOrStdout()`)

---

## 2. Terminal Cleanup & Teardown

### Bubble Tea Alt Screen Management

**Pattern:** `tea.WithAltScreen()` in all TUI entry points

Locations:
- `cmd/bigend/main.go:123` - `runTUI()` function
- `cmd/autarch/main.go` - `tui.Run()` wrapper
- `cmd/testui/main.go` - Multiple test programs
- `internal/tui/app.go:366` - Unified app launcher

```go
func Run(client *autarch.Client, views ...View) error {
    app := NewApp(client, views...)
    p := tea.NewProgram(app, tea.WithAltScreen())
    _, err := p.Run()
    return err
}
```

**Cleanup Pattern:**
- Bubble Tea automatically restores terminal state on `p.Run()` exit
- No explicit cleanup code needed (handled by Bubble Tea)
- `tea.Quit()` message triggers graceful shutdown

**Signal Handling:**
- Cobra/Intermute handle `os.Signal` cleanup (SIGINT, SIGTERM)
- Example: `cmd/autarch/main.go:130-134` uses `defer cleanup()` for Intermute manager

### No Explicit Terminal Restoration
- No custom `RestoreTerminal()` functions found
- No panic recovery with terminal reset
- Would need to be added for robust inline mode support

---

## 3. Existing Inline/Log Routing Attempts

### Direct Writer Passing Pattern (Coldwine)

**Location:** `internal/coldwine/cli/init_flow.go:57-100`

Coldwine uses callback-based progress reporting:

```go
_, err = explore.Run(root, planDir, explore.Options{
    Depth: depth,
    EmitProgress: func(msg string) {
        fmt.Fprintln(cmdOut, msg)  // Direct write to output writer
    },
})
```

**Key Pattern:**
- Functions accept `io.Writer` parameter (`cmdOut`)
- Progress callbacks write directly to writer
- Used in non-TUI context (CLI mode)
- Could inform TUI inline mode design

### No Message-Based Output System
- ❌ No existing `OutputMsg` or `LogMsg` types in messages.go
- ❌ No TUI-friendly log collection pattern
- ❌ No "log pane" or inline output area in existing views

---

## 4. Relationship Between pkg/tui and Tool-Specific TUI

### Shared Components (pkg/tui)

**Files:**
- `pkg/tui/view.go` - Interface definition
- `pkg/tui/styles.go` - Tokyo Night color palette
- `pkg/tui/layout.go`, `shelllayout.go`, `splitlayout.go` - Layout utilities
- `pkg/tui/chatpanel.go`, `docpanel.go` - Reusable components
- `pkg/tui/sidebar.go` - Project/session sidebar
- `pkg/tui/keys.go`, `help.go` - Key binding helpers

**View Interface** (pkg/tui/view.go:8-29):
```go
type View interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (View, tea.Cmd)
    View() string
    Focus() tea.Cmd
    Blur()
    Name() string
    ShortHelp() string
}
```

### Tool-Specific TUI Implementations

Each tool has:
1. **internal/{tool}/tui/** - Tool-specific view components
   - `internal/bigend/tui/model.go`, `terminal.go`, `pane.go`
   - No coldwine/gurgeh/pollard dedicated TUI (use unified views)

2. **internal/tui/views/{tool}.go** - Unified app views
   - `internal/tui/views/bigend.go`, `coldwine.go`, `gurgeh.go`, `pollard.go`
   - All implement `pkg/tui.View` interface

3. **Shared Messages** - `internal/tui/messages.go`
   - `SprintStreamLineMsg` - Agent stream output (line-based)
   - `ScanProgressMsg` - Progress during codebase scans
   - `AgentStreamMsg` - Live output lines (for agent runs)

### Design Pattern: Message-Driven Updates
- Tools emit domain-specific messages (SprintStreamLineMsg, ScanProgressMsg)
- Views handle these messages in `Update()` method
- Final rendering in `View()` method

---

## 5. Bubble Tea Program Configuration

### Entry Point Pattern

**Unified Model** (`internal/tui/app.go:29-48`):
```go
func NewApp(client *autarch.Client, views ...View) *App {
    app := &App{
        client:  client,
        tabs:    NewTabBar(names),
        views:   views,
        palette: NewPalette(),
        keys:    pkgtui.NewCommonKeys(),
        help:    pkgtui.NewHelpOverlay(),
    }
    app.updateCommands()
    return app
}

func Run(client *autarch.Client, views ...View) error {
    app := NewApp(client, views...)
    p := tea.NewProgram(app, tea.WithAltScreen())
    _, err := p.Run()
    return err
}
```

### Tea.Program Configuration
- Only option used: `tea.WithAltScreen()`
- No custom input/output handlers
- No `tea.WithMouseCellMotion()` or filtering
- Could extend with:
  - `tea.WithoutSignalHandler()` - For custom signal handling
  - `tea.WithInput(customReader)` - For intercepting input
  - `tea.WithoutRenderer()` - For headless mode

### Message Types Emitted
From `internal/tui/messages.go`:
- **Stream messages:** `SprintStreamLineMsg`, `AgentStreamMsg`
- **Progress messages:** `ScanProgressMsg`, `GeneratingMsg`
- **State transitions:** `SprintCompleteMsg`, `SpecAcceptedMsg`, `EpicsGeneratedMsg`

---

## 6. Established Patterns Relevant to Inline Mode

### ✅ Writer-Based Output (Coldwine Pattern)
- **Use case:** CLI mode progress reporting
- **Pattern:** `EmitProgress func(string)` callbacks
- **Applicable to inline mode:** Yes, for non-TUI phases

### ✅ Message-Driven Updates (Unified TUI Pattern)
- **Use case:** Async results from agents/scans
- **Pattern:** Typed message structs in `messages.go`
- **Applicable to inline mode:** Yes, extend with logging messages

### ✅ View/Update/Render Cycle
- **Pattern:** Each view implements `Update()` and `View()`
- **Applicable to inline mode:** Yes, add log output area to views

### ✅ Cleanup via Defer
- **Pattern:** `defer cleanup()` in main
- **Applicable to inline mode:** Yes, wrap tea.Program with cleanup

### ⚠️ Terminal Restoration
- **Current:** Implicit via Bubble Tea
- **Issue:** No explicit recovery from panics
- **Need:** Add signal handlers + panic recovery with `RestoreTerminal()`

---

## 7. Key Findings for Implementation

### Logging Architecture Decision
The codebase already suppresses slog output in TUI mode. For inline mode:
1. **Keep slog suppressed** - Don't compete with TUI rendering
2. **Add message-based capture** - Extend messages.go with `LogMsg` type
3. **Route logs via Tea.Cmd** - Use Bubble Tea's async message system

### Recommended Patterns

#### Pattern A: LogMsg for Inline Output
```go
// Add to internal/tui/messages.go
type LogMsg struct {
    Level   string // "info", "warn", "error", "debug"
    Source  string // "agent", "scan", "system"
    Message string
    Time    time.Time
}
```

#### Pattern B: Custom Log Handler
```go
// New: pkg/tui/loghandler/handler.go
type TUILogHandler struct {
    sendMsg func(tea.Msg) // Bubble Tea message sender
}

func (h *TUILogHandler) Handle(ctx context.Context, r slog.Record) error {
    msg := LogMsg{
        Level:   r.Level.String(),
        Message: r.Message,
        Time:    r.Time,
    }
    h.sendMsg(msg)
    return nil
}
```

#### Pattern C: Inline Log View Component
```go
// Add to pkg/tui/logpane.go (similar to terminal.go in bigend)
type LogPane struct {
    viewport viewport.Model
    logs     []LogEntry
    // ...
}

func (p *LogPane) Update(msg tea.Msg) (*LogPane, tea.Cmd) {
    switch msg := msg.(type) {
    case LogMsg:
        p.logs = append(p.logs, LogEntry{...})
        // ...
    }
}
```

### Terminal Safety
1. **Restore on exit:** Wrap `tea.Program.Run()` with try/finally logic
2. **Signal handlers:** Explicit SIGINT/SIGTERM with terminal reset
3. **Panic recovery:** Defer terminal restoration in main()

---

## 8. Solutions Documentation References

**Relevant solutions in `/docs/solutions/`:**
- Check for "TUI", "logging", "terminal", "inline" patterns
- Reference: `docs/solutions/` directory has 9 documents of solved problems

---

## Summary Table

| Aspect | Current State | Gap | Recommendation |
|--------|---------------|-----|-----------------|
| **Logging in TUI** | Suppressed via slog level | No log capture | Add LogMsg + handler |
| **Terminal Cleanup** | Automatic (Bubble Tea) | No panic recovery | Add signal handler + defer |
| **Inline Output** | Writer callbacks (CLI) | No TUI integration | Add LogPane component |
| **Message System** | Domain-specific types | No log messages | Extend messages.go |
| **Program Config** | WithAltScreen only | Limited flexibility | Consider custom handlers |
| **View Integration** | pkg/tui.View interface | No log pane | Implement as reusable pane |

---

## Implementation Strategy

### Phase 1: Message & Handler
- [ ] Add `LogMsg` to `internal/tui/messages.go`
- [ ] Create `pkg/tui/loghandler.go` with custom slog handler
- [ ] Wire slog to emit messages instead of text output

### Phase 2: Log Pane Component
- [ ] Create `pkg/tui/logpane.go` (model similar to TerminalPane)
- [ ] Add to split layouts (Sidebar + MainPane + LogPane)
- [ ] Test with existing views

### Phase 3: Terminal Safety
- [ ] Add `pkg/tui/recovery.go` with panic recovery
- [ ] Wire into `internal/tui/app.go` main update cycle
- [ ] Add explicit signal handlers to entry points

### Phase 4: Integration
- [ ] Wire LogMsg into all tool views
- [ ] Test with Gurgeh, Coldwine, Pollard agents
- [ ] Document in `docs/tui/INLINE_LOGGING.md`
