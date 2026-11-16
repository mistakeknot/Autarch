package homeask

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReleaseReflagsDismissedOwnerNotice(t *testing.T) {
	for _, ownerState := range []string{"archived", "missing"} {
		for _, fullCap := range []bool{false, true} {
			name := ownerState + "/below-cap"
			if fullCap {
				name = ownerState + "/full-cap"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv(EnvCardDailyCap, "1")
				bb := newFake(t, registryOnCreate)
				bb.threads = map[string]string{"thr_old": ownerState}
				f := bb.filer()
				f.StateDir = t.TempDir()
				if err := f.SetPilot("daycap", true); err != nil {
					t.Fatal(err)
				}
				r := testReq()
				r.Key, r.Owner, r.NonBlocking = "held", "thr_old", true
				if _, err := f.FileCard(context.Background(), r); err != nil {
					t.Fatal(err)
				}
				if report, err := f.ReleaseDetailed(context.Background()); err == nil || report.OwnerAttention[r.Key] == "" {
					t.Fatalf("initial release did not flag owner: %+v %v", report, err)
				}
				state, _, done, err := f.openState()
				if err != nil {
					t.Fatal(err)
				}
				before := state[r.Key]
				done()
				if err := f.DismissReleaseOwnerAttention(r.Key); err != nil {
					t.Fatal(err)
				}
				var footer bytes.Buffer
				if err := f.PrintReleaseOwnerAttention(&footer); err != nil || footer.Len() != 0 {
					t.Fatalf("dismissal did not clear footer: %q %v", footer.String(), err)
				}
				if fullCap {
					fill := testReq()
					fill.Key = "delivered"
					if result, err := f.FileCard(context.Background(), fill); err != nil || result.Queued {
						t.Fatalf("could not fill daily cap: %+v %v", result, err)
					}
				}
				report, err := f.ReleaseDetailed(context.Background())
				var entries *ReleaseEntryErrors
				if !errors.As(err, &entries) || !entries.DataOnly() || report.OwnerAttention[r.Key] == "" {
					t.Fatalf("release did not restore dismissed owner notice: %+v %v", report, err)
				}
				if err := f.PrintReleaseOwnerAttention(&footer); err != nil || !strings.Contains(footer.String(), r.Key) {
					t.Fatalf("restored notice missing from footer: %q %v", footer.String(), err)
				}
				state, _, done, err = f.openState()
				if err != nil {
					t.Fatal(err)
				}
				defer done()
				if !reflect.DeepEqual(state[r.Key], before) {
					t.Fatalf("reflag changed request, delivery or budget state: got %+v want %+v", state[r.Key], before)
				}
			})
		}
	}
}

func TestFullCapResolvesAttemptedHeldWithInvalidOwner(t *testing.T) {
	for _, ownerState := range []string{"archived", "missing"} {
		for _, outcome := range []string{"recover", "retire"} {
			t.Run(ownerState+"/"+outcome, func(t *testing.T) {
				t.Setenv(EnvCardDailyCap, "1")
				bb := newFake(t, registryOnCreate)
				bb.threads = map[string]string{"thr_old": "live"}
				f := bb.filer()
				f.StateDir = t.TempDir()
				now := filingTestNow() // all recovery cases start on a fixed delivery Friday
				f.Now = func() time.Time { return now }
				if err := f.SetPilot("daycap", true); err != nil {
					t.Fatal(err)
				}
				r := testReq()
				r.Key, r.Owner, r.NonBlocking = "attempted", "thr_old", true
				if result, err := f.FileCard(context.Background(), r); err != nil || !result.Queued {
					t.Fatalf("could not queue request: %+v %v", result, err)
				}
				state, _, done, err := f.openState()
				if err != nil {
					t.Fatal(err)
				}
				before := state[r.Key]
				before.Attempted = true
				done()
				var canonical CardResult
				if outcome == "recover" {
					// Simulate a prior attempt that wrote the card but lost its state acknowledgement.
					var err error
					canonical, err = f.fileCardReadback(context.Background(), r)
					if err != nil || canonical.ID == "" {
						t.Fatalf("could not seed uncertain delivery: %+v %v", canonical, err)
					}
				}
				state, path, done, err := f.openState()
				if err != nil {
					t.Fatal(err)
				}
				// Restore the last persisted attempt, before any delivery acknowledgement.
				state[r.Key] = before
				err = saveFilingState(path, state)
				done()
				if err != nil {
					t.Fatal(err)
				}
				fill := testReq()
				fill.Key = "delivered"
				if result, err := f.FileCard(context.Background(), fill); err != nil || result.Queued {
					t.Fatalf("could not fill cap: %+v %v", result, err)
				}
				state, _, done, err = f.openState()
				if err != nil {
					t.Fatal(err)
				}
				full, err := f.dailyFull(state, r)
				fillBefore := state[fill.Key]
				done()
				if err != nil || !full || fillBefore.DeliveredAt == "" {
					t.Fatalf("fixture must have a full delivered cap: full=%v err=%v", full, err)
				}
				budgetBefore, err := f.BudgetStatus("autarch")
				if err != nil {
					t.Fatal(err)
				}
				bb.threads[r.Owner] = ownerState
				callsBefore := len(bb.calls)
				report, releaseErr := f.ReleaseDetailed(context.Background())
				if report.OwnerAttention[r.Key] == "" {
					t.Fatalf("invalid owner was not reported: %+v %v", report, releaseErr)
				}
				if outcome == "recover" {
					group := report.Groups["other"]
					if releaseErr != nil || len(report.Failures) != 0 || len(group) != 1 || group[0].ID != canonical.ID || !group[0].Replay {
						t.Fatalf("canonical delivery not recovered: %+v %v", report, releaseErr)
					}
				} else {
					var entries *ReleaseEntryErrors
					if !errors.As(releaseErr, &entries) || !entries.DataOnly() || report.Failures[r.Key] == "" || len(report.Groups) != 0 {
						t.Fatalf("absent request not retired as data fault: %+v %v", report, releaseErr)
					}
				}
				for _, call := range bb.calls[callsBefore:] {
					if len(call) >= 2 && ((call[0] == "tasks" && (call[1] == "create" || call[1] == "update" || call[1] == "comment")) || (call[0] == "thread" && call[1] == "tell")) {
						t.Fatalf("terminal recovery must not deliver, edit or wake: %v", call)
					}
				}
				state, _, done, err = f.openState()
				if err != nil {
					t.Fatal(err)
				}
				after := state[r.Key]
				fillAfter := state[fill.Key]
				done()
				want := before
				want.Held = false
				want.OwnerAttention = report.OwnerAttention[r.Key]
				if outcome == "recover" {
					want.DeliveredCard = canonical.ID
				} else {
					if !strings.Contains(after.TerminalReason, r.Owner) {
						t.Fatalf("retirement lost owner failure: %+v", after)
					}
					want.Refunded = true
					want.TerminalReason = after.TerminalReason
					budgetBefore.Ordinary--
				}
				if !reflect.DeepEqual(after, want) || !reflect.DeepEqual(fillAfter, fillBefore) {
					t.Fatalf("unexpected state changes: request=%+v want=%+v; delivered entry changed=%v", after, want, !reflect.DeepEqual(fillAfter, fillBefore))
				}
				budgetAfter, err := f.BudgetStatus("autarch")
				if err != nil || budgetAfter != budgetBefore {
					t.Fatalf("terminal budget mismatch: got %+v want %+v err=%v", budgetAfter, budgetBefore, err)
				}
			})
		}
	}
}
