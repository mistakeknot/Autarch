# Autarch TUI Patterns Reference

**Purpose:** Quick lookup for established TUI, logging, and cleanup patterns in the codebase

---

## 1. Entry Point Pattern

### Web/Daemon Mode (Suppress TUI Logs)

**File:** `cmd/bigend/main.go:40-48`

```go
logLevel := slog.LevelInfo
if *tuiMode {
    logLevel = slog.LevelError  // Suppress in TUI to avoid alt screen noise
}
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: logLevel,
}))
slog.SetDefault(logger)
```

**Key Pattern:**
- slog is the standard logger
- TUI mode sets level to `LevelError` (only errors shown)
- Web/daemon modes keep full `LevelInfo`

**Where Used:**
- `cmd/bigend/main.go` (bigend entry)
- `cmd/autarch/main.go:95-99` (unified TUI entry)

---

## 2. TUI Program Initialization

### Standard Pattern with Alt Screen

**File:** `cmd/bigend/main.go:121-128` and `internal/tui/app.go:364-369`

```go
func runTUI(agg *aggregator.Aggregator) {
    m := tui.New(agg, buildInfoString())
    p := tea.NewProgram(m, tea.WithAltScreen())

    if _, err := p.Run(); err != nil {
        fmt.Printf("Error running TUI: %v\n", err)
        os.Exit(1)
    }
}
```

**Universal Pattern:**
```go
p := tea.NewProgram(app, tea.WithAltScreen())
_, err := p.Run()
```

**Configuration Options Available (not used):**
- `tea.WithoutSignalHandler()` - Disable default signal handling
- `tea.WithInput(r io.Reader)` - Custom input reader
- `tea.WithMouseCellMotion()` - Mouse support
- `tea.WithoutRenderer()` - Headless mode

---

## 3. Cleanup & Defer Pattern

### Intermute Manager Cleanup

**File:** `cmd/autarch/main.go:130-134`

```go
cleanup, err := mgr.EnsureRunning(context.Background())
if err != nil {
    return fmt.Errorf("failed to ensure intermute running: %w", err)
}
defer cleanup()
```

**Pattern:**
- Functions return `(stop func(), error)`
- Caller defers the stop function
- Ensures cleanup even on error or panic

**Terminal State:**
- Bubble Tea automatically restores terminal on exit
- No explicit `RestoreTerminal()` calls needed
- BUT: No panic recovery - would need explicit handling

---

## 4. View Interface (pkg/tui/view.go)

### All TUI Views Must Implement

**Location:** `pkg/tui/view.go:8-29`

```go
type View interface {
    // Init initializes the view and returns any initial commands
    Init() tea.Cmd

    // Update handles messages and returns the updated view and any commands
    Update(msg tea.Msg) (View, tea.Cmd)

    // View renders the view as a string
    View() string

    // Focus is called when this view becomes the active tab
    Focus() tea.Cmd

    // Blur is called when this view is no longer the active tab
    Blur()

    // Name returns the view name for display in the tab bar
    Name() string

    // ShortHelp returns keybinding hints for the footer
    ShortHelp() string
}
```

**Key Pattern:**
- All views are immutable (return new View on Update)
- `Focus()/Blur()` for tab switching
- `Init()` runs once at startup
- Enables composition in `App` struct

---

## 5. Async Message Pattern

### Domain Messages (internal/tui/messages.go)

**Stream Messages:**
```go
// SprintStreamLineMsg carries a streaming agent response chunk.
type SprintStreamLineMsg struct {
    Content string
}

// AgentStreamMsg reports a live output line from an agent run.
type AgentStreamMsg struct {
    Line string
}
```

**Progress Messages:**
```go
// ScanProgressMsg reports progress during codebase scanning
type ScanProgressMsg struct {
    Step      string   // Current step name
    Details   string   // What's happening
    Files     []string // Files found/being analyzed (optional)
    AgentLine string   // Live output line from agent (if streaming)
}
```

**Transition Messages:**
```go
// SprintCompleteMsg signals all sprint phases have been accepted.
type SprintCompleteMsg struct{}

// SpecAcceptedMsg is sent when user accepts the spec summary
type SpecAcceptedMsg struct {
    Vision       string
    Users        string
    Problem      string
    // ...
}
```

**Pattern Usage:**
1. Async goroutine does work (agent call, scan)
2. Returns typed message from `tea.Cmd`
3. View receives in `Update(msg tea.Msg)`
4. Type-assert and update local state
5. Re-render in `View()`

---

## 6. TerminalPane Component (bigend/tui/terminal.go)

### Pattern for Displaying Live Terminal Output

**Location:** `internal/bigend/tui/terminal.go:15-220`

**Key Features:**
- Wraps `viewport.Model` for scrolling
- Fetches tmux pane content periodically
- Updates only on content change (optimization)
- Handles focus/blur for keyboard input

**Update Method Pattern:**
```go
func (t *TerminalPane) Update(msg tea.Msg) (*TerminalPane, tea.Cmd) {
    var cmds []tea.Cmd

    switch msg := msg.(type) {
    case terminalContentMsg:
        if msg.session == t.sessionName && msg.err == nil {
            if msg.content != t.content {
                t.content = msg.content
                t.viewport.SetContent(t.formatContent(msg.content))
                t.viewport.GotoBottom()  // Auto-scroll
            }
            t.lastUpdate = time.Now()
        }

    case terminalTickMsg:
        if t.sessionName != "" {
            cmds = append(cmds, t.fetchContent)  // Refresh periodically
        }

    case tea.KeyMsg:
        if t.focused {
            var cmd tea.Cmd
            t.viewport, cmd = t.viewport.Update(msg)  // Delegate scroll keys
            cmds = append(cmds, cmd)
        }
    }

    return t, tea.Batch(cmds...)
}
```

**Applicable to Log Pane:**
- Similar viewport-based scrolling
- Filter/clear commands instead of session selection
- Append-only log entries instead of fetching content

---

## 7. Writer-Based Progress (Coldwine)

### Callback Pattern for Non-TUI Output

**Location:** `internal/coldwine/cli/init_flow.go:76-81`

```go
_, err = explore.Run(root, planDir, explore.Options{
    Depth: depth,
    EmitProgress: func(msg string) {
        fmt.Fprintln(cmdOut, msg)  // Direct write to output
    },
})
```

**Pattern:**
- Functions accept `io.Writer` parameter
- Callbacks write directly to it
- Used in CLI mode (not TUI)
- Progress isn't buffered - immediate output

**For Inline Mode:**
- Instead of immediate write, emit `tea.Cmd` with message
- Decouple progress from output mechanism
- Allows both CLI and TUI modes from same code

---

## 8. Unified App Structure (internal/tui/app.go)

### Tab-Based Multi-Tool View

**Location:** `internal/tui/app.go:29-48`

```go
type App struct {
    client  *autarch.Client
    tabs    *TabBar           // Tab navigation
    views   []View            // Tool views (Bigend, Gurgeh, Coldwine, Pollard)
    palette *Palette          // Command palette
    width   int
    height  int
    err     error
    keys    pkgtui.CommonKeys // Shared key bindings
    help    pkgtui.HelpOverlay
}

func NewApp(client *autarch.Client, views ...View) *App {
    app := &App{
        client:  client,
        tabs:    NewTabBar(names),
        views:   views,
        palette: NewPalette(),
        keys:    pkgtui.NewCommonKeys(),
        help:    pkgtui.NewHelpOverlay(),
    }
    app.updateCommands()  // Collect commands from all views
    return app
}
```

**Run Function Pattern:**
```go
func Run(client *autarch.Client, views ...View) error {
    app := NewApp(client, views...)
    p := tea.NewProgram(app, tea.WithAltScreen())
    _, err := p.Run()
    return err
}
```

**Extension Point for Inline Mode:**
- Add `logPane *logpane.LogPane` field
- Wire LogMsg into App.Update()
- Include in App.View() rendering

---

## 9. Shared TUI Styling (pkg/tui/)

### Tokyo Night Color Palette

**File:** `pkg/tui/colors.go`, `pkg/tui/styles.go`

**Shared Components:**
- `TitleStyle`, `SubtitleStyle`, `LabelStyle`
- `SelectedStyle`, `UnselectedStyle`
- `PanelStyle`, `PaneFocusedStyle`, `PaneUnfocusedStyle`
- `TabStyle`, `ActiveTabStyle`
- `HelpKeyStyle`, `HelpDescStyle`
- `StatusRunning`, `StatusWaiting`, `StatusIdle`, `StatusError`

**Functions:**
- `StatusIndicator()` - Renders status indicator
- `AgentBadge()` - Renders agent name badge

**All Tool Views Use:**
- Bigend TUI: `internal/bigend/tui/model.go:24-46` (re-exports)
- Unified views: `internal/tui/app.go` (direct usage)

---

## 10. Layout Components (pkg/tui/)

### Reusable Layouts

| Component | File | Purpose |
|-----------|------|---------|
| **Sidebar** | `pkg/tui/sidebar.go` | Project/session list on left |
| **ShellLayout** | `pkg/tui/shelllayout.go` | Sidebar + main pane |
| **SplitLayout** | `pkg/tui/splitlayout.go` | Two-pane horizontal split |
| **Viewport** | Uses bubbles `viewport.Model` | Scrollable content area |
| **ChatPanel** | `pkg/tui/chatpanel.go` | Chat input/output area |
| **DocPanel** | `pkg/tui/docpanel.go` | Documentation display |

**Pattern:** Compose layouts to build complex UIs
- Sidebar for navigation
- ShellLayout for 2-pane (sidebar + main)
- SplitLayout for content split
- Could extend with LogPane

---

## 11. No Existing Log Routing

### What's Missing

**❌ No LogMsg Type**
- No `LogMsg` or `OutputMsg` in `messages.go`
- Would need to be added for inline logging

**❌ No Custom Log Handler**
- slog writes to stdout as text
- No Bubble Tea integration
- Would block rendering if used in TUI

**❌ No Log Pane Component**
- No reusable log display area
- TerminalPane only for tmux output
- Would need to implement

**❌ No Terminal Recovery**
- No panic handler with `RestoreTerminal()`
- Bubble Tea handles normal exit
- But uncaught panics leave terminal in alt-screen mode

---

## 12. CLAUDE.md Guidance

**From /root/projects/Autarch/CLAUDE.md:**

> Local-only by default: servers bind to loopback; remote/multi-host deferred; non-loopback requires explicit opt-in + auth

**Implications for Inline Mode:**
- Keep slog local to process
- Don't try to send logs to Intermute server
- Focus on in-process TUI rendering

---

## Pattern Quick Reference

| Pattern | Location | Use Case |
|---------|----------|----------|
| **Log Suppression** | `cmd/bigend/main.go` | Prevent alt-screen noise |
| **TUI Initialization** | `internal/tui/app.go` | Standard entry point |
| **Cleanup Defer** | `cmd/autarch/main.go` | Resource management |
| **View Interface** | `pkg/tui/view.go` | Composable views |
| **Async Messages** | `internal/tui/messages.go` | Async results |
| **Component Template** | `internal/bigend/tui/terminal.go` | Reusable pane pattern |
| **Progress Callbacks** | `internal/coldwine/cli/init_flow.go` | CLI progress reporting |
| **App Structure** | `internal/tui/app.go` | Multi-tool layout |
| **Shared Styling** | `pkg/tui/styles.go` | Consistent colors |
| **Reusable Layout** | `pkg/tui/shelllayout.go` | Sidebar + content |

---

## Implementation Checklist for Inline Logging

### Must Have
- [ ] Add `LogMsg` to `internal/tui/messages.go`
- [ ] Create `pkg/tui/loghandler/handler.go` with slog.Handler implementation
- [ ] Create `pkg/tui/logpane/pane.go` with viewport + filtering
- [ ] Wire into `internal/tui/app.go` Run() function
- [ ] Add panic recovery to main entry points

### Nice to Have
- [ ] Add log level filtering UI
- [ ] Add log search/grep functionality
- [ ] Add log export to file
- [ ] Performance profiling for circular buffer
- [ ] Colored log levels (red=error, yellow=warn, white=info)

### Documentation
- [ ] Update `docs/tui/SHORTCUTS.md` with log pane hotkeys
- [ ] Add `docs/tui/INLINE_LOGGING.md` guide
- [ ] Update `AGENTS.md` TUI section

---

## Files Modified Count

| Category | Count | Examples |
|----------|-------|----------|
| **New Files** | 4 | loghandler, logpane, recovery, |
| **Modified Files** | 6 | messages, app, main entry points |
| **Documentation** | 3 | guides + reference |
| **Total** | ~13 | Manageable scope |

---

## Related Reading

- **messages.go:** Domain message patterns (types to emulate)
- **terminal.go:** Pane component template (TerminalPane.Update pattern)
- **view.go:** Interface contract (what all views must implement)
- **init_flow.go:** Writer-based progress (pattern to adapt)
- **app.go:** App structure (where to integrate LogPane)
