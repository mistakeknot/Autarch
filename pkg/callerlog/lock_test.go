package callerlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestAuditLockRetry(t *testing.T) {
	for _, release := range []bool{true, false} {
		name := "held-past-window"
		if release {
			name = "released-within-window"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			f, err := os.OpenFile(filepath.Join(dir, "caller-log.jsonl"), os.O_CREATE|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
				t.Fatal(err)
			}
			defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
			now := time.Unix(0, 0)
			waits := 0
			l := New(dir, func(l *Log) {
				l.lockNow = func() time.Time { return now }
				l.lockWait = func(d time.Duration) {
					waits++
					now = now.Add(d)
					if release {
						if err := unix.Flock(int(f.Fd()), unix.LOCK_UN); err != nil {
							panic(err)
						}
					}
				}
			})
			l.Record(map[string]any{"probe": "lock-retry"}, "")
			done := make(chan struct{})
			go func() { l.Close(); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("audit worker hung on held lock")
			}
			data, err := os.ReadFile(f.Name())
			if err != nil {
				t.Fatal(err)
			}
			if waits == 0 || waits > 3 || now.Sub(time.Unix(0, 0)) > 2*time.Millisecond {
				t.Fatalf("waits=%d elapsed=%s", waits, now.Sub(time.Unix(0, 0)))
			}
			if release {
				if l.Dropped() != 0 || len(data) == 0 || l.warned {
					t.Fatalf("drops=%d bytes=%d warned=%v", l.Dropped(), len(data), l.warned)
				}
			} else if l.Dropped() != 1 || len(data) != 0 || !l.warned {
				t.Fatalf("drops=%d bytes=%d warned=%v", l.Dropped(), len(data), l.warned)
			}
		})
	}
}
