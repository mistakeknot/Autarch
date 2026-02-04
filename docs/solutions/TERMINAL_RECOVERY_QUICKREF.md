# Terminal State Restoration - Quick Reference

One-page reference for terminal restoration patterns in Go TUI applications.

---

## The Five Critical Patterns

### 1. Raw Mode Setup (Must Have)

```go
import (
    "golang.org/x/term"
    "os"
)

oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
if err != nil {
    return err
}
defer term.Restore(int(os.Stdin.Fd()), oldState)
```

**Key Point:** `defer` ensures restore even on panic

---

### 2. Bubble Tea Program Creation

```go
import tea "github.com/charmbracelet/bubbletea"

p := tea.NewProgram(model,
    tea.WithAltScreen(),          // ← Required
    tea.WithContext(ctx),          // ← Required for signals
    tea.WithMouseAllMotion(),      // ← Optional
)
_, err := p.Run()
```

**Key Point:** `WithAltScreen()` + `WithContext()` = automatic cleanup

---

### 3. Signal Handling (Modern Pattern)

```go
import (
    "context"
    "os/signal"
    "syscall"
)

ctx, stop := signal.NotifyContext(context.Background(),
    syscall.SIGINT,   // Ctrl+C
    syscall.SIGTERM,  // Kill signal
)
defer stop()

p := tea.NewProgram(model, tea.WithContext(ctx))
_, err := p.Run()
// Program exits cleanly when signals received
```

**Key Point:** Context cancellation = graceful shutdown

---

### 4. Panic Recovery

```go
defer func() {
    if r := recover(); r != nil {
        EmergencyTerminalRestore()
        fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
        os.Exit(1)
    }
}()

// Your app code
```

**Emergency Restore Function:**

```go
func EmergencyTerminalRestore() error {
    sequences := "\x1b[?25h\x1b[?1049l\x1b(B\x1b[m\n\r"
    _, err := fmt.Fprint(os.Stderr, sequences)
    if err != nil {
        _, _ = fmt.Fprint(os.Stdout, sequences)
    }
    return err
}
```

---

### 5. Terminal State Wrapper

```go
type TerminalState struct {
    oldState interface{}
    fd       int
    mu       sync.Mutex
}

func NewTerminalState() (*TerminalState, error) {
    fd := int(os.Stdin.Fd())
    oldState, err := term.MakeRaw(fd)
    if err != nil {
        return nil, err
    }
    return &TerminalState{
        oldState: oldState,
        fd:       fd,
    }, nil
}

func (ts *TerminalState) Restore() error {
    ts.mu.Lock()
    defer ts.mu.Unlock()
    return term.Restore(ts.fd, ts.oldState)
}
```

---

## Complete Minimal Main

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    tea "github.com/charmbracelet/bubbletea"
    "golang.org/x/term"
)

func main() {
    // 1. Signal context
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    // 2. Raw mode
    fd := int(os.Stdin.Fd())
    oldState, err := term.MakeRaw(fd)
    if err != nil {
        os.Exit(1)
    }
    defer term.Restore(fd, oldState)

    // 3. Panic recovery
    defer func() {
        if r := recover(); r != nil {
            term.Restore(fd, oldState)
            fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
            os.Exit(1)
        }
    }()

    // 4. TUI
    p := tea.NewProgram(model{},
        tea.WithAltScreen(),
        tea.WithContext(ctx),
    )

    if _, err := p.Run(); err != nil {
        os.Exit(1)
    }
}
```

---

## ANSI Sequences Cheat Sheet

| Sequence | Function | Use Case |
|----------|----------|----------|
| `\x1b[?25h` | Show cursor | Emergency restore |
| `\x1b[?25l` | Hide cursor | TUI active |
| `\x1b[?1049h` | Enter alt-screen | Start TUI |
| `\x1b[?1049l` | Exit alt-screen | End TUI |
| `\x1b(B\x1b[m` | Reset attributes | Panic handler |
| `\x1b[H\x1b[2J` | Clear screen | Emergency restore |

**Emergency Restore One-Liner:**
```bash
printf '\x1b[?25h\x1b[?1049l\x1b(B\x1b[m\x1b[H\x1b[2J'
```

---

## Signal Handling Patterns

### Pattern A: Context-based (Recommended)

```go
ctx, stop := signal.NotifyContext(ctx, SIGINT, SIGTERM)
defer stop()
p := tea.NewProgram(m, tea.WithContext(ctx))
_, err := p.Run()  // Auto-shutdown on signal
```

**Pros:** Simple, automatic cleanup
**Cons:** Less control

### Pattern B: Custom signal channel

```go
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, SIGINT, SIGTERM)
defer signal.Stop(sigChan)

p := tea.NewProgram(m, tea.WithoutSignalHandler())
// Handle sigChan in Update()
```

**Pros:** Full control, can do double-Ctrl+C
**Cons:** More code

---

## Debugging Checklist

| Issue | Check | Fix |
|-------|-------|-----|
| Corrupted terminal | Run `reset` or `stty sane` | Verify raw mode cleanup |
| Cursor invisible | Check `\x1b[?25h` sent | Add to emergency restore |
| Ctrl+C doesn't work | Verify signal setup | Add context to tea.NewProgram |
| App hangs on exit | Check for blocking I/O | Use context timeout |
| Panic kills terminal | Check defer in panic handler | Add EmergencyTerminalRestore |
| SSH terminal issues | Check signal handling | Use WithContext pattern |

---

## Testing Commands

```bash
# Unit tests
go test -v -run TestTerminal ./...

# Integration tests
go test -v -run TestTUI ./...

# Manual test - normal exit
./myapp
# Press 'q' or Ctrl+C
# Verify: prompt appears, terminal works

# Manual test - signal handling
./myapp
# Send: kill -TERM $(pgrep myapp)
# Verify: clean exit, no corruption

# Manual test - panic (DANGEROUS)
# Add panic to code, rebuild
./myapp
# Verify: terminal restored, error visible

# Cross-platform (Linux)
strace -e ioctl ./myapp
# Look for: TCGETS, TCSETS syscalls

# Cross-platform (macOS)
./myapp
# Verify: works in Terminal.app and iTerm2

# Cross-platform (Windows)
./myapp.exe  # In Windows Terminal
# Verify: console modes set correctly
```

---

## Do's and Don'ts

### Do

- ✓ Use `defer term.Restore()` immediately after `MakeRaw()`
- ✓ Use `signal.NotifyContext()` for graceful shutdown
- ✓ Pass context to `tea.WithContext()`
- ✓ Include panic recovery in main()
- ✓ Test on target platforms (Linux, macOS, Windows)
- ✓ Document terminal state management in code comments
- ✓ Provide manual recovery instructions to users

### Don't

- ✗ Forget `defer term.Restore()`
- ✗ Use `WithoutSignalHandler()` unless you handle signals
- ✗ Run TUI from goroutines (context cancellation may not work)
- ✗ Ignore SIGWINCH (Bubble Tea handles it)
- ✗ Rely on panic recovery alone (combine with signal handling)
- ✗ Assume file descriptor is always 0
- ✗ Skip terminal capability detection

---

## Platform Differences

### Linux
- Uses `termios` syscalls via `golang.org/x/term`
- Full ANSI support in most terminals
- SIGWINCH for resize (handled by Bubble Tea)

### macOS
- Same as Linux (BSD-based)
- `os.Stdin.Fd()` typically returns 0
- Requires xterm-256color or similar for colors

### Windows
- Uses console API instead of termios
- `term.MakeRaw()` sets console modes
- ANSI support since Windows 10 v1607

---

## Environment Variables to Check

```bash
# Terminal type
echo $TERM

# Character encoding
echo $LANG

# Color support
echo $COLORTERM

# In your code:
term := os.Getenv("TERM")
if term == "" || term == "dumb" {
    // Limited terminal
}
```

---

## Common Gotchas

1. **File descriptor is not always 0**
   ```go
   // ✗ Wrong
   fd := 0

   // ✓ Right
   fd := int(os.Stdin.Fd())
   ```

2. **Panic doesn't propagate with `recover`**
   ```go
   // ✗ Wrong - recover catches panic here
   go func() {
       defer recover()
       panic("oops")  // Caught but lost
   }()

   // ✓ Right - panic propagates to main's handler
   defer func() {
       if r := recover(); r != nil {
           log.Printf("Panic: %v", r)
       }
   }()
   ```

3. **Context cancellation is not a signal**
   ```go
   // ✗ Won't handle signals
   p := tea.NewProgram(m)  // No context

   // ✓ Handles SIGINT/SIGTERM
   ctx, stop := signal.NotifyContext(bg, SIGINT, SIGTERM)
   defer stop()
   p := tea.NewProgram(m, tea.WithContext(ctx))
   ```

4. **WithoutCatchPanics still needs defer recovery**
   ```go
   defer func() {
       term.Restore(fd, oldState)
       if r := recover(); r != nil {
           EmergencyTerminalRestore()
       }
   }()
   ```

---

## References

- [golang.org/x/term](https://pkg.go.dev/golang.org/x/term)
- [Bubble Tea](https://pkg.go.dev/github.com/charmbracelet/bubbletea)
- [Go Defer/Panic/Recover](https://go.dev/blog/defer-panic-and-recover)
- [Go Graceful Shutdown](https://victoriametrics.com/blog/go-graceful-shutdown/)

---

**Last Updated:** February 2026
**Quick Ref Version:** 1.0
**Applicable:** Go 1.22+, Bubble Tea v2.0+
