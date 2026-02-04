# FrankenTUI Inline Logging - Architectural Review

**Date:** 2026-02-04
**Review Status:** APPROVED WITH RECOMMENDATIONS
**Scope:** Message routing, concurrency safety, integration with Bubble Tea architecture

---

## Executive Summary

The FrankenTUI-inspired inline logging plan is **architecturally sound** with proper consideration for concurrency, message flow, and TUI integration. The design follows established Bubble Tea patterns and respects the current Autarch system architecture. However, several critical implementation details require attention to prevent race conditions and maintain architectural integrity.

**Key Assessment:**
- Message routing architecture: SOUND
- Concurrency safety: PARTIALLY ADDRESSED - requires careful implementation
- Integration points: WELL-DEFINED
- Anti-patterns detected: MINIMAL - one separation-of-concerns concern identified

---

## 1. Architecture Overview

### Current System Context

Autarch operates with a **layered, tool-specific architecture**:

```
Application Layer (cmd/autarch/main.go)
    ↓
Tool Layers (Bigend, Gurgeh, Coldwine, Pollard)
    ↓
Shared Infrastructure (pkg/tui/, pkg/signals/, Intermute)
    ↓
Data Persistence (.gurgeh/, .coldwine/, .pollard/)
```

**Logging Current State:**
- Centralized via `log/slog` with `TextHandler` to `os.Stdout`
- Suppressed in TUI mode (level set to `slog.LevelError`)
- No TUI integration - logs bypass the visual system

### Proposed Inline Mode

The plan introduces a **pull-through architecture**:

```
slog.Info/Debug/Warn/Error
    ↓
TUIHandler (custom slog.Handler)
    ↓
LogMsg (tea.Msg type)
    ↓
tea.Program.Send()
    ↓
App.Update() → LogPane.Update()
    ↓
Circular Buffer (500 entries)
    ↓
Terminal Render
```

---

## 2. Change Assessment

### 2.1 Component Interactions

#### LogMsg Message Type
**Assessment:** CORRECT

This is the appropriate abstraction layer for message passing:

```go
type LogMsg struct {
    Level     slog.Level
    Message   string
    Timestamp time.Time
    Attrs     []slog.Attr
}
```

**Why this works:**
- Implements `tea.Msg` interface (empty interface in Bubble Tea)
- Carries sufficient context for rendering without re-querying
- Immutable after creation (safe for concurrent reads)
- No back-references to handler state (clean separation)

**File Location:** `/root/projects/Autarch/pkg/tui/log.go` (new)

---

#### TUIHandler (slog.Handler)
**Assessment:** CRITICAL - Requires careful implementation

This is the most sensitive component for concurrency:

```go
type TUIHandler struct {
    program *tea.Program    // Reference to running program
    mu      sync.Mutex      // Protects access to program
}

func (h *TUIHandler) Handle(ctx context.Context, r slog.Record) error {
    msg := LogMsg{
        Level:     r.Level,
        Message:   r.Message,
        Timestamp: r.Time,
    }

    h.mu.Lock()
    if h.program != nil {
        h.program.Send(msg)
    }
    h.mu.Unlock()

    return nil
}
```

**Why this pattern is necessary:**

1. **Program Pointer Safety:** `tea.Program` can be reassigned during lifecycle transitions
2. **Nil Guard:** Handler may be configured before program initialization
3. **Lock Scope:** Must be minimal (just around Send) to avoid blocking caller goroutines

**Critical Implementation Requirement:**

The lock MUST use `sync.Mutex`, NOT `sync.RWMutex`:
- `program.Send()` is already thread-safe internally
- Lock exists only to guard the pointer read
- Minimal contention expected (Send is fast)
- RWMutex adds unnecessary complexity

---

#### LogPane (Viewport-based Buffer)
**Assessment:** SOUND

Circular buffer design is appropriate:

```go
type LogPane struct {
    entries    []*LogMsg   // Fixed 500-entry buffer
    head       int         // Current write position
    count      int         // Actual entries (0-500)
    viewport   viewport.Model
}

func (p *LogPane) Append(msg *LogMsg) {
    p.entries[p.head] = msg
    p.head = (p.head + 1) % 500
    if p.count < 500 {
        p.count++
    }
}
```

**Why this works:**
- **No dynamic allocation:** Fixed 500-entry slice prevents GC pressure
- **Predictable performance:** O(1) append with wraparound
- **Memory bounded:** ~50KB overhead per pane (acceptable)
- **Viewport integration:** Lipgloss viewport handles scrolling, ANSI codes

**Recommendation:** Pre-allocate at initialization; avoid nil checks in hot path:

```go
func NewLogPane() *LogPane {
    return &LogPane{
        entries: make([]*LogMsg, 500),
        viewport: viewport.New(0, 0),
    }
}
```

---

#### TerminalWriter (Centralized Output)
**Assessment:** NECESSARY BUT INTRODUCES ARCHITECTURAL BOUNDARY

This is where the plan touches **terminal I/O coordination**:

```go
type TerminalWriter struct {
    mu     sync.Mutex
    stdout io.Writer    // os.Stdout or test double
}

func (w *TerminalWriter) Write(p []byte) (int, error) {
    w.mu.Lock()
    defer w.mu.Unlock()
    return w.stdout.Write(p)
}
```

**Why needed:**
- External libraries (Intermute, Pollard hunters) may write directly to stdout/stderr
- Without coordination, interleaved output corrupts TUI render
- One-writer rule is fundamental to terminal safety

**Architectural Concern - SEPARATION OF CONCERNS VIOLATION:**

The TUIHandler should NOT directly coordinate with TerminalWriter. This introduces **inappropriate intimacy**:

```go
// ANTI-PATTERN - Don't do this:
func (h *TUIHandler) Handle(ctx context.Context, r slog.Record) error {
    msg := LogMsg{ ... }
    h.program.Send(msg)
    // WRONG: Handler now owns knowledge of terminal coordination
    h.terminalWriter.Suppress()
    return nil
}
```

**Correct Pattern - Separate Concerns:**

1. **TUIHandler responsibility:** Route logs to Bubble Tea
2. **TerminalWriter responsibility:** Coordinate stdout access
3. **LogPane responsibility:** Display logs in TUI

These should NOT know about each other.

**Implementation:** Create an initialization sequence:

```go
// In main.go or TUI setup
func setupLogging(program *tea.Program) {
    // 1. Create TerminalWriter (owns stdout)
    termWriter := NewTerminalWriter(os.Stdout)

    // 2. Create TUIHandler (sends to program only)
    handler := NewTUIHandler(program)

    // 3. Chain with TerminalWriter in case fallback needed
    // They remain independent
    logger := slog.New(handler)
    slog.SetDefault(logger)

    // Separately, redirect external writers if needed
    log.SetOutput(termWriter)  // stdlib log
}
```

---

### 2.2 Message Routing Architecture

**Assessment:** SOUND - Follows Bubble Tea conventions

The proposed routing:

```
slog.Info() [ANY goroutine]
    ↓
TUIHandler.Handle() [ANY goroutine - thread-safe]
    ↓
program.Send(LogMsg) [Thread-safe, queued internally]
    ↓
App.Update(msg tea.Msg) [Main event loop - single goroutine]
    ↓
LogPane.Update(msg tea.Msg) [Handled within Update]
    ↓
app.View() [Main event loop - renders new state]
```

**Why this works:**

1. **Multiple sources:** `slog` calls come from all goroutines
2. **Single sink:** `program.Send()` is internally thread-safe (uses channels)
3. **Serialization point:** App.Update runs in main event loop (guaranteed serial)
4. **No back-pressure:** LogMsg is fire-and-forget, handler doesn't wait

**Verification:** Bubble Tea's `program.Send()` uses:
```go
// From charmbracelet/bubbletea
type Program struct {
    msgs chan tea.Msg  // Buffered channel
}

func (p *Program) Send(msg tea.Msg) {
    select {
    case p.msgs <- msg:
    case <-p.done:
    }
}
```

This is safe for concurrent sends from any goroutine.

---

## 3. Compliance Check

### 3.1 SOLID Principles

#### Single Responsibility Principle ✓
- **TUIHandler:** Routes slog records to Bubble Tea (1 responsibility)
- **LogPane:** Displays logs with scrolling (1 responsibility)
- **TerminalWriter:** Coordinates stdout access (1 responsibility)

Each component has a single, well-defined reason to change.

#### Open/Closed Principle ✓
- **Open for extension:** LogMsg can carry additional fields without breaking handlers
- **Closed for modification:** Handler interface is stable; new handler types can be added

#### Liskov Substitution Principle ✓
- TUIHandler implements `slog.Handler` interface faithfully
- Can be substituted with any other slog.Handler implementation
- Return value semantics match contract (nil error = success)

#### Interface Segregation Principle ⚠️ REQUIRES ATTENTION

**Issue:** TUIHandler needs program reference, but `tea.Program` is a large struct:

```go
// CURRENT APPROACH
type TUIHandler struct {
    program *tea.Program  // Exposes entire Program interface
}
```

**Better approach - Use interface abstraction:**

```go
// Define minimal interface
type MessageSender interface {
    Send(msg tea.Msg)
}

// TUIHandler depends only on what it needs
type TUIHandler struct {
    sender MessageSender
}

// tea.Program implements this interface implicitly
func (p *Program) Send(msg tea.Msg) { ... }
```

This decouples TUIHandler from Bubble Tea's full API surface and improves testability.

#### Dependency Inversion Principle ✓
- TUIHandler depends on abstraction (`MessageSender`), not concrete Program
- LogPane depends on abstraction (interface for scrolling), not concrete Viewport

---

### 3.2 Architectural Principles

#### Separation of Concerns ✓ (with caveats)

**Clear boundaries:**

| Component | Input | Output | Side Effects |
|-----------|-------|--------|--------------|
| TUIHandler | slog.Record | LogMsg → program.Send() | None (async) |
| LogPane | LogMsg (via Update) | Rendered string | Updates buffer |
| TerminalWriter | []byte | Written bytes | Stdout access |

**Caution:** Don't let these leak into each other's responsibilities.

#### No Circular Dependencies ✓

```
LogMsg (data type - no imports of handlers)
    ↓
TUIHandler (depends on LogMsg, tea.Program)
    ↓
LogPane (depends on LogMsg)
    ↑
App (depends on LogPane)
    ↓
main (initializes all)
```

No component has a reverse reference. Safe.

#### Cohesion ✓

All three components work toward a single goal: display logs in TUI. They're tightly coupled by design (as they should be for a feature).

---

### 3.3 Bubble Tea Integration

#### Message Flow ✓

The inline mode follows Bubble Tea's event model:

1. **External events** (logs) → converted to `tea.Msg`
2. **Queued** via `program.Send()` (thread-safe buffer)
3. **Serialized** in `app.Update()` (main loop)
4. **Rendered** via `app.View()` (synchronized)

This is the **canonical pattern** for integrating external data sources into Bubble Tea.

#### No Blocking ✓

`program.Send()` is non-blocking:
- Handler doesn't wait for app to process message
- Allows high-frequency logging without stalls
- Excess messages queue internally (bounded by channel size)

#### Panic Safety - REQUIRES IMPLEMENTATION

**Current Gap:** No panic recovery between handler and app.

**Proposed approach:**

```go
func (p *Program) Run() (Model, error) {
    defer func() {
        if r := recover(); r != nil {
            // Reset terminal and re-raise
            p.ReleaseTerminal()
            panic(r)
        }
    }()
    // ... normal run loop
}
```

**Recommendation:** Wrap this at TUI initialization in main.go:

```go
func runTUIWithRecovery(app *App) error {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("PANIC: %v\n", r)
            // Terminal is already released by Bubble Tea
            os.Exit(1)
        }
    }()

    p := tea.NewProgram(app, tea.WithAltScreen())
    _, err := p.Run()
    return err
}
```

---

## 4. Race Condition Analysis

### 4.1 Critical Sections Identified

#### Handler → Program.Send()

**Threat:** `tea.Program` pointer reassigned during lifecycle

**Scenario:**
```go
// Goroutine A (TUI setup)
handler.SetProgram(newProgram)

// Goroutine B (slog caller)
handler.Handle(...)  // Reads program pointer
```

**Mitigation - REQUIRED:**

```go
type TUIHandler struct {
    mu      sync.Mutex
    program *tea.Program
}

func (h *TUIHandler) Handle(ctx context.Context, r slog.Record) error {
    h.mu.Lock()
    program := h.program  // Copy reference
    h.mu.Unlock()

    if program != nil {
        program.Send(LogMsg{ ... })
    }
    return nil
}

func (h *TUIHandler) SetProgram(p *tea.Program) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.program = p
}
```

**Why copy the reference:**
- Minimizes lock duration
- Prevents holding lock during Send() (which might block if channel full)
- Prevents deadlock if Send() triggers log output

#### LogPane Buffer Updates

**Threat:** Update from app.Update() while View() reads

**Context:** Bubble Tea guarantees Update() and View() are serialized in the main loop. **NO LOCK NEEDED** in LogPane.

**However, if external goroutine reads the buffer:**

```go
// DANGEROUS - if View() races with external read
type LogPane struct {
    entries []*LogMsg  // No protection
}
```

**Mitigation - Document assumption:**

```go
// LogPane must only be accessed from app.Update() and app.View()
// Both run in the main event loop (guaranteed serial by Bubble Tea)
// External access requires synchronization.
type LogPane struct {
    entries []*LogMsg  // SINGLE-THREADED: Use only in Update/View
    mu      sync.RWMutex  // Only needed if accessed externally
}
```

**Recommendation:** Keep LogPane single-threaded (no lock). Document this clearly.

#### TerminalWriter Stdout Access

**Threat:** Multiple goroutines writing stdout simultaneously

**All writes MUST go through TerminalWriter:**

```go
// Centralize stdout
func setupTerminal() {
    termWriter := NewTerminalWriter(os.Stdout)

    // Redirect stdlib log (used by external libs)
    log.SetOutput(termWriter)

    // Redirect slog if not using TUIHandler
    slog.SetDefault(slog.New(termHandler))  // TUIHandler already indirect
}
```

**Lock implementation - SOUND:**

```go
func (w *TerminalWriter) Write(p []byte) (int, error) {
    w.mu.Lock()
    defer w.mu.Unlock()
    return w.stdout.Write(p)
}
```

Single mutex protecting all stdout access is correct.

---

### 4.2 Race Condition Matrix

| Scenario | Threads Involved | Risk Level | Mitigation |
|----------|------------------|------------|-----------|
| Handler reads program ptr | Slog caller + TUI setup | HIGH | Mutex + pointer copy (required) |
| Handler sends to program | Slog caller | MEDIUM | program.Send() is thread-safe |
| LogPane buffer update | App.Update() only | NONE | Guaranteed serial by Bubble Tea |
| Stdout access | Any goroutine | HIGH | Centralized TerminalWriter + mutex |
| LogMsg creation | Slog caller | NONE | Immutable value after creation |

**Overall assessment:** Manageable with correct implementation. No fundamental design flaws.

---

## 5. Architectural Anti-Patterns Detected

### 5.1 Handler Leaking Abstraction

**ANTI-PATTERN FOUND:**

```go
// WRONG - Handler knows about TerminalWriter
type TUIHandler struct {
    program       *tea.Program
    termWriter    *TerminalWriter  // Wrong level!

    func (h *TUIHandler) Handle(...) {
        msg := LogMsg{ ... }
        h.program.Send(msg)
        h.termWriter.Suppress()  // WRONG
    }
}
```

**Why this is wrong:**
1. Handler now responsible for TWO things (routing + output suppression)
2. Couples handler to terminal coordination
3. Creates testability nightmare
4. Violates Single Responsibility

**CORRECT PATTERN:**

```go
// Handler ONLY routes to Bubble Tea
type TUIHandler struct {
    mu      sync.Mutex
    program *tea.Program
}

// Terminal coordination happens elsewhere (initialization phase)
func setupLogging() {
    termWriter := NewTerminalWriter(os.Stdout)
    handler := NewTUIHandler(program)

    slog.SetDefault(slog.New(handler))
    log.SetOutput(termWriter)  // Separate concern
}
```

Each component owns its domain.

---

### 5.2 LogPane Inappropriate Intimacy

**Potential ANTI-PATTERN:**

If LogPane stores formatter state or message-processing logic:

```go
// WRONG - LogPane shouldn't format
type LogPane struct {
    formatter *LogFormatter  // WRONG

    func (p *LogPane) Update(msg tea.Msg) {
        logMsg := msg.(LogMsg)
        formatted := p.formatter.Format(logMsg)  // Logic leak
    }
}
```

**CORRECT PATTERN:**

```go
// LogPane only stores and displays; formatting happens in handler
type LogPane struct {
    entries []*LogMsg  // Pre-formatted or minimal formatting

    func (p *LogPane) Update(msg tea.Msg) {
        logMsg := msg.(LogMsg)
        p.Append(logMsg)  // Simple append
    }

    func (p *LogPane) View() string {
        // Formatting only in View() - minimal, display-level only
        var out strings.Builder
        for _, entry := range p.entries {
            out.WriteString(p.formatForDisplay(entry))
        }
        return out.String()
    }
}
```

**Recommendation:** Pre-format LogMsg in handler; LogPane treats it as immutable.

---

### 5.3 Missing Context Propagation

**POTENTIAL ISSUE:**

If LogMsg drops context attributes:

```go
// INCOMPLETE - Loses structured data
type LogMsg struct {
    Level     slog.Level
    Message   string
    Timestamp time.Time
    // Missing: Attrs []slog.Attr
}

func (h *TUIHandler) Handle(ctx context.Context, r slog.Record) error {
    msg := LogMsg{
        Level:   r.Level,
        Message: r.Message,
        // Attributes LOST
    }
}
```

**CORRECT PATTERN:**

```go
type LogMsg struct {
    Level     slog.Level
    Message   string
    Timestamp time.Time
    Attrs     map[string]any  // Preserve context
}

func (h *TUIHandler) Handle(ctx context.Context, r slog.Record) error {
    attrs := make(map[string]any)
    r.Attrs(func(a slog.Attr) bool {
        attrs[a.Key] = a.Value.Any()
        return true
    })

    msg := LogMsg{
        Level:     r.Level,
        Message:   r.Message,
        Timestamp: r.Time,
        Attrs:     attrs,
    }
    // ...
}
```

This preserves debugging information for structured logging.

---

## 6. Risk Analysis

### 6.1 Technical Risks

#### RISK: Handler Initialization Order (MEDIUM RISK)

**Scenario:** Logger configured before program initialized

```go
// WRONG ORDER - slog uses handler before program exists
slog.SetDefault(slog.New(NewTUIHandler(nil)))  // nil program!
p := tea.NewProgram(app)
p.Run()
```

**Impact:** Early logs lost or panicked

**Mitigation:**

```go
// CORRECT: Initialize handler with nil, set program after
handler := NewTUIHandler(nil)
slog.SetDefault(slog.New(handler))

// ... in main setup
p := tea.NewProgram(app)
handler.SetProgram(p)  // Atomic write
_, err := p.Run()
```

---

#### RISK: Program.Send() Channel Overflow (LOW RISK)

**Scenario:** Logs generate faster than app processes

```go
// If: 1000 logs/sec but app only renders at 60fps (16ms per frame)
// Then: ~16 logs process per frame, backlog grows
```

**Bubble Tea default:** 256-message queue (usually sufficient)

**Mitigation - OPTIONAL:**

```go
// Create program with larger queue (if needed)
p := tea.NewProgram(app, tea.WithAltScreen())
// Default is sufficient for logging; only tune if profiling shows pressure
```

**Recommendation:** Profile before optimizing. Logs are low-volume in practice.

---

#### RISK: TUI Crash Crashes Entire App (MEDIUM RISK)

**Scenario:** Panic in LogPane.Update() kills program

**Current Mitigation - INSUFFICIENT:**

```go
// If app.Update() panics, tea.Program doesn't catch it
p := tea.NewProgram(app)
_, err := p.Run()  // panic => crash
```

**Mitigation - REQUIRED:**

Wrap at the app level:

```go
func (app *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("panic in update", "error", r)
            // Don't re-raise; return quit command instead
        }
    }()
    // ... normal update logic
    return app, tea.Batch()
}
```

Or at main.go level (simpler):

```go
defer func() {
    if r := recover(); r != nil {
        // Terminal already restored by Bubble Tea
        fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
        os.Exit(1)
    }
}()

p := tea.NewProgram(app, tea.WithAltScreen())
_, err := p.Run()
```

---

### 6.2 Architectural Risks

#### RISK: Handler Becomes Bottleneck (LOW RISK)

**Analysis:**

- Mutex contention only during handler setup
- Send() is async (no wait)
- No measurable performance impact expected

**Verification needed:** Profile with `go tool pprof` if logs exceed 1000/sec

---

#### RISK: LogPane Grows Unbounded (NONE - MITIGATED BY DESIGN)

**Protected by:** Fixed 500-entry circular buffer

**No risk of memory leak.**

---

#### RISK: External Libraries Bypass TerminalWriter (MEDIUM RISK)

**Scenario:** Intermute or other dependencies write directly to stdout

```go
// External lib doesn't use TerminalWriter
go intermute.Start()  // May call fmt.Println internally
// Interleaved with TUI output
```

**Mitigation - INITIALIZATION:**

```go
// Redirect all possible outputs before starting
log.SetOutput(termWriter)
// For libraries that capture stdout at init time:
// No complete solution; requires external lib cooperation
```

**Recommendation:** Document assumption that external libraries respect `log` package.

---

### 6.3 Integration Risks

#### RISK: Inline Mode Flag Complexity (LOW RISK)

**Scenario:** `--inline` flag affects initialization path

```go
func tuiCmd() *cobra.Command {
    var inline bool
    cmd := &cobra.Command{
        RunE: func(cmd *cobra.Command, args []string) error {
            if inline {
                setupInlineLogging()
            } else {
                setupNormalLogging()
            }
        },
    }
    cmd.Flags().BoolVar(&inline, "inline", false, "...")
}
```

**Risk:** Two code paths diverge; one may bitrot

**Mitigation:** Use feature flags internally, not separate code paths:

```go
// Single initialization
handler := NewTUIHandler(nil)
slog.SetDefault(slog.New(handler))

// Feature flag only affects UI visibility
if inline {
    app.ShowLogPane = true
} else {
    app.ShowLogPane = false
}
```

---

## 7. Recommendations

### 7.1 REQUIRED Changes (Before Implementation)

1. **Interface Abstraction for Program**

   ```go
   // Add to pkg/tui/
   type MessageSender interface {
       Send(msg tea.Msg)
   }

   // TUIHandler depends on interface, not tea.Program
   type TUIHandler struct {
       mu     sync.Mutex
       sender MessageSender
   }
   ```

   **Why:** Improves testability, decouples from Bubble Tea internals

2. **Separate Handler Concerns**

   Ensure:
   - TUIHandler ONLY routes to program
   - TerminalWriter ONLY handles stdout coordination
   - LogPane ONLY displays logs

   Add clear comments marking responsibility boundaries.

3. **Panic Recovery at Entry Point**

   ```go
   // In cmd/autarch/main.go or internal/tui/run.go
   defer func() {
       if r := recover(); r != nil {
           fmt.Fprintf(os.Stderr, "Fatal: %v\n", r)
           os.Exit(1)
       }
   }()
   ```

4. **LogMsg Struct Definition**

   ```go
   // pkg/tui/log.go
   type LogMsg struct {
       Level     slog.Level
       Message   string
       Timestamp time.Time
       Attrs     map[string]any
   }
   ```

   Include all context needed for debugging.

---

### 7.2 STRONGLY RECOMMENDED Changes

1. **Thread-Safe Handler Pattern**

   ```go
   type TUIHandler struct {
       mu     sync.Mutex
       sender MessageSender
   }

   func (h *TUIHandler) SetSender(s MessageSender) {
       h.mu.Lock()
       defer h.mu.Unlock()
       h.sender = s
   }

   func (h *TUIHandler) Handle(ctx context.Context, r slog.Record) error {
       h.mu.Lock()
       sender := h.sender
       h.mu.Unlock()

       if sender == nil {
           return nil  // Drop logs before program ready
       }
       // ... create LogMsg ...
       sender.Send(msg)
       return nil
   }
   ```

2. **Pre-format LogMsg in Handler**

   Move formatting logic to handler, not LogPane:

   ```go
   func (h *TUIHandler) Handle(ctx context.Context, r slog.Record) error {
       msg := LogMsg{
           Level:     r.Level,
           Message:   r.Message,
           Timestamp: r.Time,
           Attrs:     extractAttrs(r),  // Formatting here
       }
       // ...
   }
   ```

3. **Bounded Channel for Extra Messages**

   Consider an intermediate buffer if needed:

   ```go
   type LogBuffer struct {
       ch chan LogMsg
   }

   // In goroutine: process channel, batch updates
   // Prevents message loss if app momentarily slow
   ```

   **Only if profiling shows message loss.**

---

### 7.3 NICE-TO-HAVE Enhancements

1. **Log Level Filtering in UI**

   Allow user to filter shown logs:

   ```go
   type LogPane struct {
       entries []*LogMsg
       filter  slog.Level
   }
   ```

2. **Search/Filter in LogPane**

   Add substring search in log history.

3. **Export Log Capture**

   Add command to save logs to file for debugging.

4. **Metrics**

   Track dropped messages, processing latency:

   ```go
   type LogMetrics struct {
       Processed uint64
       Dropped   uint64
       Latency   time.Duration
   }
   ```

---

## 8. Implementation Checklist

**Phase 1: Foundation**

- [ ] Define `MessageSender` interface in `pkg/tui/`
- [ ] Define `LogMsg` struct in `pkg/tui/log.go`
- [ ] Implement `TUIHandler` with mutex-protected sender
- [ ] Add panic recovery wrapper at TUI entry point

**Phase 2: Components**

- [ ] Implement `LogPane` with 500-entry circular buffer
- [ ] Implement `TerminalWriter` with stdout mutex
- [ ] Wire into app.Update() to handle LogMsg
- [ ] Add LogPane to app.View() (opt-in pane)

**Phase 3: Integration**

- [ ] Add `--inline` flag to `autarch tui` command
- [ ] Set handler log level based on flag
- [ ] Wire setup logging into main.go
- [ ] Test with high-volume log generation

**Phase 4: Refinement**

- [ ] Profile message throughput
- [ ] Add filtering UI if needed
- [ ] Document assumptions (single-threaded LogPane, etc.)
- [ ] Add integration tests

---

## 9. Testing Strategy

### 9.1 Unit Tests

```go
// pkg/tui/log_test.go

func TestTUIHandlerThreadSafety(t *testing.T) {
    // Simulate concurrent slog calls
    // Verify no panics or race condition detector hits
}

func TestLogPaneCircularBuffer(t *testing.T) {
    pane := NewLogPane()

    // Add 600 items (exceeds 500 limit)
    for i := 0; i < 600; i++ {
        pane.Append(&LogMsg{Message: fmt.Sprintf("log %d", i)})
    }

    // Verify only last 500 retained
    if pane.count != 500 {
        t.Fatalf("expected 500 entries, got %d", pane.count)
    }

    // Verify oldest entries were dropped
    if strings.Contains(pane.entries[pane.head].Message, "log 0") {
        t.Fatal("oldest entry should have been discarded")
    }
}

func TestTerminalWriterMutualExclusion(t *testing.T) {
    // Simulate concurrent Write() calls
    // Verify output isn't corrupted
}
```

### 9.2 Integration Tests

```go
func TestInlineLoggingEndToEnd(t *testing.T) {
    // Create app with LogPane
    // Generate logs from multiple goroutines
    // Verify all appear in LogPane
    // Verify no race detector warnings
}

func TestLoggingGracefulDegradation(t *testing.T) {
    // Handler before program initialized
    // Verify logs dropped silently (no panic)
}
```

### 9.3 Load Test

```bash
# Generate 10k logs/sec, verify:
# - No message loss
# - TUI responsive (sub-100ms render)
# - CPU reasonable (< 5%)
go test -run TestHighVolumeLogging -timeout 30s
```

---

## 10. Conclusion

### Summary

The FrankenTUI-inspired inline logging architecture is **fundamentally sound**. The proposed message routing, circular buffer design, and concurrency patterns follow established Bubble Tea conventions and maintain Autarch's architectural integrity.

### Verdict

**Status: APPROVED WITH REQUIRED MODIFICATIONS**

**Must implement before merging:**
1. Interface abstraction for `MessageSender` (improves testability)
2. Thread-safe handler with mutex and pointer copy
3. Panic recovery at entry point
4. Clear separation of concerns between handler, buffer, and writer

**Must document:**
1. Single-threaded assumption for LogPane
2. Initialization order requirement (handler setup before program)
3. Assumption that external libraries respect log package

### Go/No-Go

**GO** - Proceed to implementation with recommendations applied.

This design will significantly improve debugging experience without compromising system stability or architectural clarity.

---

## Appendix: Related Files

**Files to modify/create:**

| Path | Purpose |
|------|---------|
| `/root/projects/Autarch/pkg/tui/log.go` | LogMsg, TUIHandler, MessageSender interface |
| `/root/projects/Autarch/pkg/tui/log_pane.go` | LogPane circular buffer implementation |
| `/root/projects/Autarch/pkg/tui/terminal_writer.go` | Centralized stdout coordination |
| `/root/projects/Autarch/internal/tui/app.go` | Wire LogPane into app.Update/View |
| `/root/projects/Autarch/cmd/autarch/main.go` | Setup logging, panic recovery, --inline flag |

**Related architecture docs:**

- `/root/projects/Autarch/docs/ARCHITECTURE.md` - Update with logging subsystem
- `/root/projects/Autarch/AGENTS.md` - Add inline logging to development guide

