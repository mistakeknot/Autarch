package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUrgentAttemptedHeldReconcilesBeforeEscalating(t *testing.T) {
	t.Setenv(EnvCardDailyCap, "1")
	for _, outcome := range []string{"absent", "delivered", "created-key", "unavailable", "delivered-root-envelope"} {
		t.Run(outcome, func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
			enablePilot(t, f, "daycap")
			r := testReq()
			r.Key = "uncertain"
			if result, err := f.FileCard(context.Background(), r); err != nil || !result.Queued {
				t.Fatal(result, err)
			}
			state, path, done, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			entry := state[r.Key]
			entry.Attempted = true
			if outcome == "delivered" || outcome == "created-key" || outcome == "delivered-root-envelope" {
				entry.DeliveryPendingAt = f.clock().Format(time.RFC3339Nano)
			}
			if outcome == "created-key" {
				entry.CreationKey = "creation"
			}
			state[r.Key] = entry
			if err := saveFilingState(path, state); err != nil {
				t.Fatal(err)
			}
			done()
			if outcome == "delivered" || outcome == "created-key" || outcome == "delivered-root-envelope" {
				creation := r
				if outcome == "created-key" {
					creation.Key = "creation"
				}
				if _, err := f.fileCardReadback(context.Background(), creation); err != nil {
					t.Fatal(err)
				}
			}
			if outcome == "unavailable" {
				bb.fail = func(args []string) *BBResult {
					if len(args) > 2 && strings.Join(args[:3], " ") == "home get --request" {
						return badRes(1, "registry unavailable")
					}
					return nil
				}
			}
			urgent := r
			urgent.Urgent = true
			if outcome == "delivered-root-envelope" {
				urgent.RootRun = &RootRun{Script: "/sample/check.sh", SHA256: strings.Repeat("a", 64), Timeout: 30, Set: "sample"}
			}
			result, err := f.FileCard(context.Background(), urgent)
			if outcome == "delivered-root-envelope" {
				if !errors.Is(err, ErrInvalid) || result.ID != "T1" || result.Queued || bb.count("tasks", "create") != 1 {
					t.Fatalf("missing root envelope reported as delivered: %+v %v", result, err)
				}
				return
			}
			if outcome == "unavailable" {
				if !errors.Is(err, ErrOutcomeUnknown) || result.Queued || bb.count("tasks", "create") != 0 {
					t.Fatalf("uncertain proof falsely delivered/held: %+v %v", result, err)
				}

				state, _, done, stateErr := f.openState()
				if stateErr != nil {
					t.Fatal(stateErr)
				}
				done()
				if !state[r.Key].Held || !state[r.Key].Attempted || state[r.Key].Refunded {
					t.Fatal("uncertain proof changed reservation", state[r.Key])
				}
				return
			}
			if err != nil || result.Queued || result.ID != "T1" || bb.count("tasks", "create") != 1 {
				t.Fatalf("urgent attempted row stayed held or duplicated: %+v %v", result, err)
			}
			status, err := f.BudgetStatus("autarch")
			if err != nil {
				t.Fatal(err)
			}
			wantOrdinary := 1
			if outcome == "absent" {
				wantOrdinary = 0
			}
			if status.Ordinary != wantOrdinary {
				t.Fatal(status)
			}
			state, _, done, err = f.openState()
			if err != nil {
				t.Fatal(err)
			}
			done()
			if outcome != "absent" && state[r.Key].DeliveredAt == "" {
				t.Fatal("reconciliation lost the recorded delivery time")
			}
			full, capErr := f.dailyFull(state, r)
			if capErr != nil || full != (outcome != "absent") {
				t.Fatalf("wrong recovered delivery cap: %v %v", full, capErr)
			}

		})
	}
}
