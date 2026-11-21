package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestUnroutedRefileRepairsAfterPredecessorDeleted(t *testing.T) {
	f, fl, _ := updateRig(t)
	r := testReq()
	r.Ask["ask_key"] = "deleted-predecessor"
	deriveRoutingTestKey(t, &r)
	ctx := context.Background()
	if _, err := fl.FileOrUpdateCard(ctx, r, ""); err != nil {
		t.Fatal(err)
	}
	f.tasks[0].Status = "done"
	original := f.fail
	f.fail = func(args []string) *BBResult {
		if args[0] == "tasks" && args[1] == "comment" {
			return badRes(1, "comment unavailable")
		}
		return original(args)
	}
	if _, err := fl.FileOrUpdateCard(ctx, r, ""); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatal(err)
	}
	pending := f.tasks[1]
	stored, err := ParseCard(pending.Desc)
	if err != nil {
		t.Fatal(err)
	}
	f.tasks = f.tasks[1:]
	f.fail = original
	r.Ask["question"] = "Updated question?"
	deriveRoutingTestKey(t, &r)
	foreign := r
	foreign.Thread = "thr_b"
	deriveRoutingTestKey(t, &foreign)
	if _, err := fl.FileOrUpdateCard(ctx, foreign, ""); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "unrouted Request from another filing") || len(pending.Comments) != 0 {
		t.Fatalf("foreign repair: err=%v comments=%v", err, pending.Comments)
	}
	res, err := fl.FileOrUpdateCard(ctx, r, "")
	if err != nil || res.ID != pending.ID || !res.Updated || AskingThread(pending.Comments) != r.Thread || f.cards() != 1 {
		t.Fatalf("res=%+v err=%v comments=%v cards=%d", res, err, pending.Comments, f.cards())
	}
	back, err := ParseCard(pending.Desc)
	if err != nil || back.Request != stored.Request {
		t.Fatalf("request changed: %+v %v", back, err)
	}
}

func TestRefileKeyProof(t *testing.T) {
	key := refileRequestKey("thr_a", "identity", "deleted-task")
	if err := CheckRequestKey(key); err != nil {
		t.Fatal(err)
	}
	if !matchesRefileRequestKey(key, "thr_a", "identity") {
		t.Fatal("owner proof rejected")
	}
	if matchesRefileRequestKey(key, "thr_b", "identity") || matchesRefileRequestKey(key, "thr_a", "changed") {
		t.Fatal("foreign thread or identity accepted")
	}
}
