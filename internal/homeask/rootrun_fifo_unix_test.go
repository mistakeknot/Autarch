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

func TestRootRunFIFOOpenDescriptorIsRejected(t *testing.T) {
	if path := os.Getenv("AUTARCH_ROOTRUN_OPEN_FIFO_CHILD"); path != "" {
		file, err := openRootRunScript(path)
		if err != nil {
			t.Fatalf("did not reach FIFO open: %v", err)
		}
		info, err := file.Stat()
		file.Close()
		if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
			t.Fatalf("did not open a FIFO descriptor: %v %v", info, err)
		}
		if _, err := HashRootRunScript(path); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("FIFO descriptor accepted for hashing: %v", err)
		}
		return
	}
	fifo := filepath.Join(t.TempDir(), "script.fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRootRunFIFOOpenDescriptorIsRejected$")
	cmd.Env = append(os.Environ(), "AUTARCH_ROOTRUN_OPEN_FIFO_CHILD="+fifo)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("opening FIFO descriptor blocked")
	}
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}
