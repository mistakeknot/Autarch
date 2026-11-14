package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWrittenHeldEditThenRuledIsDelivered(t *testing.T) {
	bb, f, _ := updateRig(t)
	ctx := context.Background()
	r := testReq()
	r.Ask["ask_key"] = "edited-one"
	first, err := f.FileOrUpdateCard(ctx, r, "")
	if err != nil {
		t.Fatal(err)
	}
	r.Key = "held-edit"
	r.Ask["question"] = "Updated question?"
	r.NonBlocking = true
	if _, err = f.FileOrUpdateCard(ctx, r, ""); err != nil {
		t.Fatal(err)
	}
	originalRead := bb.fail
	bb.fail = func(args []string) *BBResult {
		if strings.Join(args[:min(3, len(args))], " ") == "home get --card" && bb.count("tasks", "update") > 0 {
			return badRes(1, "post-update ruling read unavailable")
		}
		return originalRead(args)
	}
	if _, err = f.ReleaseDetailed(ctx); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("missing uncertain post-write read: %v", err)
	}
	if !strings.Contains(bb.tasks[0].Desc, "Updated question?") {
		t.Fatal("probe did not write edit")
	}
	state, _, unlock, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state[r.Key]
	unlock()
	if entry.DeliveredCard != first.ID {
		t.Errorf("successful edit target not recorded: %+v", entry)
	}
	bb.fail = func(args []string) *BBResult {
		if strings.Join(args[:min(3, len(args))], " ") == "home get --card" {
			res := okRes(CardView{TaskID: first.ID, ProjectID: "P1", State: "ruled", DecisionState: "ruled", CardKey: "P1-1"})
			return &res
		}
		return nil
	}
	for n := 0; n < 3; n++ {
		report, err := f.ReleaseDetailed(ctx)
		if err != nil || len(report.Failures) != 0 {
			t.Errorf("written edit reported as unwritten: %+v %v", report, err)
		}
		if n == 0 && (len(report.Groups["other"]) != 1 || report.Groups["other"][0].ID != first.ID) {
			t.Errorf("edit delivery absent: %+v", report)
		}
		if n > 0 && len(report.Groups) != 0 {
			t.Errorf("edit reported again: %+v", report)
		}
	}
	state, _, unlock, err = f.openState()
	if err != nil {
		t.Fatal(err)
	}
	entry = state[r.Key]
	unlock()
	if entry.Held || entry.Refunded || !entry.Uncharged || entry.DeliveredCard != first.ID || entry.TerminalReason != "" {
		t.Errorf("written edit not finalized uncharged: %+v", entry)
	}
	budget, err := f.BudgetStatus("autarch")
	if err != nil || budget.Ordinary != 1 {
		t.Fatal("edit changed charge", budget, err)
	}
	if bb.count("tasks", "create") != 1 || bb.count("tasks", "update") != 1 {
		t.Fatal("recovery mutated card again")
	}
}
