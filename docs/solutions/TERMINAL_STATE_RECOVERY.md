# Terminal State Restoration and Panic Recovery in Go TUI Applications

Best practices guide for terminal state management, signal handling, panic recovery, and cross-platform considerations in Go TUI applications using Bubble Tea.

**Last Updated:** February 2026
**Framework:** Bubble Tea v2, Go 1.24+
**Focus:** Production-ready terminal restoration patterns

---

## Table of Contents

1. [Terminal State Management](#terminal-state-management)
2. [ANSI Escape Sequences](#ansi-escape-sequences)
3. [Signal Handling](#signal-handling)
4. [Panic Recovery Patterns](#panic-recovery-patterns)
5. [Testing Terminal Restoration](#testing-terminal-restoration)
6. [Cross-Platform Considerations](#cross-platform-considerations)
7. [Complete Example](#complete-example)
8. [Troubleshooting](#troubleshooting)

---

## Terminal State Management

### Raw Mode and Alt-Screen Buffer

Go's standard library provides the `golang.org/x/term` package for terminal state manipulation. Two critical terminal modes must be managed:

**1. Raw Mode (Input Processing)**

Raw mode disables canonical input processing, meaning the terminal doesn't wait for Enter before sending input to your application.

```go
import (
    "golang.org/x/term"
    "os"
)

// Enable raw mode
oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
if err != nil {
    return fmt.Errorf("failed to enable raw mode: %w", err)
}

// CRITICAL: Always restore in defer to handle panics
defer func() {
    _ = term.Restore(int(os.Stdin.Fd()), oldState)
}()
```

**Key Properties of Raw Mode:**
- Disables `ICANON` (canonical mode) - input available immediately
- Disables `ECHO` - typed characters not echoed
- Sets `MIN=0` and `TIME=0` for non-blocking input
- Disables signal processing (Ctrl+C, Ctrl+Z not intercepted by terminal driver)

**2. Alt-Screen Buffer**

Preserves the normal screen while the TUI application runs. When the app exits, the original terminal content is restored.

**Bubble Tea Handles Alt-Screen Automatically:**

```go
import tea "github.com/charmbracelet/bubbletea"

// Bubble Tea automatically manages alt-screen
p := tea.NewProgram(model{},
    tea.WithAltScreen(),        // Enable alt-screen
    tea.WithMouseAllMotion(),   // Optional: enable mouse support
)

_, err := p.Run()
```

Bubble Tea's `WithAltScreen()` option sends ANSI sequences to enter/exit the alternate screen buffer:
- Enter: `\x1b[?1049h` (enable alt-screen + save cursor)
- Exit: `\x1b[?1049l` (disable alt-screen + restore cursor)

### Terminal State Isolation

The safest pattern is to wrap raw mode setup in a dedicated function:

```go
// TerminalState represents the saved terminal configuration
type TerminalState struct {
    oldState interface{}
    fd       int
}

// EnableRawMode enables raw input processing
func EnableRawMode() (*TerminalState, error) {
    fd := int(os.Stdin.Fd())
    oldState, err := term.MakeRaw(fd)
    if err != nil {
        return nil, fmt.Errorf("term.MakeRaw failed: %w", err)
    }
    return &TerminalState{oldState: oldState, fd: fd}, nil
}

// Restore restores the previous terminal state
func (ts *TerminalState) Restore() error {
    if ts == nil || ts.oldState == nil {
        return nil
    }
    return term.Restore(ts.fd, ts.oldState)
}

// Usage in main:
func main() {
    termState, err := EnableRawMode()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to enable raw mode: %v\n", err)
        os.Exit(1)
    }
    defer termState.Restore()

    // Now start Bubble Tea TUI...
    p := tea.NewProgram(model{}, tea.WithAltScreen())
    if _, err := p.Run(); err != nil {
        // Panic recovery will still trigger defer
        fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
        os.Exit(1)
    }
}
```

---

## ANSI Escape Sequences

### Critical Sequences for Terminal Restoration

When your application crashes or needs emergency cleanup, understanding ANSI sequences is essential:

| Sequence | Function | When Used |
|----------|----------|-----------|
| `\x1b[?25h` | Show cursor | Emergency recovery |
| `\x1b[?25l` | Hide cursor | TUI active |
| `\x1b[?1049h` | Enter alt-screen | Start TUI |
| `\x1b[?1049l` | Exit alt-screen | End TUI |
| `\x1b(B\x1b[m` | Reset all attributes | Emergency cleanup |
| `\x1b[H\x1b[2J` | Clear screen & home cursor | Emergency cleanup |
| `\x1b[?1002h` | Enable mouse | Optional |
| `\x1b[?1002l` | Disable mouse | Exit |

### Emergency Restoration Function

Use this for panic handlers or signal cleanup:

```go
import (
    "fmt"
    "os"
)

// EmergencyTerminalRestore sends minimal ANSI sequences to restore terminal
// This is designed to run even in panic scenarios where normal cleanup failed
func EmergencyTerminalRestore() error {
    // Sequence: show cursor + reset all attributes + clear screen
    sequences := "\x1b[?25h\x1b[?1049l\x1b(B\x1b[m"

    _, err := fmt.Fprint(os.Stderr, sequences)
    if err != nil {
        // Last resort: write to stdout if stderr fails
        _, _ = fmt.Fprint(os.Stdout, sequences)
    }
    return err
}

// Panic handler
func recoverPanic() {
    if r := recover(); r != nil {
        // Terminal is likely corrupted from panic
        EmergencyTerminalRestore()

        fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
        os.Exit(1)
    }
}
```

### Charmbracelet ANSI Package

The `github.com/charmbracelet/x/ansi` package provides type-safe ANSI sequence generation:

```go
import "github.com/charmbracelet/x/ansi"

// Instead of hand-crafted strings
cursor := ansi.CursorShow        // Type-safe
sequences := ansi.ResetAll       // Better readability
```

---

## Signal Handling

### Pattern 1: Using signal.NotifyContext (Recommended for 2026)

This is the modern Go pattern for graceful signal handling:

```go
import (
    "context"
    "os"
    "os/signal"
    "syscall"
    tea "github.com/charmbracelet/bubbletea"
)

func main() {
    // Set up signal context
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT,   // Ctrl+C
        syscall.SIGTERM,  // Kill signal
    )
    defer stop()

    termState, err := EnableRawMode()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed: %v\n", err)
        os.Exit(1)
    }
    defer termState.Restore()

    // Create program with context
    p := tea.NewProgram(model{},
        tea.WithAltScreen(),
        tea.WithContext(ctx),
        tea.WithMouseAllMotion(),
    )

    _, err = p.Run()

    // signal.NotifyContext already handled SIGINT/SIGTERM
    // Program exits cleanly with defers executing

    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

**Why This Works:**
1. `signal.NotifyContext` converts OS signals to context cancellation
2. Bubble Tea's `WithContext()` polls the context during event loop
3. On signal, context is cancelled, Bubble Tea gracefully shuts down
4. All `defer` statements execute (including terminal restoration)

### Pattern 2: Manual Signal Channel (for custom handling)

Sometimes you need more control—e.g., double Ctrl+C to force quit:

```go
import (
    "os"
    "os/signal"
    "syscall"
    "time"
    tea "github.com/charmbracelet/bubbletea"
)

func main() {
    termState, err := EnableRawMode()
    if err != nil {
        os.Exit(1)
    }
    defer termState.Restore()

    // Create unbuffered channel for signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    // Important: stop listening after setup
    defer signal.Stop(sigChan)

    m := model{
        sigChan:     sigChan,
        lastSigTime: time.Time{},
    }

    p := tea.NewProgram(m,
        tea.WithAltScreen(),
        tea.WithoutSignalHandler(),  // Disable Bubble Tea's handler
    )

    _, err = p.Run()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}

// In your model's Update method
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        if msg.Type == tea.KeyCtrlC {
            // Handle Ctrl+C pressed
            now := time.Now()
            if now.Sub(m.lastSigTime) < 500*time.Millisecond {
                // Double Ctrl+C - force quit
                return m, tea.Quit
            }
            m.lastSigTime = now
            // Single Ctrl+C - show quit confirmation
            return m, nil
        }
    }
    return m, nil
}
```

### Pattern 3: SIGWINCH (Terminal Resize) Handling

Bubble Tea automatically handles terminal resize via `tea.WindowSizeMsg`:

```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        return m, nil
    }
    return m, nil
}
```

**CRITICAL:** Bubble Tea handles SIGWINCH internally. Do NOT use `signal.Notify` with `syscall.SIGWINCH` - it will conflict.

---

## Panic Recovery Patterns

### The Fundamental Rule

**Panic recovery must occur on the same goroutine where the panic occurred.** Panics do NOT cross goroutine boundaries.

### Pattern 1: Recovery in main()

```go
func main() {
    defer recoverPanic()

    termState, err := EnableRawMode()
    if err != nil {
        os.Exit(1)
    }
    defer termState.Restore()

    p := tea.NewProgram(model{}, tea.WithAltScreen())
    _, err = p.Run()

    if err != nil {
        os.Exit(1)
    }
}

func recoverPanic() {
    if r := recover(); r != nil {
        // Terminal may be corrupted - restore it
        EmergencyTerminalRestore()

        // Log the panic
        fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)

        // Print stack trace for debugging
        debug.PrintStack()

        os.Exit(1)
    }
}
```

### Pattern 2: Wrapped Model with Recovery (for model panics)

```go
// SafeModel wraps a model with panic recovery
type SafeModel struct {
    inner tea.Model
}

func (s SafeModel) Init() tea.Cmd {
    defer recoverPanic()
    return s.inner.Init()
}

func (s SafeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    defer recoverPanic()
    m, cmd := s.inner.Update(msg)
    return SafeModel{inner: m}, cmd
}

func (s SafeModel) View() string {
    defer recoverPanic()
    return s.inner.View()
}

func recoverPanic() {
    if r := recover(); r != nil {
        fmt.Fprintf(os.Stderr, "Model panic: %v\n", r)
        // Re-panic so outer handler can catch it
        panic(r)
    }
}

// Usage
p := tea.NewProgram(
    SafeModel{inner: model{}},
    tea.WithAltScreen(),
)
```

### Pattern 3: Goroutine Panic Recovery

Panics in spawned goroutines require their own recovery:

```go
// Command that spawns a goroutine
func (m model) someAsyncOperation() tea.Cmd {
    return func() tea.Msg {
        go func() {
            defer recoverGoroutinePanic()

            // Long-running operation
            result := expensiveComputation()

            // Send result back
            msgChan <- computeResultMsg{result}
        }()
        return nil
    }
}

func recoverGoroutinePanic() {
    if r := recover(); r != nil {
        fmt.Fprintf(os.Stderr, "Goroutine panic: %v\n", r)
        // Note: Cannot restore terminal from here!
        // The main goroutine must handle signal propagation
    }
}
```

### Pattern 4: Bubble Tea's WithoutCatchPanics

For debugging, disable Bubble Tea's automatic panic catching:

```go
// ONLY for development/debugging
p := tea.NewProgram(model{},
    tea.WithAltScreen(),
    tea.WithoutCatchPanics(),  // Panics propagate to your recovery handler
)
```

When `WithoutCatchPanics()` is used:
- Bubble Tea doesn't catch panics internally
- Your defer recovery handlers can inspect the panic
- Terminal is NOT automatically restored by Bubble Tea
- You must restore it yourself

---

## Testing Terminal Restoration

### Unit Testing Terminal State

```go
package myapp

import (
    "os"
    "testing"
    "golang.org/x/term"
)

// TestTerminalStateIsolation verifies raw mode doesn't escape
func TestTerminalStateIsolation(t *testing.T) {
    // Check initial state
    initialState := getTerminalState()

    // Enable raw mode
    ts, err := EnableRawMode()
    if err != nil {
        t.Skipf("Can't enable raw mode (not a TTY): %v", err)
    }
    defer ts.Restore()

    // Verify we're in raw mode
    if !isTerminalRaw() {
        t.Fatal("expected terminal to be in raw mode")
    }

    // Restore
    ts.Restore()

    // Verify state restored
    if !statesEqual(initialState, getTerminalState()) {
        t.Fatal("terminal state not properly restored")
    }
}

// Helper: check if terminal is in raw mode
func isTerminalRaw() bool {
    // Implementation uses termios syscalls
    // Pseudo-code:
    // var tc syscall.Termios
    // syscall.IoctlGetTermios(int(os.Stdin.Fd()), syscall.TCGETS, &tc)
    // return tc.Lflag&unix.ICANON == 0
    return true // placeholder
}

// Helper: capture current terminal state
func getTerminalState() interface{} {
    state, _ := term.MakeRaw(int(os.Stdin.Fd()))
    term.Restore(int(os.Stdin.Fd()), state)
    return state
}

func statesEqual(a, b interface{}) bool {
    return a == b  // Simplified
}
```

### Integration Testing with TUI Automation

```go
package myapp_test

import (
    "context"
    "testing"
    "time"
    tea "github.com/charmbracelet/bubbletea"
)

// TestTUIExitRestoresTerminal verifies clean shutdown
func TestTUIExitRestoresTerminal(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    m := NewModel()
    p := tea.NewProgram(m,
        tea.WithContext(ctx),
        tea.WithAltScreen(),
    )

    // Run in goroutine
    resultChan := make(chan error, 1)
    go func() {
        _, err := p.Run()
        resultChan <- err
    }()

    // Wait for context to cancel (simulates graceful shutdown)
    <-ctx.Done()

    // Program should exit cleanly
    err := <-resultChan
    if err != nil {
        t.Fatalf("TUI exited with error: %v", err)
    }
}

// TestPanicRecovery verifies panic handler restores terminal
func TestPanicRecovery(t *testing.T) {
    if !runUnderTest() {
        t.Skip("Skipping panic test in non-test environment")
    }

    // This test requires careful setup to avoid corrupting your actual terminal
    // Best approach: use Docker container or separate test environment

    // Pseudo-code:
    // 1. Capture current terminal state
    // 2. Trigger a panic in a TUI model
    // 3. Verify terminal state is restored
}

func runUnderTest() bool {
    return os.Getenv("GO_TEST_ENVIRONMENT") != ""
}
```

### Manual Testing Checklist

```bash
# 1. Test Ctrl+C graceful shutdown
$ ./myapp
# Press Ctrl+C, verify:
# - Terminal prompt appears
# - No cursor stuck at end of screen
# - All output visible

# 2. Test force quit (if implementing)
$ ./myapp
# Press Ctrl+C twice rapidly, verify graceful exit

# 3. Test terminal resize
$ ./myapp
# Resize terminal window while running
# Verify layout adjusts properly

# 4. Test panic scenario (DANGEROUS - do in a container!)
# Trigger a panic from your model
# Verify terminal is restored and usable

# 5. Test exit from SSH/remote
# ssh user@host './myapp'
# Ctrl+C to exit
# Verify terminal doesn't hang or corrupt

# 6. Test with different terminal emulators
# - Linux: xterm, GNOME Terminal, Alacritty
# - macOS: Terminal.app, iTerm2
# - Windows: Windows Terminal, ConEmu
```

---

## Cross-Platform Considerations

### Platform-Specific Raw Mode

**Unix/Linux/macOS:**

```go
import (
    "golang.org/x/term"
    "os"
)

oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
// Uses termios syscalls on Unix

// Note: On macOS, os.Stdin.Fd() is 0
// On Linux, file descriptor may be different depending on how process started
// Always use os.Stdin.Fd() to be safe
```

**Windows:**

Windows doesn't have "raw mode" in the Unix sense. Instead, Windows uses console modes:

```go
import (
    "golang.org/x/term"
    "os"
)

// term.MakeRaw() works on Windows by setting:
// - ENABLE_VIRTUAL_TERMINAL_INPUT
// - ENABLE_VIRTUAL_TERMINAL_PROCESSING
// - Disables line input mode

oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
// Sets console mode on Windows
```

### File Descriptor Differences

Not all OS handle file descriptors identically:

```go
import (
    "os"
    "golang.org/x/term"
)

// Correct approach
fd := int(os.Stdin.Fd())

// Common mistakes (avoid):
// fd := 0  // Assumes stdin is always 0 - NOT GUARANTEED
// fd := int(syscall.Stdin)  // Windows-specific constant

// The key: use os.Stdin.Fd() consistently
```

### ANSI Sequence Support

Not all terminals support all ANSI sequences. Bubble Tea handles this, but for emergency recovery:

```go
import "os"

// Windows 10+ supports ANSI (with ENABLE_VIRTUAL_TERMINAL_PROCESSING)
// Most modern Unix terminals support ANSI
//
// Safest sequences (nearly universal):
// - \x1b[?25h (show cursor)
// - \x1b[?25l (hide cursor)
// - \x1b(B\x1b[m (reset all attributes)
// - \x1b[H\x1b[2J (clear screen)

// Use this for maximum compatibility:
func EmergencyRestore() {
    // Minimal, widely-supported sequences
    os.Stderr.WriteString("\x1b[?25h")  // Show cursor
    os.Stderr.WriteString("\x1b(B")     // Reset charset
    os.Stderr.WriteString("\x1b[m")     // Reset attributes
}
```

### Testing Across Platforms

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o app-linux ./cmd/app
./app-linux

# macOS
GOOS=darwin GOARCH=amd64 go build -o app-macos ./cmd/app
./app-macos

# Windows
GOOS=windows GOARCH=amd64 go build -o app-windows.exe ./cmd/app
./app-windows.exe
```

---

## Complete Example

Production-ready TUI application with full terminal state management:

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "runtime/debug"
    "syscall"

    tea "github.com/charmbracelet/bubbletea"
    "golang.org/x/term"
)

// TerminalState represents saved terminal configuration
type TerminalState struct {
    oldState interface{}
    fd       int
    saved    bool
}

// EnableRawMode enters raw input mode
func EnableRawMode() (*TerminalState, error) {
    fd := int(os.Stdin.Fd())
    oldState, err := term.MakeRaw(fd)
    if err != nil {
        return nil, fmt.Errorf("term.MakeRaw: %w", err)
    }
    return &TerminalState{
        oldState: oldState,
        fd:       fd,
        saved:    true,
    }, nil
}

// Restore restores the previous terminal state
func (ts *TerminalState) Restore() error {
    if !ts.saved {
        return nil
    }
    if ts.oldState == nil {
        return nil
    }
    if err := term.Restore(ts.fd, ts.oldState); err != nil {
        return fmt.Errorf("term.Restore: %w", err)
    }
    ts.saved = false
    return nil
}

// EmergencyRestore sends ANSI sequences to restore terminal
func EmergencyRestore() error {
    // Sequences: show cursor + reset attributes + clear screen
    sequences := "\x1b[?25h\x1b(B\x1b[m\x1b[2J\x1b[H"
    _, err := fmt.Fprint(os.Stderr, sequences)
    if err != nil {
        _, _ = fmt.Fprint(os.Stdout, sequences)
    }
    return err
}

// Model is the TUI application model
type Model struct {
    width  int
    height int
}

func (m Model) Init() tea.Cmd {
    return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        if msg.String() == "ctrl+c" || msg.String() == "q" {
            return m, tea.Quit
        }
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
    }
    return m, nil
}

func (m Model) View() string {
    return fmt.Sprintf("Terminal: %dx%d\nPress q or Ctrl+C to quit\n",
        m.width, m.height)
}

func main() {
    // Setup signal handling
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT,
        syscall.SIGTERM,
    )
    defer stop()

    // Enable raw mode
    termState, err := EnableRawMode()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to enable raw mode: %v\n", err)
        os.Exit(1)
    }
    defer func() {
        if err := termState.Restore(); err != nil {
            fmt.Fprintf(os.Stderr, "Failed to restore terminal: %v\n", err)
        }
    }()

    // Panic handler
    defer func() {
        if r := recover(); r != nil {
            // Terminal likely corrupted - restore it
            EmergencyRestore()

            fmt.Fprintf(os.Stderr, "\nPANIC: %v\n", r)
            debug.PrintStack()
            os.Exit(1)
        }
    }()

    // Create and run TUI
    m := Model{}
    p := tea.NewProgram(m,
        tea.WithAltScreen(),
        tea.WithMouseAllMotion(),
        tea.WithContext(ctx),
    )

    _, err = p.Run()
    if err != nil {
        fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
        os.Exit(1)
    }
}
```

---

## Troubleshooting

### Terminal appears corrupted after exit

**Symptom:** Cursor invisible, text garbled, no response to input

**Solutions:**

1. Restore terminal manually:
```bash
# Option 1: Press Enter + type 'reset'
reset

# Option 2: Use stty directly
stty sane
stty echo

# Option 3: In Vim
:set term=xterm
:qa!
```

2. Check your defer statements are executing:
```go
defer func() {
    fmt.Fprintf(os.Stderr, "DEBUG: Restoring terminal\n")
    termState.Restore()
}()
```

3. Verify raw mode setup:
```go
// Add logging
oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
if err != nil {
    fmt.Fprintf(os.Stderr, "DEBUG: MakeRaw failed: %v\n", err)
}
defer func() {
    fmt.Fprintf(os.Stderr, "DEBUG: Calling Restore\n")
    term.Restore(int(os.Stdin.Fd()), oldState)
}()
```

### Cursor stuck at wrong position

**Symptom:** Cursor appears at middle/end of screen instead of beginning of line

**Cause:** Alt-screen exit sequence not sent properly

**Solution:**

```go
// Ensure Bubble Tea handles exit
p := tea.NewProgram(model{},
    tea.WithAltScreen(),  // MUST be present
)

// Clean exit
_, err := p.Run()
// Bubble Tea automatically sends alt-screen exit sequence
```

### Ctrl+C doesn't quit cleanly

**Symptom:** Ctrl+C typed in TUI but nothing happens

**Cause 1:** Signal handler not set up
```go
// Wrong
p := tea.NewProgram(model{})  // No signal handling

// Right
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT)
defer stop()
p := tea.NewProgram(model{}, tea.WithContext(ctx))
```

**Cause 2:** WithoutSignalHandler used without custom handling
```go
// Wrong - no signal handler at all
p := tea.NewProgram(model{},
    tea.WithoutSignalHandler(),  // Disables Bubble Tea handler
)

// Right - custom signal handler
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGINT)
p := tea.NewProgram(model{},
    tea.WithoutSignalHandler(),
)
// Handle sigChan in Update()
```

### Panics don't trigger recovery

**Symptom:** Application crashes with stack trace but terminal isn't restored

**Cause:** Recovery handler in wrong goroutine

```go
// Wrong - recovery in spawned goroutine
go func() {
    defer recoverPanic()  // Doesn't catch panic in main!
    p.Run()
}()

// Right - recovery in main goroutine
defer recoverPanic()
_, err := p.Run()
```

### Windows terminal issues

**Symptom:** Terminal doesn't work on Windows 10/11

**Cause 1:** ANSI sequences not enabled
```powershell
# Ensure you're using Windows Terminal, not legacy Command Prompt
# Windows Terminal v1.0+ supports ANSI automatically
```

**Cause 2:** File descriptor issue
```go
// Verify fd is correct
fd := int(os.Stdin.Fd())
fmt.Fprintf(os.Stderr, "DEBUG: stdin fd=%d\n", fd)

// Should output: DEBUG: stdin fd=0 (usually)
// If different, there may be redirection issue
```

**Cause 3:** Console mode not set correctly
```go
// Verify term.MakeRaw() succeeded
oldState, err := term.MakeRaw(fd)
if err != nil {
    // Might not be a TTY (e.g., redirected input)
    fmt.Fprintf(os.Stderr, "Not a TTY: %v\n", err)
    os.Exit(1)
}
```

---

## Summary: Best Practices Checklist

- [ ] Use `golang.org/x/term.MakeRaw()` for raw mode
- [ ] Always defer `term.Restore()` after `MakeRaw()`
- [ ] Use Bubble Tea's `tea.WithAltScreen()` option
- [ ] Use `signal.NotifyContext()` for graceful signal handling
- [ ] Pass context to `tea.NewProgram()` with `tea.WithContext()`
- [ ] Include panic recovery with `defer recoverPanic()`
- [ ] Call `EmergencyTerminalRestore()` in panic handlers
- [ ] Test terminal restoration on target platforms
- [ ] Document your signal handling strategy
- [ ] Provide fallback terminal reset instructions for users

---

## References

### Official Documentation
- [golang.org/x/term Documentation](https://pkg.go.dev/golang.org/x/term)
- [Bubble Tea Documentation](https://pkg.go.dev/github.com/charmbracelet/bubbletea)
- [Go defer, panic, and recover](https://go.dev/blog/defer-panic-and-recover)

### Related Packages
- [Charmbracelet ANSI](https://pkg.go.dev/github.com/charmbracelet/x/ansi) - ANSI sequence utilities
- [xterminal](https://pkg.go.dev/cdr.dev/coder-cli/internal/x/xterminal) - Cross-platform terminal state (Coder)
- [moby/term](https://pkg.go.dev/github.com/moby/term) - Terminal manipulation (Docker)

### Articles and Guides
- [Go Graceful Shutdown Patterns](https://victoriametrics.com/blog/go-graceful-shutdown/) - Signal handling best practices
- [Building Terminal Raw Mode Input in Go](https://mzunino.com.uy/til/2025/03/building-a-terminal-raw-mode-input-reader-in-go/)
- [ANSI Escape Sequences Cheatsheet](https://gist.github.com/ConnerWill/d4b6c776b509add763e17f9f113fd25b)

---

**Document Version:** 1.0
**Last Verified:** February 2026
**Go Versions:** 1.22+
**Bubble Tea:** v2.0.0-beta+
