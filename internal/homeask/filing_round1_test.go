package homeask

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpdateAtBudgetLimitIsFree(t *testing.T) {
	_, f, _ := updateRig(t)
	f.WeeklyBudget = 1
	r := testReq()
	r.Ask["ask_key"] = "one"
	if _, err := f.FileOrUpdateCard(context.Background(), r, ""); err != nil {
		t.Fatal(err)
	}
	r.Key = "edit"
	r.Ask["question"] = "Refreshed question?"
	got, err := f.FileOrUpdateCard(context.Background(), r, "")
	if err != nil || !got.Updated {
		t.Fatal(got, err)
	}
	budget, _ := f.BudgetStatus("autarch")
	if budget.Ordinary != 1 {
		t.Fatal(budget)
	}
}

func TestHeldSupersedesReleaseIsFree(t *testing.T) {
	_, f, _ := updateRig(t)
	f.WeeklyBudget = 1
	ctx := context.Background()
	r := testReq()
	first, err := f.FileCard(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	r.Key = "held-edit"
	r.Ask["question"] = "Refreshed question?"
	r.NonBlocking = true
	if _, err = f.FileOrUpdateCard(ctx, r, first.ID); err != nil {
		t.Fatal(err)
	}
	groups, err := f.Release(ctx)
	if err != nil || len(groups["other"]) != 1 || !groups["other"][0].Updated {
		t.Fatal(groups, err)
	}
	budget, _ := f.BudgetStatus("autarch")
	if budget.Ordinary != 1 {
		t.Fatal(budget)
	}
}

func TestHeldUpdateFallbackHonorsBudget(t *testing.T) {
	bb, f, _ := updateRig(t)
	f.WeeklyBudget = 1
	ctx := context.Background()
	r := testReq()
	r.Ask["ask_key"] = "one"
	if _, err := f.FileOrUpdateCard(ctx, r, ""); err != nil {
		t.Fatal(err)
	}
	r.Key = "held-fallback"
	r.Ask["question"] = "Refreshed?"
	r.NonBlocking = true
	if _, err := f.FileOrUpdateCard(ctx, r, ""); err != nil {
		t.Fatal(err)
	}
	bb.tasks[0].Status = "done"
	if _, err := f.Release(ctx); err == nil {
		t.Fatal("replacement bypassed full budget")
	}
	if bb.count("tasks", "create") != 1 {
		t.Fatal("created over budget")
	}
}

func TestHeldUpdateReplacementConsumesSlot(t *testing.T) {
	bb, f, _ := updateRig(t)
	f.WeeklyBudget = 2
	ctx := context.Background()
	r := testReq()
	r.Ask["ask_key"] = "one"
	if _, err := f.FileOrUpdateCard(ctx, r, ""); err != nil {
		t.Fatal(err)
	}
	r.Key = "held-replacement"
	r.Ask["question"] = "Refreshed?"
	r.NonBlocking = true
	if _, err := f.FileOrUpdateCard(ctx, r, ""); err != nil {
		t.Fatal(err)
	}
	bb.tasks[0].Status = "done"
	if _, err := f.Release(ctx); err != nil {
		t.Fatal(err)
	}
	count, _ := f.BudgetStatus("autarch")
	if count.Ordinary != 2 {
		t.Fatal("replacement not charged", count)
	}
}

func TestAttemptedTerminalEntriesAreRetired(t *testing.T) {
	for _, why := range []string{"missing-supersedes", "ruled-supersedes", "dead-owner", "missing-media", "project-mismatch"} {
		t.Run(why, func(t *testing.T) {
			_, f, homeState := updateRig(t)
			ctx := context.Background()
			r := testReq()
			supersedes := ""
			if why == "ruled-supersedes" {
				base := testReq()
				base.Key = "base"
				res, err := f.FileCard(ctx, base)
				if err != nil {
					t.Fatal(err)
				}
				supersedes = res.ID
				*homeState = "ruled"
			}
			r.Key = "held-" + why
			r.NonBlocking = true
			if why == "missing-supersedes" {
				supersedes = "gone"
			}
			if why == "dead-owner" {
				r.Owner = "thr_gone"
			}
			if why == "missing-media" {
				p := filepath.Join(t.TempDir(), "shot.png")
				os.WriteFile(p, []byte("image"), 0600)
				r.Attachments = []string{p}
			}
			if _, err := f.FileOrUpdateCard(ctx, r, supersedes); err != nil {
				t.Fatal(err)
			}
			state, path, unlock, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			e := state[r.Key]
			e.Attempted = true
			state[r.Key] = e
			err = saveFilingState(path, state)
			unlock()
			if err != nil {
				t.Fatal(err)
			}
			if why == "missing-media" {
				os.Remove(r.Attachments[0])
			}
			if why == "project-mismatch" {
				f.Check = func(string, string) error { return fmt.Errorf("%w: project/root mismatch", ErrInvalid) }
			}
			if _, err = f.Release(ctx); err == nil {
				t.Fatal("expected keyed refusal")
			}
			state, _, unlock, err = f.openState()
			if err != nil {
				t.Fatal(err)
			}
			e = state[r.Key]
			raw, _ := json.Marshal(e)
			unlock()
			if e.Held || !strings.Contains(string(raw), "terminal_reason") {
				t.Fatalf("not retired with reason: %s", raw)
			}
			budget, _ := f.BudgetStatus("autarch")
			want := 0
			if why == "ruled-supersedes" {
				want = 1
			}
			if budget.Ordinary != want {
				t.Fatal(budget)
			}
		})
	}
}

func TestReplaySkipsMediaAndBudgetAcrossStateDirs(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	ctx := context.Background()
	r := testReq()
	if _, err := bb.filer().FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	f := bb.filer()
	f.WeeklyBudget = 1
	held := testReq()
	held.Key = "full"
	held.NonBlocking = true
	if _, err := f.FileCard(ctx, held); err != nil {
		t.Fatal(err)
	}
	r.UI = true
	r.Type = "play"
	r.Attachments = []string{filepath.Join(t.TempDir(), "gone.png")}
	got, err := f.FileCard(ctx, r)
	if err != nil || !got.Replay {
		t.Fatal(got, err)
	}
	budget, _ := f.BudgetStatus("autarch")
	if budget.Ordinary != 1 {
		t.Fatal(budget)
	}
}

func TestPacificWeekBoundaryLocalClock(t *testing.T) {
	zone, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 15, 23, 59, 0, 0, zone)
	f := newFake(t, registryOnCreate).filer()
	f.WeeklyBudget = 1
	f.Now = func() time.Time { return now }
	r := testReq()
	r.NonBlocking = true
	ctx := context.Background()
	if _, err = f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	r.Key = "next"
	if _, err = f.FileCard(ctx, r); err == nil {
		t.Fatal("Sunday limit not enforced")
	}
	now = now.Add(time.Minute)
	if _, err = f.FileCard(ctx, r); err == nil {
		t.Fatal("Monday incorrectly reset the budget")
	}
	now = time.Date(2026, 11, 19, 12, 0, 0, 0, zone)
	if _, err = f.FileCard(ctx, r); err != nil {
		t.Fatal("Thursday noon slot not reset", err)
	}
	count, _ := f.BudgetStatus("autarch")
	if count.Ordinary != 1 {
		t.Fatal(count)
	}
}

func TestAttemptedOwnerBecomesTerminal(t *testing.T) {
	bb, f, _ := updateRig(t)
	ctx := context.Background()
	r := testReq()
	r.Owner = "thr_a"
	r.NonBlocking = true
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if len(args) > 2 && strings.Join(args[:3], " ") == "tasks create --project" {
			return BBResult{Code: 1}
		}
		return bb.run(ctx, env, args...)
	}
	if _, err := f.Release(ctx); err == nil {
		t.Fatal("expected uncertain attempt")
	}
	state, _, unlock, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	e := state[r.Key]
	unlock()
	if !e.Attempted || !e.Held {
		t.Fatal(e)
	}
	bb.threads["thr_a"] = "archived"
	f.Run = bb.run
	if _, err = f.Release(ctx); err == nil {
		t.Fatal("expected terminal owner refusal")
	}
	state, _, unlock, err = f.openState()
	if err != nil {
		t.Fatal(err)
	}
	e = state[r.Key]
	unlock()
	if e.Held || !e.Refunded || e.TerminalReason == "" {
		t.Fatal(e)
	}
	count, _ := f.BudgetStatus("autarch")
	if count.Ordinary != 0 {
		t.Fatal(count)
	}
}

func TestReleaseReportsKnownCardOnStateFailure(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	r := testReq()
	r.NonBlocking = true
	if _, err := f.FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	dir, _ := f.stateDir()
	bb.afterCreate = func(_ *memBB, _ string) {
		if err := os.WriteFile(filepath.Join(dir, "filings.json"), []byte("broken"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	report, err := f.ReleaseDetailed(context.Background())
	if err == nil || report.Failures[r.Key] == "" || len(report.Groups["other"]) != 1 {
		t.Fatal(report, err)
	}
}
