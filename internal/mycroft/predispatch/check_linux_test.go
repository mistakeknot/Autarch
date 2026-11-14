package predispatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

func TestPreDispatchKillsDescendant(t *testing.T) {
	for _, mode := range []string{"timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			pidFile := filepath.Join(dir, "child.pid")
			command := filepath.Join(dir, "check")
			// Keep the shell alive, rather than exec'ing its child as the old
			// timeout test does. The sleep is Run's grandchild.
			script := "#!/bin/sh\nsleep 30 &\necho $! > \"$1\"\nwait\n"
			if err := os.WriteFile(command, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			timeout := time.Second
			if mode == "cancel" {
				timeout = 0 // Exercise Run's default as well.
			}
			done := make(chan error, 1)
			go func() {
				done <- Run(ctx, mycroft.PreDispatchCheck{
					Command: []string{command, pidFile}, Timeout: mycroft.Duration(timeout),
				}, "b1", "agent")
			}()
			var pid int
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				data, err := os.ReadFile(pidFile)
				if err == nil {
					pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
					if pid > 0 {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
			}
			if pid <= 0 {
				t.Fatal("check did not report its child PID")
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			if !descendantRunning(t, pid) {
				t.Fatal("grandchild was not running before cancellation")
			}
			if mode == "cancel" {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("interrupted check succeeded")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Run did not return after interruption")
			}
			deadline = time.Now().Add(time.Second)
			for descendantRunning(t, pid) {
				if time.Now().After(deadline) {
					t.Fatalf("grandchild %d survived %s", pid, mode)
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
}

func descendantRunning(t *testing.T, pid int) bool {
	t.Helper()
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	// An orphan may remain a zombie until reaped by init; it cannot run.
	state := strings.Fields(string(data)[strings.LastIndex(string(data), ")")+1:])[0]
	return state != "Z" && state != "X"
}
