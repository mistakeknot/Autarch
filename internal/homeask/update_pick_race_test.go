package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLegacyUpdatePickAfterReadLeavesPickedCardUntouched(t *testing.T) {
	f, fl, state := updateRig(t)
	r := testReq()
	r.Ask["ask_key"] = "pick-between-read-and-write"
	first, err := fl.FileOrUpdateCard(context.Background(), r, "")
	if err != nil {
		t.Fatal(err)
	}
	originalDesc, originalTitle := f.tasks[0].Desc, f.tasks[0].Title
	old := f.fail
	picked := false
	f.fail = func(args []string) *BBResult {
		result := old(args)
		if !picked && strings.Join(args[:min(3, len(args))], " ") == "home get --card" && argFlag(args, "--project") == "" {
			// The open snapshot has already been serialized. Record a pick
			// before returning it, so every subsequent write occurs after it.
			picked = true
			*state = "ruled"
		}
		return result
	}
	r.Title = "Changed title"
	r.Ask["question"] = "Changed question?"
	res, err := fl.FileOrUpdateCard(context.Background(), r, "")
	if !picked {
		t.Fatal("interleaving did not record a pick")
	}
	if f.tasks[0].Desc != originalDesc || f.tasks[0].Title != originalTitle || f.count("tasks", "update") != 0 {
		t.Fatalf("picked card edited: updates=%d title=%q err=%v", f.count("tasks", "update"), f.tasks[0].Title, err)
	}
	if err != nil || res.ID == first.ID || res.Updated || f.cards() != 2 {
		t.Fatalf("replacement=%+v err=%v cards=%d", res, err, f.cards())
	}
}

func TestLegacyReplacementRetryRepairsRoutingWithoutEditingPredecessor(t *testing.T) {
	f, fl, _ := updateRig(t)
	r := testReq()
	r.Ask["ask_key"] = "replacement-retry"
	first, err := fl.FileOrUpdateCard(context.Background(), r, "")
	if err != nil {
		t.Fatal(err)
	}
	original := f.tasks[0].Desc
	r.Ask["question"] = "Changed question?"
	old := f.fail
	f.fail = func(args []string) *BBResult {
		if args[0] == "tasks" && args[1] == "comment" {
			return badRes(1, "routing unavailable")
		}
		return old(args)
	}
	if _, err := fl.FileOrUpdateCard(context.Background(), r, ""); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("want uncertain routing outcome, got %v", err)
	}
	f.fail = old
	res, err := fl.FileOrUpdateCard(context.Background(), r, "")
	if err != nil || !res.Replay || res.Updated || res.ID == first.ID || f.cards() != 2 || res.ID != f.tasks[1].ID || AskingThread(f.tasks[1].Comments) != r.Thread || f.tasks[0].Desc != original || f.count("tasks", "update") != 0 {
		t.Fatalf("res=%+v err=%v cards=%d", res, err, f.cards())
	}
}
