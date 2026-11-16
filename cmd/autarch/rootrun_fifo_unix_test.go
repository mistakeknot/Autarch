//go:build unix

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRootRunFlagRefusesFIFO(t *testing.T) {
	if path := os.Getenv("AUTARCH_ROOTRUN_FIFO_CHILD"); path != "" {
		_, err := parseRootRunFlag("script=" + path + ",timeout=30,set=sample")
		if err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatal(err)
		}
		return
	}
	fifo := filepath.Join(t.TempDir(), "script.fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRootRunFlagRefusesFIFO$")
	cmd.Env = append(os.Environ(), "AUTARCH_ROOTRUN_FIFO_CHILD="+fifo)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("root-run read blocked on FIFO")
	}
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestRootRunFlagRefusesOversizedScript(t *testing.T) {
	p := filepath.Join(t.TempDir(), "large.sh")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(8 << 20); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := parseRootRunFlag("script=" + p + ",timeout=30,set=sample"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatal("oversized script not refused:", err)
	}
}
