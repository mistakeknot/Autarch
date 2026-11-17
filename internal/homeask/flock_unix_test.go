//go:build !windows

package homeask

import (
	"context"
	"os"
	"syscall"
	"testing"
)

// tryLock reports whether the lock at path could be taken right now (and releases it).
func tryLock(path string) bool {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false
	}
	defer f.Close()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return false
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return true
}

func TestCardFilerHoldsTheLockForTheWholeFiling(t *testing.T) {
	f := newFake(t, registryOnCreate)
	fl := f.filer()
	held := 0
	inner := fl.Run
	fl.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		p, _ := fl.lockPath(testReq().Key)
		if tryLock(p) {
			t.Errorf("lock not held during bb %v", args)
		} else {
			held++
		}
		return inner(ctx, env, args...)
	}
	if _, err := fl.FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
	if held < 5 {
		t.Fatalf("lock checked %d times", held)
	}
}
