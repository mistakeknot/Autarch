package homeask

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func pacificTestTime(t *testing.T, value string) time.Time {
	t.Helper()
	at, err := time.ParseInLocation("2006-01-02 15:04:05", value, pacific)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestBudgetPacificWeekAndDST(t *testing.T) {
	for _, tc := range []struct{ now, start, end string }{
		{"2026-11-19 11:59:59", "2026-11-12T12:00:00-08:00", "2026-11-19T12:00:00-08:00"},
		{"2026-11-19 12:00:00", "2026-11-19T12:00:00-08:00", "2026-11-26T12:00:00-08:00"},
		{"2026-11-01 01:30:00", "2026-10-29T12:00:00-07:00", "2026-11-05T12:00:00-08:00"},
		{"2026-11-01 03:00:00", "2026-10-29T12:00:00-07:00", "2026-11-05T12:00:00-08:00"},
		{"2026-03-08 01:59:59", "2026-03-05T12:00:00-08:00", "2026-03-12T12:00:00-07:00"},
		{"2026-03-08 03:00:00", "2026-03-05T12:00:00-08:00", "2026-03-12T12:00:00-07:00"},
	} {
		t.Run(tc.now, func(t *testing.T) {
			f := newFake(t, registryOnCreate).filer()
			f.Now = func() time.Time { return pacificTestTime(t, tc.now).UTC() }
			r := testReq()
			r.NonBlocking = true
			if _, err := f.FileCard(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			status, err := f.BudgetStatus("AUTARCH")
			if err != nil || status.Ordinary != 1 {
				t.Fatal(status, err)
			}
			raw, err := json.Marshal(status)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if fields["week_start"] != tc.start || fields["week_end"] != tc.end || fields["time_zone"] != "America/Los_Angeles" {
				t.Fatalf("wrong Pacific budget period: %s", raw)
			}
			state, _, done, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			defer done()
			if state[r.Key].Week != tc.start[:10] {
				t.Fatalf("reservation week = %q", state[r.Key].Week)
			}
		})
	}
}

func TestPacificHoldBoundariesAndExemptions(t *testing.T) {
	for _, tc := range []struct {
		name, now, exemption string
		held                 bool
	}{
		{"Sunday", "2026-11-15 23:59:59", "", false},
		{"Monday boundary", "2026-11-16 00:00:00", "", true},
		{"Thursday before noon", "2026-11-19 11:59:59", "", true},
		{"Thursday noon", "2026-11-19 12:00:00", "", false},
		{"Thursday afternoon", "2026-11-19 14:00:00", "", false},
		{"Friday", "2026-11-20 10:00:00", "", false},
		{"urgent", "2026-11-16 00:00:00", "urgent", false},
		{"keep-list", "2026-11-16 00:00:00", "spend", false},
		{"merge", "2026-11-16 00:00:00", "merge", false},
		{"root-run", "2026-11-16 00:00:00", "root-run", false},
		{"March Monday", "2026-03-09 00:00:00", "", true},
		{"March noon", "2026-03-12 12:00:00", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			f.Now = func() time.Time { return pacificTestTime(t, tc.now).UTC() }
			r := testReq()
			switch tc.exemption {
			case "urgent":
				r.Urgent = true
			case "spend":
				r.KeepList = "spend"
			case "merge":
				r.Type = "merge"
			case "root-run":
				r.RootRun = &RootRun{Script: "/sample.sh", SHA256: strings.Repeat("a", 64), Timeout: 30, Set: "sample"}
			}
			result, err := f.FileCard(context.Background(), r)
			if err != nil || result.Queued != tc.held || bb.count("tasks", "create") != map[bool]int{true: 0, false: 1}[tc.held] {
				t.Fatalf("result=%+v err=%v creates=%d", result, err, bb.count("tasks", "create"))
			}
			status, err := f.BudgetStatus("autarch")
			if err != nil {
				t.Fatal(err)
			}
			if tc.exemption == "urgent" || tc.exemption == "spend" {
				if status.Ordinary != 0 || status.Exempt != 1 {
					t.Fatal(status)
				}
			} else if status.Ordinary != 1 {
				t.Fatal(status)
			}
		})
	}
}

func TestPacificReleaseWaitsForNoonAndDoesNotRecharge(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	now := pacificTestTime(t, "2026-11-15 23:59:00")
	f.Now = func() time.Time { return now }
	ctx := context.Background()
	for _, typ := range []string{"play", "design"} {
		r := testReq()
		r.Key, r.Type, r.NonBlocking = typ, typ, true
		if _, err := f.FileCard(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	now = pacificTestTime(t, "2026-11-19 11:59:59")
	before := len(bb.calls)
	report, err := f.ReleaseDetailed(ctx)
	if err != nil || len(report.Groups) != 0 || len(bb.calls) != before {
		t.Fatalf("early release performed external work: %+v %v", report, err)
	}
	now = now.Add(time.Second)
	// A new reservation fills the new week. Old queued cards still release.
	f.WeeklyBudget = 1
	r := testReq()
	r.Key, r.NonBlocking = "current", true
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	report, err = f.ReleaseDetailed(ctx)
	if err != nil || len(report.Groups["play"]) != 1 || len(report.Groups["design"]) != 1 || len(report.Groups["other"]) != 1 {
		t.Fatalf("noon release: %+v %v", report, err)
	}
	status, err := f.BudgetStatus("autarch")
	if err != nil || status.Ordinary != 1 {
		t.Fatal("release charged old reservations again", status, err)
	}
	report, err = f.ReleaseDetailed(ctx)
	if err != nil || len(report.Groups) != 0 || bb.count("tasks", "create") != 3 {
		t.Fatal("duplicate release", report, err)
	}
}

func TestPacificLegacyISOReservations(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	state, path, done, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	// The original file has both timestamped and untimestamped ISO reservations.
	state["unknown"] = filingEntry{Project: "autarch", Week: "2026-W47"}
	state["early"] = filingEntry{Project: "autarch", Week: "2026-W47", FiledAt: "2026-11-16T10:00:00-08:00"}
	state["late"] = filingEntry{Project: "autarch", Week: "2026-W47", FiledAt: "2026-11-20T10:00:00-08:00"}
	if err := saveFilingState(path, state); err != nil {
		done()
		t.Fatal(err)
	}
	done()
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		now  string
		used int
	}{
		{"2026-11-18 12:00:00", 2},
		{"2026-11-19 12:00:00", 2},
		{"2026-11-23 00:00:00", 2},
		{"2026-11-26 12:00:00", 0},
	} {
		f.Now = func() time.Time { return pacificTestTime(t, tc.now) }
		status, err := f.BudgetStatus("autarch")
		if err != nil || status.Ordinary != tc.used {
			t.Fatalf("%s: %+v %v", tc.now, status, err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != string(original) {
		t.Fatal("status rewrote legacy reservations", err)
	}
	f.Now = func() time.Time { return pacificTestTime(t, "2026-11-19 12:00:00") }
	f.WeeklyBudget = 2
	r := testReq()
	r.NonBlocking = true
	if _, err := f.FileCard(context.Background(), r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("legacy reservations did not enforce the budget: %v", err)
	}
}

func TestPacificLegacyHeldReleaseKeepsReservation(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	now := pacificTestTime(t, "2026-11-16 10:00:00")
	f.Now = func() time.Time { return now }
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
	entry.Week, entry.FiledAt = "2026-W47", ""
	state[r.Key] = entry
	if err := saveFilingState(path, state); err != nil {
		done()
		t.Fatal(err)
	}
	done()
	now = pacificTestTime(t, "2026-11-19 12:00:00")
	f.WeeklyBudget = 1
	report, err := f.ReleaseDetailed(context.Background())
	if err != nil || len(report.Groups["other"]) != 1 {
		t.Fatal(report, err)
	}
	status, err := f.BudgetStatus("autarch")
	if err != nil || status.Ordinary != 1 {
		t.Fatal("legacy reservation charged twice", status, err)
	}
	state, _, done, err = f.openState()
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if e := state[r.Key]; e.Week != "2026-W47" || e.Held || e.Refunded || e.Uncharged || bb.count("tasks", "create") != 1 {
		t.Fatalf("legacy delivery changed its reservation: %+v", e)
	}
}

func TestPacificLegacyReplacementChecksMappedBudget(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	f.WeeklyBudget = 1
	state, path, done, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	state["reserved"] = filingEntry{Project: "autarch", Week: "2026-11-12"}
	state["edit"] = filingEntry{Project: "autarch", Week: "2026-W47", Uncharged: true, FiledAt: "2026-11-16T10:00:00-08:00"}
	if err := saveFilingState(path, state); err != nil {
		done()
		t.Fatal(err)
	}
	done()
	r := testReq()
	r.Key = "edit"
	if err := f.chargeCreation(r, "replacement"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("replacement ignored mapped reservation: %v", err)
	}
}

func TestPacificReleaseExemptionsInsideWindow(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
	for _, kind := range []string{"urgent", "keep-list", "merge", "root-run"} {
		r := testReq()
		r.Key, r.NonBlocking = kind, true
		switch kind {
		case "urgent":
			r.Urgent = true
		case "keep-list":
			r.KeepList = "spend"
		case "merge":
			r.Type = "merge"
		case "root-run":
			r.RootRun = &RootRun{Script: "/sample.sh", SHA256: strings.Repeat("a", 64), Timeout: 30, Set: "sample"}
		}
		if _, err := f.FileCard(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	report, err := f.ReleaseDetailed(context.Background())
	if err != nil || len(report.Groups["other"]) != 3 || len(report.Groups["merge"]) != 1 {
		t.Fatalf("hold incorrectly delayed operational cards: %+v %v", report, err)
	}
}

func TestPacificReleaseRechecksAfterMondayBoundary(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	now := pacificTestTime(t, "2026-11-15 23:59:59")
	f.Now = func() time.Time { return now }
	r := testReq()
	r.NonBlocking = true
	if _, err := f.FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	f.Check = func(string, string) error {
		now = now.Add(time.Second)
		return nil
	}
	report, err := f.ReleaseDetailed(context.Background())
	if err != nil || len(report.Groups) != 0 || bb.count("tasks", "create") != 0 {
		t.Fatalf("crossed Monday boundary during preflight: %+v %v", report, err)
	}
}

func TestDailyCapFillingUsesDeliveryDayClock(t *testing.T) {
	t.Setenv(EnvCardDailyCap, "1")
	for _, tc := range []struct {
		name, at string
		held     bool
	}{
		{"Friday", "2026-11-13 12:00:00", false},
		{"Sunday", "2026-11-15 23:59:00", false},
		{"Thursday before noon", "2026-11-19 11:59:00", true},
		{"Thursday noon", "2026-11-19 12:00:00", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t, registryOnCreate).filer()
			now := pacificTestTime(t, tc.at)
			f.Now = func() time.Time { return now }
			enablePilot(t, f, "daycap")
			r := testReq() // ordinary throughout; urgency must not bypass cap checks
			first, err := f.FileCard(context.Background(), r)
			if err != nil || first.Queued != tc.held {
				t.Fatalf("first ordinary card: %+v %v", first, err)
			}
			state, _, done, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			full, err := f.dailyFull(state, r)
			done()
			if err != nil || full == tc.held {
				t.Fatalf("only delivery fills the daily cap: full=%v held=%v err=%v", full, tc.held, err)
			}
			r.Key = "second"
			second, err := f.FileCard(context.Background(), r)
			reason := "daily-cap"
			if tc.held {
				reason = "weekday-window"
			}
			if err != nil || !second.Queued || second.HoldReason != reason {
				t.Fatalf("wrong ordinary hold: %+v %v; want %s", second, err, reason)
			}
		})
	}
}
