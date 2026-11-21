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
	mode := os.Getenv("MYCROFT_SIGNAL_HELPER")
	if mode == "" {
		return
	}
	rootCmd.SetArgs([]string{"pre-dispatch"})
	if mode == "spawn" {
		rootCmd.SetArgs([]string{"spawn", "b1", "--card", "card", "--approve"})
	}
	main()
}

func TestSpawnTerminalSignalDuringSeatResolve(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	release := filepath.Join(dir, "release")
	called := filepath.Join(dir, "bb-called")
	// Pause the fake resolver until the test has delivered SIGTERM. No real
	// routing service or worker CLI is used in this subprocess. SIGTERM
	// orphans this shell, so the loop must end on its own: it stops when the
	// test's temp dir is gone and after about 30 seconds at most.
	ic := `#!/bin/sh
echo $$ > "$RESOLVE_READY"
n=0
while [ ! -e "$RESOLVE_RELEASE" ]; do
  [ -d "${RESOLVE_RELEASE%/*}" ] || exit 0
  n=$((n+1)); [ "$n" -lt 3000 ] || exit 1
  sleep 0.01
done
printf '%s\n' '{"requested_role":"routine-execution","profile_ref":"fixture","profile":{"backend":"codex","model":"fixture","reasoning_effort":"medium"},"policy_hash":"fixture","frontier_required":false}'
`
	bb := `#!/bin/sh
echo called >> "$BB_CALLED"
case "$1 $2" in
  'tasks show') printf '%s\n' '{"task":{"key":"card"}}' ;;
  'thread create') printf '%s\n' '{"id":"thr_fixture"}' ;;
esac
`
	for name, script := range map[string]string{"ic": ic, "bb": bb} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestMycroftSignalHelper$")
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"),
		"MYCROFT_SIGNAL_HELPER=spawn", "MYCROFT_DATA_DIR="+dir,
		"CLAVAIN_ROUTING_POLICY=fixture", "AUTARCH_BB_PROJECT=fixture", "AUTARCH_BB_ENVIRONMENT=fixture",
		"RESOLVE_READY="+ready, "RESOLVE_RELEASE="+release, "BB_CALLED="+called)
	// Run the helper in its own process group, so cleanup can also reach the
	// fake resolver it starts, which outlives the helper after SIGTERM.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = os.WriteFile(release, nil, 0600)
	})
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("spawn did not reach seat resolution")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("spawn must exit on SIGTERM, got %v", err)
		}
		status, ok := exitErr.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() || status.Signal() != syscall.SIGTERM {
			t.Errorf("spawn must retain default SIGTERM handling, got %v", err)
		}
	case <-time.After(time.Second):
		t.Error("spawn swallowed SIGTERM during seat resolution")
		// Let the defective version continue, so the test also observes any
		// worker CLI calls made after it should have exited.
		if err := os.WriteFile(release, nil, 0600); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("spawn did not finish after resolver release")
		}
	}
	if data, err := os.ReadFile(called); err == nil {
		t.Errorf("bb was called after SIGTERM: %s", data)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	// The fake resolver must not outlive the test.
	data, err := os.ReadFile(ready)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("resolver pid %q: %v", data, err)
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	deadline = time.Now().Add(3 * time.Second)
	for processLive(resolver) {
		if time.Now().After(deadline) {
			t.Fatalf("fake resolver %d still running after its process group was killed", resolver)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// processLive reports whether pid names a process that has not exited. A
// zombie waiting to be reaped counts as exited.
func processLive(pid int) bool {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	s := string(stat)
	i := strings.LastIndexByte(s, ')')
	return i < 0 || i+2 >= len(s) || s[i+2] != 'Z'
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
