//go:build unix

package homeask

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestEvidenceFIFO(t *testing.T) {
	if path := os.Getenv("AUTARCH_EVIDENCE_FIFO_PROBE"); path != "" {
		if _, err := evidenceRead(path); err == nil {
			t.Fatal("accepted FIFO")
		}
		// Also exercise the open after preflight: replacement by a FIFO must not block.
		f, err := openEvidenceFile(path)
		if err == nil {
			f.Close()
		}
		return
	}
	fifo := filepath.Join(t.TempDir(), "image.png")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEvidenceFIFO$", "-test.timeout=10s")
	cmd.Env = append(os.Environ(), "AUTARCH_EVIDENCE_FIFO_PROBE="+fifo)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("evidence read hung on FIFO: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("FIFO refusal: %v: %s", err, out)
	}
}
