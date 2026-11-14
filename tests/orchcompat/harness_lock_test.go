package orchcompat

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestHarnessUpdatePreservesCompetingWrite(t *testing.T) {
	e := newEnv(t)
	_, _, path := fbPaths(e.fbdir)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	write := func() {
		t.Helper()
		s, err := fbLoad(e.fbdir)
		if err != nil {
			t.Fatal(err)
		}
		s.Labels["competing"] = true
		if err := fbSave(e.fbdir, s); err != nil {
			t.Fatal(err)
		}
	}
	blocked := false
	e.update(func(s *fbState) {
		// A separate open descriptor represents the fake-bb writer. Attempt
		// its transaction precisely between the harness load and save.
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if errors.Is(err, syscall.EWOULDBLOCK) {
			blocked = true
		} else if err != nil {
			t.Fatal(err)
		} else {
			write()
			if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
				t.Fatal(err)
			}
		}
		s.Labels["harness"] = true
	})
	if blocked {
		// The protected transaction finished; let the competing writer retry.
		unlock, err := fbLock(e.fbdir)
		if err != nil {
			t.Fatal(err)
		}
		write()
		unlock()
	}
	s := e.st()
	if !s.Labels["harness"] || !s.Labels["competing"] {
		t.Fatalf("lost update: harness=%v competing=%v (competing writer blocked=%v)", s.Labels["harness"], s.Labels["competing"], blocked)
	}
	if !blocked {
		t.Fatal("e.update did not hold the fake-bb lock")
	}
}
