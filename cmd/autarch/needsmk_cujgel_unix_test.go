//go:build unix

package main

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestNeedsMkCujgelFIFO(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_fixture")
	p := filepath.Join(t.TempDir(), "link")
	if err := syscall.Mkfifo(p, 0600); err != nil {
		t.Fatal(err)
	}
	s := &stubBB{linkedBBProject: "proj_fixture"}
	_, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Compare variants", "--ask-file", writeAsk(t), "--cujgel", p)
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "regular file") || s.creates != 0 {
		t.Fatalf("fifo not refused clearly: %v", err)
	}
}
