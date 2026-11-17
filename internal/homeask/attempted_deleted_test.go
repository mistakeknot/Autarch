package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAttemptedDeletedCardUrgentRetryIsTerminal(t *testing.T) {
	for _, kind := range []string{"request-key", "creation-key", "recorded"} {
		t.Run(kind, func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
			r := testReq()
			r.Key = "attempted"
			if got, err := f.FileCard(context.Background(), r); err != nil || !got.Queued {
				t.Fatal(got, err)
			}
			state, path, done, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			entry := state[r.Key]
			entry.Attempted = true
			creation := r
			if kind == "creation-key" {
				creation.Key = "actual-creation"
				entry.CreationKey = creation.Key
			}
			done()
			card, err := f.fileCardReadback(context.Background(), creation)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "recorded" {
				entry.DeliveredCard = card.ID
				entry.DeliveredKey = card.Key
				entry.DeliveredProject = card.Project
			}
			state, path, done, err = f.openState()
			if err != nil {
				t.Fatal(err)
			}
			state[r.Key] = entry
			if err := saveFilingState(path, state); err != nil {
				t.Fatal(err)
			}
			done()
			bb.tasks[0].Deleted = true
			r.Urgent = true
			for range 2 {
				result, err := f.FileCard(context.Background(), r)
				if !errors.Is(err, ErrInvalid) || errors.Is(err, ErrOutcomeUnknown) || result.Queued || !strings.Contains(strings.ToLower(err.Error()), "new request") || strings.Contains(err.Error(), "re-run the same command") {
					t.Fatalf("deleted delivery not terminal: %+v %v", result, err)
				}
			}
			if bb.count("tasks", "create") != 1 {
				t.Fatal("created a duplicate for a deleted card")
			}
			status, err := f.BudgetStatus("autarch")
			if err != nil || status.Ordinary != 1 {
				t.Fatal("refunded an already-created card", status, err)
			}
		})
	}
}
