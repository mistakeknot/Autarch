//go:build !windows

package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailureIncludesStderrTail(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\necho diagnostic-tail >&2\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("AUTARCH_AGENT", "claude")
	_, err := Run(context.Background(), t.TempDir(), []string{"-p", "hello"})
	if err == nil || !strings.Contains(err.Error(), "diagnostic-tail") {
		t.Fatalf("missing stderr tail: %v", err)
	}
}

func TestErrorResultAndStderrTail(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\necho '{\"type\":\"result\",\"is_error\":true,\"result\":\"please authenticate\"}'\necho diagnostic-tail >&2\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("AUTARCH_AGENT", "claude")
	_, err := Run(context.Background(), t.TempDir(), []string{"-p", "hello"})
	if err == nil || (!strings.Contains(err.Error(), "diagnostic-tail") || !strings.Contains(err.Error(), "please authenticate")) {
		t.Fatalf("missing stderr tail: %v", err)
	}
}

func TestErrorResultPrefix(t *testing.T) {
	for _, exit := range []string{"0", "7"} {
		t.Run(exit, func(t *testing.T) {
			dir := t.TempDir()
			script := "#!/bin/sh\necho '{\"type\":\"result\",\"is_error\":true,\"result\":\"please authenticate\"}'\nexit " + exit + "\n"
			if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			_, err := Run(context.Background(), dir, []string{"-p", "hello"})
			if err == nil || !strings.HasPrefix(err.Error(), "agent returned error: please authenticate") {
				t.Fatalf("inconsistent error prefix: %v", err)
			}
		})
	}
}
