package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMycroftSignalHelper(t *testing.T) {
	if os.Getenv("MYCROFT_SIGNAL_HELPER") != "1" {
		return
	}
	rootCmd.SetArgs([]string{"pre-dispatch"})
	main()
}

func TestPreDispatchTerminalSignal(t *testing.T) {
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			dir := t.TempDir()
			pidFile := filepath.Join(dir, "child.pid")
			check := filepath.Join(dir, "check")
			if err := os.WriteFile(check, []byte("#!/bin/sh\nsleep 30 &\necho $! > \"$1\"\nwait\n"), 0700); err != nil {
				t.Fatal(err)
			}
			config := fmt.Sprintf("pre_dispatch_check:\n  command: [%q, %q]\n", check, pidFile)
			if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestMycroftSignalHelper$")
			cmd.Env = append(os.Environ(), "MYCROFT_SIGNAL_HELPER=1", "MYCROFT_DATA_DIR="+dir)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			t.Cleanup(func() { _ = cmd.Process.Kill() })
			var pid int
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				data, _ := os.ReadFile(pidFile)
				pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
				if pid > 0 {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if pid <= 0 {
				t.Fatal("check did not start")
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
					t.Errorf("signal must cancel the check and exit 1, got %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("command did not exit after signal")
			}
			deadline = time.Now().Add(time.Second)
			for {
				data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
				if os.IsNotExist(err) || errors.Is(err, syscall.ESRCH) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				state := strings.Fields(string(data)[strings.LastIndex(string(data), ")")+1:])[0]
				if state == "Z" || state == "X" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("grandchild %d survived %s", pid, sig)
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
}
