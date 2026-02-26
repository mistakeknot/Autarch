// File: docs/solutions/TERMINAL_RECOVERY_EXAMPLES.go
// Terminal State Restoration - Practical Code Examples
// DO NOT INCLUDE IN BUILD - Reference implementation only

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// ============================================================================
// EXAMPLE 1: Basic Terminal State Management
// ============================================================================

// Example1_BasicRawMode demonstrates the simplest raw mode pattern
func Example1_BasicRawMode() {
	fd := int(os.Stdin.Fd())

	// Enable raw mode - capture previous state
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to enable raw mode: %v\n", err)
		os.Exit(1)
	}

	// CRITICAL: defer ensures restoration even on panic
	defer func() {
		if err := term.Restore(fd, oldState); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to restore terminal: %v\n", err)
		}
	}()

	// Now terminal is in raw mode
	// Your application runs here
}

// ============================================================================
// EXAMPLE 2: Terminal State Wrapper Type
// ============================================================================

// TerminalState encapsulates terminal state management
type TerminalState struct {
	oldState *term.State
	fd       int
	restored bool
	mu       sync.Mutex
}

// NewTerminalState creates and enables raw mode
func NewTerminalState() (*TerminalState, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("MakeRaw failed: %w", err)
	}

	return &TerminalState{
		oldState: oldState,
		fd:       fd,
		restored: false,
	}, nil
}

// Restore returns terminal to previous state (safe to call multiple times)
func (ts *TerminalState) Restore() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.restored {
		return nil // Already restored
	}

	if err := term.Restore(ts.fd, ts.oldState); err != nil {
		return fmt.Errorf("Restore failed: %w", err)
	}

	ts.restored = true
	return nil
}

// Example2_WrapperUsage shows how to use the wrapper
func Example2_WrapperUsage() {
	ts, err := NewTerminalState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed: %v\n", err)
		os.Exit(1)
	}
	defer ts.Restore()

	// Terminal is now in raw mode
	// Application code here
}

// ============================================================================
// EXAMPLE 3: Panic Recovery with Terminal Restoration
// ============================================================================

// EmergencyTerminalRestore sends minimal ANSI sequences to fix terminal
// Use this in panic handlers when normal restoration may have failed
func EmergencyTerminalRestore() error {
	// Sequences:
	// \x1b[?25h   - Show cursor
	// \x1b[?1049l - Exit alt-screen (if active)
	// \x1b(B      - Reset character set to ASCII
	// \x1b[m      - Reset all attributes
	// \n\r        - Move to next line

	sequences := "\x1b[?25h\x1b[?1049l\x1b(B\x1b[m\n\r"

	// Try stderr first (more likely to be terminal)
	_, err := fmt.Fprint(os.Stderr, sequences)
	if err != nil {
		// Fallback to stdout
		_, _ = fmt.Fprint(os.Stdout, sequences)
	}
	return err
}

// SafeMain wraps main function with panic recovery
func SafeMain(fn func() error) error {
	defer func() {
		if r := recover(); r != nil {
			// Terminal may be corrupted - restore it immediately
			EmergencyTerminalRestore()

			// Log the panic
			fmt.Fprintf(os.Stderr, "\n=== PANIC ===\n")
			fmt.Fprintf(os.Stderr, "Error: %v\n", r)
			fmt.Fprintf(os.Stderr, "Stack trace:\n")
			debug.PrintStack()

			os.Exit(1)
		}
	}()

	return fn()
}

// Example3_PanicRecovery demonstrates panic recovery
func Example3_PanicRecovery() {
	SafeMain(func() error {
		ts, err := NewTerminalState()
		if err != nil {
			return err
		}
		defer ts.Restore()

		// If panic occurs here, EmergencyTerminalRestore() will run
		// before the process exits
		_ = ts

		return nil
	})
}

// ============================================================================
// EXAMPLE 4: Signal Handling with Context (Recommended 2026 Pattern)
// ============================================================================

// Example4_SignalHandling demonstrates modern signal handling with context
func Example4_SignalHandling(model tea.Model) error {
	// Create context that cancels on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT,  // Ctrl+C
		syscall.SIGTERM, // Kill signal
	)
	defer stop() // Stop listening for signals

	// Enable raw mode
	ts, err := NewTerminalState()
	if err != nil {
		return err
	}
	defer ts.Restore()

	// Create program with context
	// Bubble Tea will gracefully shutdown when context is cancelled
	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithContext(ctx),
		tea.WithMouseAllMotion(),
	)

	_, err = p.Run()

	// When p.Run() returns, ctx has been cancelled (signal received)
	// All defers execute (terminal is restored)

	return err
}

// ============================================================================
// EXAMPLE 5: Custom Signal Handling (Double Ctrl+C Pattern)
// ============================================================================

// Model for custom signal handling
type CustomSignalModel struct {
	sigChan     chan os.Signal
	lastSigTime time.Time
	shouldQuit  bool
}

// Update handles custom signal logic
func (m CustomSignalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Bubble Tea sends KeyMsg for Ctrl+C when using WithoutSignalHandler
		if msg.Type == tea.KeyCtrlC {
			now := time.Now()

			// Check if this is the second Ctrl+C within 500ms
			if now.Sub(m.lastSigTime) < 500*time.Millisecond {
				return m, tea.Quit // Second press - quit immediately
			}

			// First press - show warning
			m.lastSigTime = now
			fmt.Fprintf(os.Stderr, "Press Ctrl+C again to quit...\n")
			return m, nil
		}
	}
	return m, nil
}

func (m CustomSignalModel) Init() tea.Cmd  { return nil }
func (m CustomSignalModel) View() string   { return "Custom signal handling\n" }

// Example5_DoubleCtrlC demonstrates double-press-to-quit
func Example5_DoubleCtrlC() error {
	ts, err := NewTerminalState()
	if err != nil {
		return err
	}
	defer ts.Restore()

	m := CustomSignalModel{
		sigChan:     make(chan os.Signal, 1),
		lastSigTime: time.Now().Add(-1 * time.Second), // Initialize to old time
	}

	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithoutSignalHandler(), // Handle signals ourselves
	)

	// Optionally set up signal listening in Update()
	// (simplified version shown; real implementation would use channels)

	_, err = p.Run()
	return err
}

// ============================================================================
// EXAMPLE 6: Testing Terminal Restoration
// ============================================================================

// MockTerminal simulates a terminal for testing
type MockTerminal struct {
	rawMode bool
	altScr  bool
	mu      sync.Mutex
}

// TestTerminalRestoration demonstrates testing terminal state
func TestTerminalRestoration() error {
	// Pseudo-test code
	fmt.Println("Testing terminal restoration:")

	// 1. Verify initial state
	fmt.Println("1. Terminal state before: normal")

	// 2. Enable raw mode
	ts, err := NewTerminalState()
	if err != nil {
		if err.Error() == "not a terminal" {
			fmt.Println("   (Skipping - not running in TTY)")
			return nil
		}
		return err
	}

	fmt.Println("2. Terminal state after raw mode: RAW")

	// 3. Restore
	if err := ts.Restore(); err != nil {
		return err
	}

	fmt.Println("3. Terminal state after restore: normal")
	fmt.Println("   ✓ Test passed")

	return nil
}

// ============================================================================
// EXAMPLE 7: Goroutine-Safe Signal Handling
// ============================================================================

// SignalHandlerPool manages signals for multiple goroutines
type SignalHandlerPool struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSignalHandlerPool creates a signal handler pool
func NewSignalHandlerPool() *SignalHandlerPool {
	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	return &SignalHandlerPool{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start launches a worker
func (p *SignalHandlerPool) Start(fn func(context.Context) error) error {
	p.wg.Add(1)

	go func() {
		defer p.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "Worker panic: %v\n", r)
				EmergencyTerminalRestore()
			}
		}()

		// Pass context to worker - it will stop when context is cancelled
		if err := fn(p.ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Worker error: %v\n", err)
		}
	}()

	return nil
}

// Wait blocks until all workers exit or context is cancelled
func (p *SignalHandlerPool) Wait() {
	p.wg.Wait()
}

// Close cancels all workers
func (p *SignalHandlerPool) Close() {
	p.cancel()
	p.Wait()
}

// Example7_GoroutinePool demonstrates goroutine-safe signal handling
func Example7_GoroutinePool() error {
	pool := NewSignalHandlerPool()
	defer pool.Close()

	// Start multiple workers
	pool.Start(func(ctx context.Context) error {
		for {
			select {
			case <-ctx.Done():
				return nil // Context cancelled, exit gracefully
			case <-time.After(1 * time.Second):
				fmt.Println("Worker tick")
			}
		}
	})

	pool.Wait()
	return nil
}

// ============================================================================
// EXAMPLE 8: Cross-Platform Terminal Detection
// ============================================================================

// IsTerminal checks if stdout is a terminal
func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// IsInputTerminal checks if stdin is a terminal
func IsInputTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// DetectTerminalCapabilities returns terminal info
func DetectTerminalCapabilities() map[string]interface{} {
	return map[string]interface{}{
		"os":                 os.Getenv("OS"), // "Linux", "Darwin", "Windows"
		"terminal":           os.Getenv("TERM"),
		"is_terminal":        IsTerminal(),
		"is_input_terminal":  IsInputTerminal(),
		"supports_ansi":      supportsANSI(),
		"supports_256color":  supports256Color(),
		"unicode_capable":    supportsUnicode(),
	}
}

func supportsANSI() bool {
	// Most modern terminals support ANSI
	term := os.Getenv("TERM")
	return term != "" && term != "dumb"
}

func supports256Color() bool {
	term := os.Getenv("TERM")
	return term == "xterm-256color" || term == "screen-256color"
}

func supportsUnicode() bool {
	// Simplified check
	return os.Getenv("LANG") != ""
}

// Example8_DetectCapabilities shows terminal capability detection
func Example8_DetectCapabilities() {
	caps := DetectTerminalCapabilities()
	for key, val := range caps {
		fmt.Printf("%-20s: %v\n", key, val)
	}
}

// ============================================================================
// EXAMPLE 9: Production-Ready Main Function
// ============================================================================

// ProductionMain is a template for production TUI applications
func ProductionMain(model tea.Model) error {
	// 1. Set up signal handling with context
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	// 2. Enable raw mode if running in terminal
	var ts *TerminalState
	if IsInputTerminal() {
		var err error
		ts, err = NewTerminalState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to enable raw mode: %v\n", err)
			return err
		}
		defer ts.Restore()
	}

	// 3. Panic recovery
	defer func() {
		if r := recover(); r != nil {
			EmergencyTerminalRestore()
			fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
			debug.PrintStack()
			os.Exit(1)
		}
	}()

	// 4. Create and run Bubble Tea program
	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithContext(ctx),
		tea.WithMouseAllMotion(),
	)

	_, err := p.Run()

	// 5. All defers execute (terminal restored, signals stopped)
	return err
}

// ============================================================================
// EXAMPLE 10: Emergency Terminal Recovery for External Panics
// ============================================================================

// ExternalProcessRunner runs external commands with terminal cleanup
type ExternalProcessRunner struct {
	ts *TerminalState
}

// NewExternalProcessRunner creates a runner with saved terminal state
func NewExternalProcessRunner() (*ExternalProcessRunner, error) {
	ts, err := NewTerminalState()
	if err != nil {
		return nil, err
	}

	return &ExternalProcessRunner{ts: ts}, nil
}

// Run executes fn with guaranteed terminal restoration
func (r *ExternalProcessRunner) Run(fn func() error) error {
	defer r.ts.Restore()

	// If fn panics, ts.Restore() in defer will still execute
	return fn()
}

// Example10_ExternalProcesses shows running external code safely
func Example10_ExternalProcesses() error {
	runner, err := NewExternalProcessRunner()
	if err != nil {
		return err
	}

	return runner.Run(func() error {
		// External code that might panic
		// Terminal will be restored regardless
		fmt.Println("Running external function...")
		return nil
	})
}
