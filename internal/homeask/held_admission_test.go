package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestHeldAskEscalationsDeliverAndRetireReservation(t *testing.T) {
	for _, kind := range []string{"urgent", "merge", "root-run", "keep-list", "Thursday noon"} {
		t.Run(kind, func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			f.WeeklyBudget = 1
			now := pacificTestTime(t, "2026-11-16 10:00:00")
			f.Now = func() time.Time { return now }
			first := testReq()
			first.Key = "held"
			first.Ask["ask_key"] = "stable"
			if got, err := f.FileOrUpdateCard(context.Background(), first, ""); err != nil || !got.Queued {
				t.Fatal(got, err)
			}
			next := testReq()
			next.Key = "escalated"
			next.Ask["ask_key"] = "stable"
			next.Ask["question"] = "New facts: act now?"
			switch kind {
			case "urgent":
				next.Urgent = true
			case "merge":
				next.Type = "merge"
			case "root-run":
				next.RootRun = &RootRun{Script: "/sample/check.sh", SHA256: strings.Repeat("a", 64), Timeout: 30, Set: "sample"}
			case "keep-list":
				next.KeepList = "spend"
			case "Thursday noon":
				now = pacificTestTime(t, "2026-11-19 12:00:00")
			}
			got, err := f.FileOrUpdateCard(context.Background(), next, "")
			if err != nil || got.Queued || got.ID == "" || bb.count("tasks", "create") != 1 {
				t.Fatalf("revision held: %+v %v", got, err)
			}
			state, _, done, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			done()
			old := state[first.Key]
			if old.Held || !old.Refunded || old.Request.Urgent || old.Request.KeepList != "" {
				t.Fatalf("reservation not explicitly retired: %+v", old)
			}
			wantOrdinary := 1
			wantExempt := 0
			if kind == "urgent" || kind == "keep-list" {
				wantOrdinary = 0
				wantExempt = 1
			}
			status, err := f.BudgetStatus("autarch")
			if err != nil || status.Ordinary != wantOrdinary || status.Exempt != wantExempt {
				t.Fatalf("bad counts: %+v %v", status, err)
			}
			now = pacificTestTime(t, "2026-11-19 12:00:00")
			if _, err := f.ReleaseDetailed(context.Background()); err != nil {
				t.Fatal(err)
			}
			if bb.count("tasks", "create") != 1 {
				t.Fatal("retired ask released again")
			}
		})
	}
}

func TestHeldAskSameKeyDifferentAskerKeepsOwnReservation(t *testing.T) {
	for _, owner := range []bool{false, true} {
		t.Run(map[bool]string{false: "thread", true: "owner"}[owner], func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
			first := testReq()
			first.Key = "first"
			first.Ask["ask_key"] = "stable"
			if _, err := f.FileOrUpdateCard(context.Background(), first, ""); err != nil {
				t.Fatal(err)
			}
			other := testReq()
			other.Key = "other"
			other.Ask["ask_key"] = "stable"
			if owner {
				other.Owner = "thr_other"
			} else {
				other.Thread = "thr_other"
			}
			result, err := f.FileOrUpdateCard(context.Background(), other, "")
			if err != nil || !result.Queued || result.Key != other.Key {
				t.Fatalf("separate asker refused: %+v %v", result, err)
			}
			status, err := f.BudgetStatus("autarch")
			if err != nil || status.Ordinary != 2 {
				t.Fatal(status, err)
			}
		})
	}
}

func TestHeldRetirementRollsBackOnDefiniteRefusal(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
	first := testReq()
	first.Key = "held"
	first.Ask["ask_key"] = "stable"
	if _, err := f.FileOrUpdateCard(context.Background(), first, ""); err != nil {
		t.Fatal(err)
	}
	next := testReq()
	next.Key = "bad-owner"
	next.Ask["ask_key"] = "stable"
	next.Urgent = true
	bb.fail = func(args []string) *BBResult {
		if len(args) > 2 && strings.Join(args[:3], " ") == "tasks project list" {
			result := okRes(map[string]any{"projects": []any{}})
			return &result
		}
		return nil
	}
	_, err := f.FileOrUpdateCard(context.Background(), next, "")
	if !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	state, _, done, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	done()
	if len(state) != 1 || !state[first.Key].Held || state[first.Key].Refunded {
		t.Fatalf("definite refusal did not restore the held ask: %+v", state)
	}
}

func TestHeldAdmissionRetryIsBounded(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
	first := testReq()
	first.Key = "first"
	first.Ask["ask_key"] = "stable"
	if _, err := f.FileOrUpdateCard(context.Background(), first, ""); err != nil {
		t.Fatal(err)
	}
	reads := 0
	f.Now = func() time.Time {
		reads++
		if reads%2 == 1 {
			return pacificTestTime(t, "2026-11-19 11:59:59")
		}
		return pacificTestTime(t, "2026-11-19 12:00:00")
	}
	next := testReq()
	next.Key = "new"
	next.Ask["ask_key"] = "stable"
	_, err := f.FileOrUpdateCard(context.Background(), next, "")
	if !errors.Is(err, ErrOutcomeUnknown) || reads != 16 {
		t.Fatalf("unbounded/incorrect retry: %v (%d clock reads)", err, reads)
	}
	if bb.count("tasks", "create") != 0 {
		t.Fatal("retry contention created a card")
	}
}
