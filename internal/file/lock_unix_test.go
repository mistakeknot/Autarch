//go:build !windows

package file

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAtomicWriteFileBlocksWhenLocked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")

	lock, err := LockFile(path)
	if err != nil {
		t.Fatalf("unexpected lock error: %v", err)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("unexpected executable error: %v", err)
	}

	cmd := exec.Command(exe, "-test.run=TestLockFileHelper", "-test.v")
	cmd.Env = append(os.Environ(), "TAND_LOCK_HELPER=1", "TAND_LOCK_PATH="+path)
	if err := cmd.Start(); err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		_ = lock.Unlock()
		if err == nil {
			t.Fatal("expected write to block while locked")
		}
		return
	case <-time.After(50 * time.Millisecond):
	}

	if err := lock.Unlock(); err != nil {
		t.Fatalf("unexpected unlock error: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected helper to finish after unlock: %v", err)
		}
	case <-time.After(1 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("expected write to finish after unlock")
	}
}

func TestLockFileHelper(t *testing.T) {
	if os.Getenv("TAND_LOCK_HELPER") != "1" {
		t.Skip("helper only")
	}
	path := os.Getenv("TAND_LOCK_PATH")
	if path == "" {
		t.Fatal("missing lock path")
	}
	lock, err := LockFile(path)
	if err != nil {
		t.Fatalf("unexpected lock error: %v", err)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatalf("unexpected unlock error: %v", err)
	}
}
