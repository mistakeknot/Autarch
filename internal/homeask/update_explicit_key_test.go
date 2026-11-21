package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// These fixtures exercise arbitrary explicit keys, without a thread-derived key
// or routing comment that could otherwise establish the caller's ownership.
func explicitUnroutedFixture(t *testing.T) (*memBB, *CardFiler, CardRequest) {
	t.Helper()
	f, fl, _ := updateRig(t)
	r := testReq()
	r.Key = "explicit-repair"
	r.Blocks = nil
	r.Ask["ask_key"] = "explicit-repair"
	original := f.fail
	f.fail = func(args []string) *BBResult {
		if args[0] == "tasks" && args[1] == "comment" {
			return badRes(1, "comment unavailable")
		}
		return original(args)
	}
	if _, err := fl.FileOrUpdateCard(context.Background(), r, ""); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("initial filing: %v", err)
	}
	f.fail = original
	if len(f.tasks) != 1 || len(f.tasks[0].Comments) != 0 {
		t.Fatalf("expected one unrouted card: %+v", f.tasks)
	}
	return f, fl, r
}

func TestExplicitKeyUnroutedForeignChangedContentRejected(t *testing.T) {
	f, fl, r := explicitUnroutedFixture(t)
	original := f.tasks[0].Desc
	r.Thread = "thr_b"
	r.Ask["question"] = "Foreign question?"
	_, err := fl.FileOrUpdateCard(context.Background(), r, "")
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "unrouted Request from another filing") || f.tasks[0].Desc != original || len(f.tasks[0].Comments) != 0 || f.count("tasks", "update") != 0 || f.cards() != 1 {
		t.Fatalf("foreign repair: err=%v comments=%v", err, f.tasks[0].Comments)
	}
}

func TestExplicitKeyUnroutedOriginalReplay(t *testing.T) {
	f, fl, r := explicitUnroutedFixture(t)
	res, err := fl.FileOrUpdateCard(context.Background(), r, "")
	if err != nil || !res.Replay || res.ID != f.tasks[0].ID || AskingThread(f.tasks[0].Comments) != r.Thread || f.cards() != 1 {
		t.Fatalf("original replay: res=%+v err=%v", res, err)
	}
}

func TestExplicitKeyDescriptionDoesNotBindFilingThread(t *testing.T) {
	a := testReq()
	a.Key, a.Blocks = "explicit-repair", nil
	b := a
	b.Thread = "thr_b"
	ad, ai, ae := a.Description()
	bd, bi, be := b.Description()
	if ae != nil || be != nil || ad != bd || ai != bi {
		t.Fatalf("expected indistinguishable stored descriptions: identities %q/%q errors %v/%v", ai, bi, ae, be)
	}
}
