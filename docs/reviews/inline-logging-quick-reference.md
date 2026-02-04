# Inline Logging Quick Reference

**Review:** `/root/projects/Autarch/docs/reviews/inline-logging-architecture-review.md`

---

## The Good (No Changes Needed)

✓ Message routing architecture is sound
✓ Circular buffer design prevents unbounded growth
✓ Fire-and-forget async pattern reduces latency
✓ No circular dependencies
✓ Respects Bubble Tea event model
✓ Single sink (app.Update) prevents race conditions

---

## The Critical (Must Fix Before Implementation)

### 1. TUIHandler Needs Interface Abstraction

**BEFORE:**
```go
type TUIHandler struct {
    program *tea.Program  // Exposes entire struct
}
```

**AFTER:**
```go
type MessageSender interface {
    Send(msg tea.Msg)
}

type TUIHandler struct {
    mu     sync.Mutex
    sender MessageSender  // Depend on abstraction
}
```

**Why:** Improves testability and decouples from Bubble Tea internals.

---

### 2. Protect Program Pointer with Mutex

**Handler must safely copy pointer before using:**

```go
func (h *TUIHandler) Handle(ctx context.Context, r slog.Record) error {
    h.mu.Lock()
    sender := h.sender  // Copy reference
    h.mu.Unlock()

    if sender == nil {
        return nil  // Gracefully handle early logs
    }

    msg := LogMsg{ ... }
    sender.Send(msg)  // No lock during Send
    return nil
}
```

**Why:** Prevents pointer reassignment race condition.

---

### 3. Panic Recovery at Entry Point

**In main.go or internal/tui/run.go:**

```go
defer func() {
    if r := recover(); r != nil {
        fmt.Fprintf(os.Stderr, "Fatal: %v\n", r)
        os.Exit(1)
    }
}()

p := tea.NewProgram(app, tea.WithAltScreen())
_, err := p.Run()
```

**Why:** Ensures terminal is cleaned up even if app panics.

---

## The Important (Should Fix)

### 4. LogMsg Must Carry Full Context

```go
type LogMsg struct {
    Level     slog.Level
    Message   string
    Timestamp time.Time
    Attrs     map[string]any  // Include structured data
}
```

**Why:** Preserves debugging information for structured logging.

---

### 5. Separate Concerns Clearly

**TUIHandler owns:** Routing to program
**TerminalWriter owns:** Stdout coordination
**LogPane owns:** Display

These should NOT know about each other.

```go
// WRONG - Handler shouldn't touch TerminalWriter
h.terminalWriter.Suppress()

// RIGHT - They're independent
// TerminalWriter manages stdout
// Handler only sends to program
```

---

### 6. Document Single-Threaded Assumption

**Add to LogPane:**

```go
// LogPane is SINGLE-THREADED:
// - Only access from app.Update() and app.View()
// - Both guaranteed serial by Bubble Tea
// - External access requires synchronization
```

**Why:** Prevents future developers from adding unsafe concurrent reads.

---

## The Optional (Nice to Have)

- Log level filtering UI
- Search/grep in log history
- Export logs to file
- Performance metrics (throughput, latency)

---

## Initialization Sequence (Don't Mess Up)

```go
// 1. Create handler with nil sender
handler := NewTUIHandler(nil)

// 2. Configure slog immediately (ok to have nil sender)
slog.SetDefault(slog.New(handler))

// 3. Redirect stdlib log
log.SetOutput(termWriter)

// 4. Create program
program := tea.NewProgram(app, tea.WithAltScreen())

// 5. Wire handler to program BEFORE running
handler.SetSender(program)  // Atomic write

// 6. Run program
program.Run()
```

**Why:** Ensures logs before step 5 are dropped safely, not panicked.

---

## Testing Checklist

- [ ] Unit: Handler thread safety (race detector)
- [ ] Unit: Circular buffer doesn't lose entries until full
- [ ] Unit: TerminalWriter serializes writes
- [ ] Integration: High-volume logging (10k/sec)
- [ ] Integration: Graceful degradation (nil sender)
- [ ] Load: No TUI jank under sustained logging

---

## Risk Summary

| Risk | Level | Mitigation |
|------|-------|-----------|
| Handler pointer race | HIGH | Mutex + copy |
| Panic crashes app | MEDIUM | Defer recovery |
| LogPane unsync read | NONE | Documented single-threaded |
| Message loss | LOW | 256-msg default queue |
| External lib bypass | MEDIUM | Document assumption |

---

## Files Involved

**Create:**
- `pkg/tui/log.go` - LogMsg, TUIHandler, MessageSender
- `pkg/tui/log_pane.go` - Circular buffer
- `pkg/tui/terminal_writer.go` - Stdout coordination

**Modify:**
- `internal/tui/app.go` - Add LogPane to app
- `cmd/autarch/main.go` - Setup logging, panic recovery, --inline flag

