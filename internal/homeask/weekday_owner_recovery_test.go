package homeask

import (
	"context"
	"testing"
	"time"
)

func TestWeekdayHoldRecoversAttemptedInvalidOwner(t *testing.T) {
	for _, outcome := range []string{"delivered", "creation-key", "absent"} {
		t.Run(outcome, func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			bb.threads = map[string]string{"thr_owner": "live"}
			f := bb.filer()
			now := filingTestNow()
			f.Now = func() time.Time { return now }
			req := testReq()
			req.Key = "held-owner"
			req.Owner = "thr_owner"
			req.NonBlocking = true
			if got, err := f.FileCard(context.Background(), req); err != nil || !got.Queued {
				t.Fatal(got, err)
			}
			state, path, done, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			entry := state[req.Key]
			entry.Attempted = true
			if outcome == "creation-key" {
				entry.CreationKey = "actual-creation"
			}
			state[req.Key] = entry
			if err := saveFilingState(path, state); err != nil {
				t.Fatal(err)
			}
			done()
			if outcome != "absent" {
				created := req
				if outcome == "creation-key" {
					created.Key = entry.CreationKey
				}
				if _, err := f.fileCardReadback(context.Background(), created); err != nil {
					t.Fatal(err)
				}
			}
			now = pacificTestTime(t, "2026-11-16 10:00:00")
			bb.threads[req.Owner] = "archived"
			creates := bb.count("tasks", "create")
			report, releaseErr := f.ReleaseDetailed(context.Background())
			if report.OwnerAttention[req.Key] == "" {
				t.Fatal("missing owner remediation", report, releaseErr)
			}
			if outcome != "absent" {
				if releaseErr != nil || len(report.Groups["other"]) != 1 || report.Groups["other"][0].ID != "T1" {
					t.Fatal(report, releaseErr)
				}
			} else if releaseErr == nil || report.Failures[req.Key] == "" {
				t.Fatal("absent attempted request not retired", report)
			}
			state, _, done, err = f.openState()
			if err != nil {
				t.Fatal(err)
			}
			done()
			if state[req.Key].Held || state[req.Key].Refunded != (outcome == "absent") {
				t.Fatalf("terminal recovery bypassed inside hold: %+v", state[req.Key])
			}
			if bb.count("tasks", "create") != creates {
				t.Fatal("recovery created a card")
			}
		})
	}
}
