package tui

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRestoreTerminalOnPanic_NoPanic(t *testing.T) {
	// Verify RestoreTerminalOnPanic is a no-op when there's no panic
	func() {
		defer RestoreTerminalOnPanic()
		// No panic — should return normally
	}()
}

func TestRestoreTerminalOnPanic_ResetsTerminalAndExits(t *testing.T) {
	if os.Getenv("TEST_PANIC_SUBPROCESS") == "1" {
		defer RestoreTerminalOnPanic()
		panic("test panic message")
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRestoreTerminalOnPanic_ResetsTerminalAndExits")
	cmd.Env = append(os.Environ(), "TEST_PANIC_SUBPROCESS=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d", exitErr.ExitCode())
	}

	output := stderr.String()
	if !strings.Contains(output, "\033[?1049l") {
		t.Error("stderr missing alt-screen disable sequence")
	}
	if !strings.Contains(output, "\033[?25h") {
		t.Error("stderr missing cursor show sequence")
	}
	if !strings.Contains(output, "test panic message") {
		t.Error("stderr missing panic message")
	}
}
