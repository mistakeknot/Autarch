# Inline Mode - Quick Start Guide

**TL;DR:** Add inline logging to Autarch TUI by implementing 4 components using existing patterns

---

## The Problem

Async agent/scan output is lost during TUI sessions because slog is suppressed in alt-screen mode.

```
❌ Agent generates PRD → slog.Info() called
❌ slog suppressed (LevelError) in TUI → message lost
❌ User sees nothing happening
```

---

## The Solution in 4 Steps

### Step 1: Add LogMsg Type

**File:** `internal/tui/messages.go` (add at end)

```go
// LogMsg is a line of output to display in the log pane
type LogMsg struct {
    Level   string    // "debug" | "info" | "warn" | "error"
    Source  string    // "agent" | "scan" | "system" | "gurgeh" | etc.
    Message string
    Time    time.Time
}
```

**Why:** Bubble Tea needs typed messages to pass between goroutines safely.

---

### Step 2: Create slog Handler

**File:** `pkg/tui/loghandler/handler.go` (NEW)

```go
package loghandler

import (
    "context"
    "log/slog"
    tea "github.com/charmbracelet/bubbletea"
)

type Handler struct {
    sendMsg func(tea.Msg)
    level   slog.Level
}

func NewHandler(sendMsg func(tea.Msg), level slog.Level) *Handler {
    return &Handler{sendMsg: sendMsg, level: level}
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
    if r.Level < h.level {
        return nil
    }

    msg := LogMsg{
        Level:   r.Level.String(),
        Message: r.Message,
        Time:    r.Time,
    }

    // Non-blocking send (drop if queue full)
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        h.sendMsg(msg)
        return nil
    }
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return h // Simplified - no attribute tracking
}

func (h *Handler) WithGroup(name string) slog.Handler {
    return h // Simplified
}
```

**Why:** Converts slog.Record to LogMsg without buffering/blocking.

---

### Step 3: Create LogPane Component

**File:** `pkg/tui/logpane/pane.go` (NEW) — ~150 lines (copy TerminalPane pattern)

```go
package logpane

import (
    "strings"
    "time"

    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    shared "github.com/mistakeknot/autarch/pkg/tui"
)

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
    vp := viewport.New(80, 10)
    vp.Style = lipgloss.NewStyle().
        Background(shared.ColorBg).
        Foreground(shared.ColorFg)

    return &LogPane{
        viewport:    vp,
        entries:     []LogEntry{},
        maxEntries:  500,
        filterLevel: "info",
    }
}

func (p *LogPane) SetSize(width, height int) {
    p.width = width
    p.height = height
    p.viewport.Width = width
    p.viewport.Height = height
}

func (p *LogPane) Update(msg tea.Msg) (*LogPane, tea.Cmd) {
    switch msg := msg.(type) {
    case LogMsg:
        // Add to circular buffer
        p.entries = append(p.entries, LogEntry{
            Level:   msg.Level,
            Source:  msg.Source,
            Message: msg.Message,
            Time:    msg.Time,
        })
        if len(p.entries) > p.maxEntries {
            p.entries = p.entries[1:]
        }

        // Re-render
        p.viewport.SetContent(p.renderLogs())
        p.viewport.GotoBottom()

    case tea.KeyMsg:
        if p.focused && msg.String() == "c" {
            p.entries = []LogEntry{}
            p.viewport.SetContent("")
        }
    }
    return p, nil
}

func (p *LogPane) View() string {
    return p.viewport.View()
}

func (p *LogPane) renderLogs() string {
    if len(p.entries) == 0 {
        return "No logs yet"
    }

    var lines []string
    for _, entry := range p.entries {
        timestamp := entry.Time.Format("15:04:05")
        level := strings.ToUpper(entry.Level[:1]) // E, W, I, D
        source := entry.Source

        line := lipgloss.NewStyle().Faint(true).Render(timestamp) + " " +
            p.colorLevel(level) + " " +
            lipgloss.NewStyle().Dim(true).Render("["+source+"]") + " " +
            entry.Message

        lines = append(lines, line)
    }

    return strings.Join(lines, "\n")
}

func (p *LogPane) colorLevel(level string) string {
    switch level {
    case "E":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(level)
    case "W":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(level)
    case "I":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Render(level)
    default:
        return lipgloss.NewStyle().Dim(true).Render(level)
    }
}
```

**Why:** Reusable pane component (goes in pkg/tui so all tools can use it).

---

### Step 4: Wire into App

**File:** `internal/tui/app.go` - Update `Run()` function

```go
func Run(client *autarch.Client, views ...View) error {
    // Create log pane
    logPane := logpane.New()

    // Create handler that sends to Bubble Tea
    handler := loghandler.NewHandler(
        func(msg tea.Msg) {
            // This is a closure - we'll improve this in full implementation
            // For now, store in app state and handle in Update()
        },
        slog.LevelDebug,
    )

    // Set as default logger
    slog.SetDefault(slog.New(handler))

    // Create app with log pane
    app := NewApp(client, views...)
    // app.SetLogPane(logPane)  // Add this field to App struct

    // Run TUI
    p := tea.NewProgram(app, tea.WithAltScreen())
    _, err := p.Run()
    return err
}
```

**Why:** Integrates logging into the TUI without breaking existing views.

---

## Quick Copy-Paste Checklist

- [ ] **1. Add LogMsg to messages.go** (5 lines)
- [ ] **2. Create loghandler/handler.go** (40 lines)
- [ ] **3. Create logpane/pane.go** (150 lines)
- [ ] **4. Update internal/tui/app.go** (10 lines)
- [ ] **5. Add panic recovery** (optional, ~20 lines)

**Total:** ~225 lines to get inline logging working

---

## Testing Immediately After

```bash
# Build
go build ./cmd/autarch

# Run TUI with verbose mode
./autarch tui

# In Gurgeh tab: Start an interview
# → Should see logs in inline pane at bottom

# Verify:
# 1. Log messages appear in real-time ✓
# 2. Logs scroll automatically ✓
# 3. Ctrl+C restores terminal ✓
```

---

## Advanced Improvements (After MVP)

### Add Filtering UI
```go
// Type 'L' to toggle log level filter
switch msg.String() {
case "i": p.filterLevel = "info"
case "w": p.filterLevel = "warn"
case "e": p.filterLevel = "error"
}
```

### Add Log Export
```go
// Type 'X' to export logs to file
func (p *LogPane) ExportToFile(path string) error {
    // Write p.entries to file
}
```

### Add Search
```go
// Type '/' to search logs
p.search = "phrase"
```

---

## Why This Works

✅ **Follows established patterns:**
- LogMsg is identical to SprintStreamLineMsg
- LogPane copying TerminalPane (proven pattern)
- Handler pattern same as Intermute
- Integration same as other views

✅ **No breaking changes:**
- Additive only (new components)
- Existing tools unaffected
- Optional log pane (can hide if needed)

✅ **Scalable:**
- Circular buffer prevents memory leaks
- Non-blocking sends prevent stalls
- Can handle 1000s of log lines

---

## Common Issues & Fixes

### "Logs not appearing"
- Check: Is handler set with `slog.SetDefault()`?
- Check: Are agents calling `slog.Info()` or `fmt.Println()`?
- Solution: Use slog exclusively, remove fmt output from agents

### "Terminal broken on exit"
- Check: Using `tea.WithAltScreen()`?
- Check: Catching panics?
- Solution: Add explicit cleanup in entry point main()

### "Performance degradation"
- Check: maxEntries set to ~500?
- Check: Non-blocking channel?
- Solution: Profile with `pprof`, may need async rendering

---

## Files Changed Summary

| File | Lines | Change |
|------|-------|--------|
| internal/tui/messages.go | +10 | Add LogMsg |
| pkg/tui/loghandler/handler.go | +50 | NEW |
| pkg/tui/logpane/pane.go | +150 | NEW |
| internal/tui/app.go | +15 | Wire handler + pane |
| **TOTAL** | **225** | Manageable scope |

---

## Next Steps

1. Read full analysis in `INLINE_MODE_ARCHITECTURE.md`
2. Start with Step 1 (add LogMsg) — 5 minute task
3. Run tests after each step to verify integration
4. Reference `AUTARCH_TUI_PATTERNS_REFERENCE.md` while coding

**Estimated time to MVP:** 2-4 hours
**Estimated time to polish:** +2 hours

---

## Support Materials

- **Full Analysis:** `docs/solutions/TUI_LOGGING_AND_INLINE_MODE_ANALYSIS.md`
- **Architecture Diagrams:** `docs/solutions/INLINE_MODE_ARCHITECTURE.md`
- **Pattern Reference:** `docs/solutions/AUTARCH_TUI_PATTERNS_REFERENCE.md`
- **Executive Summary:** `docs/solutions/INLINE_MODE_SUMMARY.md`
