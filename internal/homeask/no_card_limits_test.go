package homeask

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// This hand-written pre-removal state includes a count hold, an in-flight
// uncharged reservation, and a timestamp-free ISO-week reporting record.
const legacyCountState = `{
 "legacy-held": {
  "request": {
   "Project":"P1","Title":"Ship the thing?","Key":"legacy-held","Thread":"thr_a",
   "Blocks":["thread:thr_a","bead:Autarch-1"],
   "Ask":{"project":"autarch","project_root":"/srv/autarch","question":"Ship it?",
    "options":[{"id":"yes","label":"Yes","kind":"ruling-only"},{"id":"no","label":"No","kind":"ruling-only"}]}
  },
  "identity":"d978b44d835e92ce","project":"autarch","week":"2026-11-12",
  "held":true,"held_reason":"daily-cap"
 },
 "pending":{"request":{},"project":"autarch","week":"2026-11-12",
  "held":false,"delivery_pending_at":"2026-11-13T12:00:00Z","uncharged":true},
 "iso":{"request":{},"project":"autarch","week":"2026-W47","held":false}
}`

func writeLegacyCountState(t *testing.T, f *CardFiler) {
	t.Helper()
	dir, err := f.stateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "filings.json"), []byte(legacyCountState), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyCountStateJSONReleases(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	writeLegacyCountState(t, f)
	report, err := f.ReleaseDetailed(context.Background())
	if err != nil || len(report.Groups[""])+len(report.Groups["other"]) != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	state, _, done, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if state["legacy-held"].Held {
		t.Fatal("count hold stranded")
	}
	if !state["pending"].Uncharged || state["pending"].DeliveryPendingAt == "" || state["iso"].Week != "2026-W47" {
		t.Fatal("legacy reporting records lost")
	}
}

func TestLegacyCountHoldRefiles(t *testing.T) {
	for _, reason := range []string{"daily-cap", "weekly-budget", "budget"} {
		t.Run(reason, func(t *testing.T) {
			f := newFake(t, registryOnCreate).filer()
			writeLegacyCountState(t, f)
			// Exercise the same on-disk request with each legacy count reason.
			raw := strings.ReplaceAll(legacyCountState, "daily-cap", reason)
			dir, _ := f.stateDir()
			if err := os.WriteFile(filepath.Join(dir, "filings.json"), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			r := testReq()
			r.Key = "legacy-held"
			result, err := f.FileCard(context.Background(), r)
			if err != nil || result.Queued || result.ID == "" {
				t.Fatalf("%+v %v", result, err)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "queued") || strings.Contains(string(encoded), "daily-cap") {
				t.Fatal(string(encoded))
			}
		})
	}
}
