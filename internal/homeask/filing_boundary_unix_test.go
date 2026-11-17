//go:build !windows

package homeask

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestMondayBoundaryQueueChecksRunOutsideLocks(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.StateDir = t.TempDir()
	before := pacificTestTime(t, "2026-11-15 23:59:59")
	after := pacificTestTime(t, "2026-11-16 00:00:00")
	f.Now = func() time.Time { return before }
	enablePilot(t, f, "daycap")
	var now atomic.Int64
	now.Store(before.UnixNano())
	clockRead := make(chan struct{})
	var signal sync.Once
	f.Now = func() time.Time { at := time.Unix(0, now.Load()); signal.Do(func() { close(clockRead) }); return at }
	unlock, err := lockFile(filepath.Join(f.StateDir, "filing.lock"))
	if err != nil {
		t.Fatal(err)
	}
	var release sync.Once
	defer release.Do(unlock)
	var checked atomic.Bool
	f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if strings.Join(args[:min(len(args), 3)], " ") == "tasks list --status" {
			checked.Store(true)
			for _, name := range []string{"filing.lock", "pilot-delivery.lock"} {
				fd, err := os.OpenFile(filepath.Join(f.StateDir, name), os.O_CREATE|os.O_RDWR, 0600)
				if err != nil {
					t.Error(err)
					continue
				}
				err = syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
				if err != nil {
					t.Errorf("queue validation held %s", name)
				} else {
					syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
				}
				fd.Close()
			}
		}
		return bb.run(ctx, env, args...)
	}
	req := testReq()
	req.Ask["ask_key"] = "boundary"
	done := make(chan error, 1)
	go func() {
		result, err := f.FileOrUpdateCard(context.Background(), req, "")
		if err == nil && !result.Queued {
			t.Error("crossing did not hold the ordinary request")
		}
		done <- err
	}()
	select {
	case <-clockRead:
	case <-time.After(5 * time.Second):
		t.Fatal("initial clock not read")
	}
	now.Store(after.UnixNano())
	release.Do(unlock)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("admission retry deadlocked")
	}
	if !checked.Load() {
		t.Fatal("queue prerequisites were not checked")
	}
}
