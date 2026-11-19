package homeask

import (
	"context"
	"fmt"
	"testing"
)

// mk-q67fj: count settings must never prevent filing.
func TestNoCardCountLimits(t *testing.T) {
	for _, setting := range []string{"1", "100000", "invalid"} {
		t.Run(setting, func(t *testing.T) {
			t.Setenv(EnvCardWeeklyBudget, setting)
			t.Setenv(EnvCardDailyCap, setting)
			f := newFake(t, registryOnCreate).filer()
			enablePilot(t, f, "daycap")
			f.Warn = func(s string) { t.Errorf("unexpected warning: %s", s) }
			for i := 0; i < 60; i++ {
				r := testReq()
				r.Key = fmt.Sprintf("ordinary-%d", i)
				result, err := f.FileCard(context.Background(), r)
				if err != nil || result.Queued || result.ID == "" {
					t.Fatalf("card %d: %+v %v", i, result, err)
				}
			}
		})
	}
}

// Legacy reservations and count-only holds remain readable and releasable.
func TestLegacyCountHeldStateReleases(t *testing.T) {
	for _, reason := range []string{"daily-cap", "weekly-budget", "budget"} {
		t.Run(reason, func(t *testing.T) {
			f := newFake(t, registryOnCreate).filer()
			enablePilot(t, f, "daycap")
			r := testReq()
			r.NonBlocking = true
			if _, err := f.FileCard(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			state, path, done, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			entry := state[r.Key]
			entry.HeldReason = reason
			entry.Request.NonBlocking = false
			entry.Week = "2026-W47"
			state[r.Key] = entry
			err = saveFilingState(path, state)
			done()
			if err != nil {
				t.Fatal(err)
			}
			report, err := f.ReleaseDetailed(context.Background())
			if err != nil || len(report.Groups["other"]) != 1 {
				t.Fatalf("%+v %v", report, err)
			}
			state, _, done, err = f.openState()
			if err != nil {
				t.Fatal(err)
			}
			defer done()
			if state[r.Key].Held {
				t.Fatal("legacy count hold stranded")
			}
		})
	}
}
