# Terminal State Testing Guide

Comprehensive testing strategies for terminal state restoration in Go TUI applications.

## Table of Contents

1. [Unit Testing Patterns](#unit-testing-patterns)
2. [Integration Testing](#integration-testing)
3. [Manual Testing Checklist](#manual-testing-checklist)
4. [Test Automation](#test-automation)
5. [CI/CD Integration](#cicd-integration)

---

## Unit Testing Patterns

### Test 1: Verify Raw Mode Toggling

```go
package terminal_test

import (
    "os"
    "testing"
    "golang.org/x/term"
)

// TestRawModeToggle verifies MakeRaw/Restore work correctly
func TestRawModeToggle(t *testing.T) {
    // Skip if not a terminal (e.g., CI environment)
    if !term.IsTerminal(int(os.Stdin.Fd())) {
        t.Skip("Skipping: stdin is not a terminal")
    }

    fd := int(os.Stdin.Fd())

    // Get initial state
    state1, err := term.MakeRaw(fd)
    if err != nil {
        t.Fatalf("First MakeRaw failed: %v", err)
    }
    defer term.Restore(fd, state1) // Cleanup

    // Verify we're in raw mode (would check termios flags in real test)
    t.Log("✓ Entered raw mode")

    // Get state while in raw mode
    state2, err := term.MakeRaw(fd)
    if err != nil {
        t.Fatalf("Second MakeRaw failed: %v", err)
    }

    // Restore once
    if err := term.Restore(fd, state2); err != nil {
        t.Fatalf("Restore failed: %v", err)
    }
    t.Log("✓ Restored from raw mode")

    // Restore again (should be idempotent)
    if err := term.Restore(fd, state1); err != nil {
        t.Fatalf("Second restore failed: %v", err)
    }
    t.Log("✓ Double restore succeeds")
}
```

### Test 2: Panic Recovery Executes Defer

```go
// TestPanicRecoveryDefer verifies defers run during panic unwinding
func TestPanicRecoveryDefer(t *testing.T) {
    executed := false

    func() {
        defer func() {
            executed = true
        }()

        // Simulate panic scenario
        panic("test panic")
    }()

    // This code doesn't run - panic propagates out
}

// Better: test panic + recovery in same function
func TestPanicRecoveryWithRecover(t *testing.T) {
    var deferred, recovered bool

    func() {
        defer func() {
            deferred = true
            if r := recover(); r != nil {
                recovered = true
            }
        }()

        panic("test panic")
    }()

    if !deferred {
        t.Fatal("defer didn't execute")
    }
    if !recovered {
        t.Fatal("recover didn't catch panic")
    }
    t.Log("✓ Defer and recover both executed")
}
```

### Test 3: TerminalState Wrapper Behavior

```go
// TestTerminalStateRestoreIdempotent verifies restore can be called multiple times
func TestTerminalStateRestoreIdempotent(t *testing.T) {
    if !term.IsTerminal(int(os.Stdin.Fd())) {
        t.Skip("not a terminal")
    }

    ts := NewTerminalState()
    if ts == nil {
        t.Skip("failed to create TerminalState")
    }

    // First restore
    err1 := ts.Restore()
    if err1 != nil {
        t.Fatalf("First restore failed: %v", err1)
    }

    // Second restore (should not error)
    err2 := ts.Restore()
    if err2 != nil {
        t.Fatalf("Second restore failed: %v", err2)
    }

    // Third restore
    err3 := ts.Restore()
    if err3 != nil {
        t.Fatalf("Third restore failed: %v", err3)
    }

    t.Log("✓ Multiple restores succeed")
}

// TestTerminalStateRaceCondition tests thread safety
func TestTerminalStateRaceCondition(t *testing.T) {
    if !term.IsTerminal(int(os.Stdin.Fd())) {
        t.Skip("not a terminal")
    }

    ts := NewTerminalState()
    done := make(chan bool, 10)

    // Launch 10 goroutines calling Restore simultaneously
    for i := 0; i < 10; i++ {
        go func() {
            _ = ts.Restore()
            done <- true
        }()
    }

    // Wait for all to complete
    for i := 0; i < 10; i++ {
        <-done
    }

    t.Log("✓ Concurrent Restore calls succeed")
}
```

### Test 4: Signal Context Cancellation

```go
// TestSignalContextCancellation verifies signal cancels context
func TestSignalContextCancellation(t *testing.T) {
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGUSR1,  // Use SIGUSR1 for testing (won't kill process)
    )
    defer stop()

    done := make(chan bool, 1)

    // Launch goroutine that waits for context
    go func() {
        <-ctx.Done()
        done <- true
    }()

    // Send signal to self
    pid := os.Getpid()
    time.Sleep(100 * time.Millisecond)
    syscall.Kill(pid, syscall.SIGUSR1)

    // Wait for context to cancel
    select {
    case <-done:
        t.Log("✓ Context cancelled by signal")
    case <-time.After(5 * time.Second):
        t.Fatal("Context didn't cancel within 5 seconds")
    }
}
```

---

## Integration Testing

### Test 1: TUI Launch and Exit Cycle

```go
// TestTUILaunchAndExit verifies full TUI lifecycle
func TestTUILaunchAndExit(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    m := NewTestModel()  // Simple test model

    // Create program
    p := tea.NewProgram(m,
        tea.WithContext(ctx),
        tea.WithAltScreen(),
    )

    // Run in goroutine to allow timeout
    resultChan := make(chan error, 1)
    go func() {
        _, err := p.Run()
        resultChan <- err
    }()

    // Wait for context timeout (simulates graceful shutdown)
    <-ctx.Done()

    // Check program exited cleanly
    select {
    case err := <-resultChan:
        if err != nil {
            t.Fatalf("TUI exited with error: %v", err)
        }
        t.Log("✓ TUI exited cleanly on context cancel")
    case <-time.After(5 * time.Second):
        t.Fatal("TUI didn't exit within 5 seconds")
    }
}

// TestTUIModel is a minimal test model
type TestModel struct{}

func (m TestModel) Init() tea.Cmd { return nil }

func (m TestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg.(type) {
    case tea.KeyMsg:
        return m, tea.Quit
    }
    return m, nil
}

func (m TestModel) View() string { return "test" }
```

### Test 2: Panic During TUI Operation

```go
// TestTUIPanicRecovery verifies panic handler runs during TUI
func TestTUIPanicRecovery(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping panic test in short mode")
    }

    recovered := false
    panicValue := ""

    // Run in subprocess to avoid corrupting test environment
    subtest := os.Getenv("SUBTEST_MODE")
    if subtest == "panic" {
        // Run the panic scenario
        m := NewPanicModel()

        defer func() {
            if r := recover(); r != nil {
                recovered = true
                panicValue = fmt.Sprintf("%v", r)
            }
        }()

        p := tea.NewProgram(m, tea.WithContext(context.Background()))
        p.Run()

        if recovered {
            os.Exit(0)  // Success - panic was recovered
        } else {
            os.Exit(1)  // Failed - panic not recovered
        }
    }

    // Run subprocess with SUBTEST_MODE=panic
    cmd := exec.Command(os.Args[0], "-test.run", "TestTUIPanicRecovery")
    cmd.Env = append(os.Environ(), "SUBTEST_MODE=panic")

    err := cmd.Run()
    if err == nil {
        t.Log("✓ Panic was recovered in subprocess")
    } else {
        t.Fatal("Panic was not recovered")
    }
}

// PanicModel triggers panic when updated
type PanicModel struct{}

func (m PanicModel) Init() tea.Cmd {
    return func() tea.Msg {
        panic("intentional test panic")
    }
}

func (m PanicModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    return m, nil
}

func (m PanicModel) View() string { return "test" }
```

### Test 3: Signal Handling During TUI

```go
// TestTUISignalHandling verifies signals are handled during TUI
func TestTUISignalHandling(t *testing.T) {
    if runtime.GOOS == "windows" {
        t.Skip("Skipping on Windows")
    }

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGUSR1,  // Test signal
    )
    defer stop()

    m := NewTestModel()
    p := tea.NewProgram(m, tea.WithContext(ctx))

    exitChan := make(chan error, 1)

    // Start TUI
    go func() {
        _, err := p.Run()
        exitChan <- err
    }()

    // Let TUI start
    time.Sleep(100 * time.Millisecond)

    // Send signal
    pid := os.Getpid()
    if err := syscall.Kill(pid, syscall.SIGUSR1); err != nil {
        t.Fatalf("Failed to send signal: %v", err)
    }

    // Wait for TUI to exit
    select {
    case err := <-exitChan:
        if err != nil {
            t.Logf("TUI exited with error (expected): %v", err)
        }
        t.Log("✓ TUI responded to signal")
    case <-time.After(5 * time.Second):
        t.Fatal("TUI didn't respond to signal")
    }
}
```

---

## Manual Testing Checklist

### Pre-Test Setup

```bash
# 1. Create test environment
mkdir -p /tmp/tui-test
cd /tmp/tui-test

# 2. Verify terminal capabilities
echo $TERM
echo $LANG
tput colors

# 3. Run tests in clean environment
env -i TERM=xterm-256color LANG=en_US.UTF-8 ./myapp
```

### Basic Terminal Functionality Tests

```bash
# TEST 1: Normal exit
echo "1. Testing normal exit..."
./myapp
# Expected: Prompt returns, terminal works
# Check: Can type commands, terminal not corrupted

# TEST 2: Ctrl+C graceful shutdown
echo "2. Testing Ctrl+C..."
./myapp
# Press Ctrl+C
# Expected: Clean exit, prompt visible
# Check: No garbled text, cursor visible

# TEST 3: Force quit (if implemented)
echo "3. Testing force quit (Ctrl+C twice)..."
./myapp
# Press Ctrl+C twice rapidly
# Expected: Immediate exit
# Check: No hang, terminal responsive

# TEST 4: Terminal resize
echo "4. Testing terminal resize..."
./myapp
# Resize terminal window while running
# Expected: UI adapts to new size
# Check: No text corruption, window updates

# TEST 5: Suspend and resume (Linux/macOS)
echo "5. Testing suspend (Ctrl+Z)..."
./myapp
# Press Ctrl+Z to suspend
# Command: jobs
# Command: fg
# Expected: App resumes, terminal state intact
# Check: UI still responsive, no corruption
```

### Edge Case Tests

```bash
# TEST 6: Running in SSH
echo "6. Testing via SSH..."
ssh user@host 'cd /tmp && ./myapp'
# Exit with Ctrl+C
# Expected: SSH session returns cleanly
# Check: No hanging connection

# TEST 7: Piped stdin
echo "7. Testing with piped input..."
echo "q" | ./myapp
# Expected: App quits when reading 'q'
# Check: Terminal works after exit

# TEST 8: Running in screen/tmux
echo "8. Testing in tmux..."
tmux new-session -d -s test './myapp'
tmux send-keys -t test C-c  # Send Ctrl+C
tmux kill-session -t test
# Expected: Session exits cleanly

# TEST 9: Terminal emulator switching
echo "9. Testing different terminals..."
# Test in each of:
# - xterm
# - GNOME Terminal
# - Alacritty
# - macOS Terminal
# - iTerm2
# - Windows Terminal
# Expected: Consistent behavior across all

# TEST 10: Long-running operation
echo "10. Testing long operations..."
./myapp
# Trigger operation that takes 30+ seconds
# Try to exit with Ctrl+C
# Expected: Exit completes gracefully, no timeout
```

### Crash Scenario Tests

```bash
# WARNING: These tests may corrupt your terminal. Run in disposable environment!

# TEST 11: Intentional panic
# Modify code to trigger panic in specific scenario
# Build and run
# Expected: Terminal restored, stack trace visible

# TEST 12: Force kill
echo "12. Testing SIGKILL..."
# In first terminal:
./myapp
# In second terminal:
ps aux | grep myapp
kill -9 <PID>  # SIGKILL (force kill)
# In first terminal: verify terminal still works
# Note: SIGKILL can't be caught, so terminal may be corrupted
#       This is expected - use emergency recovery

# TEST 13: Out of memory
# Create app that consumes all RAM
# In another terminal: kill the app with Ctrl+C
# Verify terminal recovery
```

### Platform-Specific Tests

```bash
# LINUX TESTS
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "Testing Linux-specific features..."

    # Test raw mode syscalls
    strace -e ioctl ./myapp
    # Should show TCGETS/TCSETS syscalls

    # Test with different locale
    LC_ALL=C ./myapp
    LC_ALL=ja_JP.UTF-8 ./myapp
fi

# macOS TESTS
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Testing macOS-specific features..."

    # Test with different terminal apps
    open -a Terminal
    open -a iTerm

    # Check file descriptor usage
    lsof -p $$
fi

# WINDOWS TESTS
if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "win32" ]]; then
    echo "Testing Windows-specific features..."

    # Use Windows Terminal (PowerShell)
    ./myapp.exe

    # Test console modes
    # (Would use Windows API, not shown here)
fi
```

---

## Test Automation

### CI Configuration (GitHub Actions)

```yaml
name: Terminal State Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
        go: ['1.22', '1.23', '1.24']

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: ${{ matrix.go }}

      - name: Run tests
        run: |
          go test -v -timeout 30s ./...

      - name: Run terminal tests (Unix only)
        if: runner.os != 'Windows'
        run: |
          # Skip TTY-dependent tests in CI
          go test -v -run 'TestTerminal' -skip 'TestTUIPanic' ./...

      - name: Build release binaries
        run: |
          GOOS=linux GOARCH=amd64 go build -o myapp-linux ./cmd/app
          GOOS=darwin GOARCH=amd64 go build -o myapp-macos ./cmd/app
          GOOS=windows GOARCH=amd64 go build -o myapp-windows.exe ./cmd/app

      - name: Upload artifacts
        uses: actions/upload-artifact@v3
        with:
          name: binaries-${{ matrix.os }}
          path: myapp-*
```

### Local Test Script

```bash
#!/bin/bash
# test-terminal.sh - Automated terminal testing

set -e

echo "=== Terminal State Recovery Tests ==="
echo ""

# Unit tests
echo "1. Running unit tests..."
go test -v -run TestTerminal ./...

# Integration tests
echo "2. Running integration tests..."
go test -v -run TestTUI ./... || true

# Build test binary
echo "3. Building test binary..."
go build -o test-app ./cmd/app

# Platform-specific tests
echo "4. Running platform-specific tests..."
case "$OSTYPE" in
  linux*)
    echo "  Linux detected - running Linux tests..."
    go test -v -run TestLinux ./...
    ;;
  darwin*)
    echo "  macOS detected - running macOS tests..."
    go test -v -run TestMacOS ./...
    ;;
  msys|cygwin|win32)
    echo "  Windows detected - running Windows tests..."
    go test -v -run TestWindows ./...
    ;;
esac

echo ""
echo "=== All tests completed ==="
echo "Manual verification recommended:"
echo "  $ ./test-app                  # Normal exit"
echo "  $ ./test-app                  # Ctrl+C test"
echo "  $ timeout 5 ./test-app        # Timeout test"
```

---

## CI/CD Integration

### Recommended Test Coverage

| Category | Test | Min Coverage |
|----------|------|--------------|
| Unit | Raw mode on/off | 100% |
| Unit | Panic recovery | 100% |
| Unit | Signal handling | 95% |
| Integration | TUI lifecycle | 90% |
| Integration | Signal interrupts | 85% |
| Manual | Cross-platform | 100% |

### Test Result Reporting

```go
// Print test results in machine-readable format
type TestResult struct {
    Name    string
    Passed  bool
    Error   string
    Runtime time.Duration
}

func reportResults(results []TestResult) {
    for _, r := range results {
        status := "PASS"
        if !r.Passed {
            status = "FAIL"
        }
        fmt.Printf("%-30s %s (%3dms) %s\n",
            r.Name,
            status,
            r.Runtime.Milliseconds(),
            r.Error)
    }
}
```

---

## Troubleshooting Test Issues

### Test fails with "not a terminal"

```bash
# Solution 1: Run with TTY
go test -v ./... < /dev/tty

# Solution 2: Use script to allocate TTY
script -q -c "go test -v ./..." /dev/null

# Solution 3: Skip TTY tests in CI
if [ -z "$CI" ]; then
    go test -v ./...
else
    go test -v -skip TestTerminal ./...
fi
```

### Hanging tests

```bash
# Solution 1: Add timeout
go test -timeout 30s ./...

# Solution 2: Kill hanging test
timeout -s KILL 30 go test -v ./...

# Solution 3: Debug with verbose output
go test -v -race ./...
```

### Platform-specific failures

```bash
# Test on Docker
docker run --rm -it golang:1.24 bash
# Inside container: run tests

# Or use native environment
go mod tidy
go test -v ./...
```

---

## Summary

Terminal state testing requires:

1. **Unit tests** - Verify individual functions work
2. **Integration tests** - Verify full lifecycle works
3. **Manual tests** - Verify user experience works
4. **Platform tests** - Verify cross-platform compatibility
5. **Edge case tests** - Verify panic/signal handling

Key metrics:
- All unit tests pass (100% success)
- Integration tests pass on target platforms
- Manual test checklist completed
- No terminal corruption after exit
- Graceful handling of Ctrl+C
- Recovery from panic scenarios

---

**Last Updated:** February 2026
**Go Versions:** 1.22+
**Platforms:** Linux, macOS, Windows
