package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDeletedHeldReplacementRetiresWithoutRefund(t *testing.T) {
	bb, f, _ := updateRig(t)
	ctx := context.Background()
	r := testReq()
	r.Ask["ask_key"] = "replacement-one"
	if _, err := f.FileOrUpdateCard(ctx, r, ""); err != nil {
		t.Fatal(err)
	}
	r.Key = "held-removed"
	r.Ask["question"] = "Updated question?"
	r.NonBlocking = true
	if _, err := f.FileOrUpdateCard(ctx, r, ""); err != nil {
		t.Fatal(err)
	}
	bb.tasks[0].Status = "done"
	originalRead := bb.fail
	bb.fail = func(args []string) *BBResult {
		if strings.Join(args[:min(3, len(args))], " ") == "home get --card" && args[3] == "T2" {
			return badRes(1, "post-create index unavailable")
		}
		return originalRead(args)
	}
	f.ReadbackTimeout = 10 * time.Millisecond
	f.ReadbackInterval = time.Millisecond
	if _, err := f.ReleaseDetailed(ctx); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("missing post-create uncertainty: %v", err)
	}
	if len(bb.tasks) != 2 {
		t.Fatal("replacement not created")
	}
	state, _, unlock, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state[r.Key]
	unlock()
	if entry.CreationKey == "" || entry.CreationKey == r.Key || entry.Uncharged {
		t.Fatal("replacement was not charged under refile key", entry)
	}
	removedID := bb.tasks[1].ID
	bb.tasks[1].Deleted = true
	bb.fail = originalRead
	for n := 0; n < 3; n++ {
		report, err := f.ReleaseDetailed(ctx)
		if err != nil || len(report.Failures) != 0 {
			t.Errorf("removed delivery stayed stuck: %+v %v", report, err)
		}
		if n > 0 && len(report.Groups) != 0 {
			t.Errorf("removed delivery reported again: %+v", report)
		}
	}
	state, _, unlock, err = f.openState()
	if err != nil {
		t.Fatal(err)
	}
	entry = state[r.Key]
	unlock()
	if entry.Held || entry.Refunded || entry.Uncharged || entry.DeliveredCard != removedID || !strings.Contains(entry.TerminalReason, "delivered-then-removed") {
		t.Errorf("not retired as charged removed delivery: %+v", entry)
	}
	budget, err := f.BudgetStatus("autarch")
	if err != nil || budget.Ordinary != 2 {
		t.Fatal("removed delivery was refunded or double charged", budget, err)
	}
	if bb.count("tasks", "create") != 2 {
		t.Fatal("recovery created a duplicate")
	}
}

func TestRemovedRecoveryRejectsChangedOrReusedCard(t *testing.T) {
	for _, kind := range []string{"changed-card", "foreign-deleted-binding"} {
		t.Run(kind, func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			ctx := context.Background()
			r := testReq()
			r.NonBlocking = true
			if _, err := f.FileCard(ctx, r); err != nil {
				t.Fatal(err)
			}
			if _, err := f.Release(ctx); err != nil {
				t.Fatal(err)
			}
			state, path, unlock, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			entry := state[r.Key]
			entry.Held = true
			entry.Attempted = true
			entry.DeliveredCard = ""
			state[r.Key] = entry
			err = saveFilingState(path, state)
			unlock()
			if err != nil {
				t.Fatal(err)
			}
			if kind == "changed-card" {
				identity, _ := RequestIdentity(r)
				bb.tasks[0].Desc = strings.Replace(bb.tasks[0].Desc, identity, strings.Repeat("f", 16), 1)
			} else {
				bb.tasks[0].Deleted = true
				binding := bb.registry[r.Key]
				binding.Identity = strings.Repeat("f", 16)
				bb.registry[r.Key] = binding
			}
			if _, err := f.ReleaseDetailed(ctx); err == nil {
				t.Fatal("untrusted binding finalized as delivery")
			}
			state, _, unlock, err = f.openState()
			if err != nil {
				t.Fatal(err)
			}
			entry = state[r.Key]
			unlock()
			if !entry.Held || entry.Refunded || entry.DeliveredCard != "" {
				t.Fatal("mismatch changed delivery state", entry)
			}
			count, err := f.BudgetStatus("autarch")
			if err != nil || count.Ordinary != 1 {
				t.Fatal("mismatch changed charge", count, err)
			}
		})
	}
}
