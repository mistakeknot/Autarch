//go:build unix

package homeask

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFileRefusesRootRunFIFO(t *testing.T) {
	if path := os.Getenv("AUTARCH_ROOTRUN_FIFO_CHILD"); path != "" {
		f := newFake(t, registryOnCreate).filer()
		a := Ask{V: 1, Kind: "decide", Asker: "thread", Thread: "thr_a", Project: "sample", ProjectRoot: "/sample", Question: "Proceed?", Options: []Option{{ID: "a", Label: "A", Kind: "ruling-only"}, {ID: "b", Label: "B", Kind: "ruling-only"}}, RootRun: &RootRun{Script: path, SHA256: strings.Repeat("a", 64), Timeout: 30, Set: "sample"}}
		_, err := f.File(context.Background(), a)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "regular file") {
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
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFileRefusesRootRunFIFO$")
	cmd.Env = append(os.Environ(), "AUTARCH_ROOTRUN_FIFO_CHILD="+fifo)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("root-run read blocked on FIFO")
	}
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}
