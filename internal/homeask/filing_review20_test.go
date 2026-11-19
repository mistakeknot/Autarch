package homeask

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdmissionUsesClockAfterStateLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("advisory locking unavailable")
	}
	for _, tc := range []struct {
		name, before, after string
		update, held        bool
	}{
		{"Thursday noon", "2026-11-19 11:59:59", "2026-11-19 12:00:00", false, false},
		{"Monday", "2026-11-15 23:59:59", "2026-11-16 00:00:00", false, true},
		{"Monday update", "2026-11-15 23:59:59", "2026-11-16 00:00:00", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t, registryOnCreate).filer()
			dir, err := f.stateDir()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			unlock, err := lockFile(filepath.Join(dir, "filing.lock"))
			if err != nil {
				t.Fatal(err)
			}
			var release sync.Once
			defer release.Do(unlock)
			var now atomic.Int64
			now.Store(pacificTestTime(t, tc.before).UnixNano())
			read := make(chan struct{})
			var signal sync.Once
			f.Now = func() time.Time {
				at := time.Unix(0, now.Load())
				signal.Do(func() { close(read) })
				return at
			}
			r := testReq()
			if tc.update {
				r.Ask["ask_key"] = "new-ask"
			}
			type outcome struct {
				result CardResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				var result CardResult
				var err error
				if tc.update {
					result, err = f.FileOrUpdateCard(context.Background(), r, "")
				} else {
					result, err = f.FileCard(context.Background(), r)
				}
				done <- outcome{result, err}
			}()
			select {
			case <-read:
			case <-time.After(5 * time.Second):
				t.Fatal("admission never read its initial clock")
			}
			after := pacificTestTime(t, tc.after)
			now.Store(after.UnixNano())
			release.Do(unlock)
			select {
			case got := <-done:
				if got.err != nil || got.result.Queued != tc.held {
					t.Fatalf("used pre-lock hold decision: %+v %v", got.result, got.err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("admission remained blocked after unlock")
			}
			state, _, doneState, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			defer doneState()
			e := state[r.Key]
			if e.Week != pacificBudgetWeek(after).key() || e.Uncharged {
				t.Fatalf("used stale reservation week or skipped queue admission: %+v", e)
			}
		})
	}
}

func TestWeekdayHoldStillRefreshesOwnerNotice(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	bb.threads = map[string]string{"thr_old": "archived"}
	f := bb.filer()
	f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
	r := testReq()
	r.Owner = "thr_old"
	if _, err := f.FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	report, err := f.ReleaseDetailed(context.Background())
	if err == nil || report.OwnerAttention[r.Key] == "" || len(report.Groups) != 0 || bb.count("tasks", "create") != 0 {
		t.Fatalf("weekday hold hid the owner notice: %+v %v", report, err)
	}
}
