package agenttargets

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
		select {}
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
	defer func() {
		if p, e := os.FindProcess(pid); e == nil {
			p.Kill()
		}
	}()
	parent.Process.Kill()
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
