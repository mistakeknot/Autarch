package tui

import (
	"fmt"
	"os"
	"runtime/debug"
)

// RestoreTerminalOnPanic recovers from a panic, resets the terminal display,
// prints the panic value and stack trace to stderr, and calls os.Exit(1).
//
// Must be called via defer at the start of a TUI entry point:
//
//	defer pkgtui.RestoreTerminalOnPanic()
//
// Note: os.Exit bypasses all remaining deferred functions. Any cleanup
// defers registered after this one (e.g., logHandler.Close()) will not
// run on panic. This is acceptable since the process is terminating.
func RestoreTerminalOnPanic() {
	if r := recover(); r != nil {
		// CSI sequences to restore terminal:
		// \033[?1049l = disable alt-screen
		// \033[?25h   = show cursor
		// \033[0m     = reset attributes
		fmt.Fprint(os.Stderr, "\033[?1049l\033[?25h\033[0m\n")
		fmt.Fprintf(os.Stderr, "panic: %v\n\n", r)
		fmt.Fprint(os.Stderr, string(debug.Stack()))
		os.Exit(1)
	}
}
