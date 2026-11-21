package agenttargets

import (
	"context"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParentDeath(t *testing.T) {
	if file := os.Getenv("AGENT_PARENT_HELPER"); file != "" {
		cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "echo $$ > \"$1\"; exec /bin/sleep 60", "sh", file)
		configureProcess(cmd)
		if _, err := startProcess(cmd); err != nil {
			os.Exit(2)
		}
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		<-signals
	}
	file := filepath.Join(t.TempDir(), "pid")
	parent := exec.Command(os.Args[0], "-test.run=^TestParentDeath$")
	parent.Env = append(os.Environ(), "AGENT_PARENT_HELPER="+file)
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { parent.Process.Kill(); parent.Wait() }()
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(file)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("helper never ready")
	}
	// Open a stable process handle while the helper is alive, and verify
	// ownership before arming cleanup. Signals through the pidfd cannot target
	// a later process that reuses this PID.
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		t.Fatalf("open child pidfd: %v", err)
	}
	defer unix.Close(fd)
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	fields := strings.Fields(string(stat)[strings.LastIndex(string(stat), ")")+1:])
	if err != nil || len(fields) < 2 || fields[1] != strconv.Itoa(parent.Process.Pid) {
		t.Fatal("child ownership could not be verified")
	}
	defer unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, 0)

	parent.Process.Kill()
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		// An orphan can remain a zombie until the host reaps it; it cannot run.
		if os.IsNotExist(err) || strings.Contains(string(b), ") Z ") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("child still running after parent died")
}
