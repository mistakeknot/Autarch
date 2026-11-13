package homeask

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func deriveRoutingTestKey(t *testing.T, req *CardRequest) {
	t.Helper()
	ident, err := RequestIdentity(*req)
	if err != nil {
		t.Fatal(err)
	}
	req.Key = DerivedKey(req.Thread, ident)
}
func TestUnroutedAskCannotBeClaimedByAnotherThread(t *testing.T) {
	for _, sameContent := range []bool{false, true} {
		t.Run(fmt.Sprint("same-content=", sameContent), func(t *testing.T) {
			f, fl, _ := updateRig(t)
			a := testReq()
			a.Ask["ask_key"] = "unrouted-owner"
			deriveRoutingTestKey(t, &a)
			original := f.fail
			f.fail = func(args []string) *BBResult {
				if args[0] == "tasks" && args[1] == "comment" {
					return badRes(1, "comment unavailable")
				}
				return original(args)
			}
			_, err := fl.FileOrUpdateCard(context.Background(), a, "")
			if !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatal(err)
			}
			f.fail = original
			b := a
			b.Thread = "thr_b"
			b.Ask = map[string]any{}
			for k, v := range a.Ask {
				b.Ask[k] = v
			}
			if !sameContent {
				b.Ask["question"] = "Another thread's question?"
			}
			deriveRoutingTestKey(t, &b)
			_, err = fl.FileOrUpdateCard(context.Background(), b, "")
			if !errors.Is(err, ErrInvalid) || len(f.tasks[0].Comments) != 0 || f.cards() != 1 || f.count("tasks", "update") != 0 {
				t.Fatalf("B err=%v cards=%d comments=%v", err, f.cards(), f.tasks[0].Comments)
			}
			res, err := fl.FileOrUpdateCard(context.Background(), a, "")
			if err != nil || res.ID != f.tasks[0].ID || AskingThread(f.tasks[0].Comments) != a.Thread || len(f.tasks[0].Comments) != 1 {
				t.Fatalf("A res=%+v err=%v comments=%v", res, err, f.tasks[0].Comments)
			}
		})
	}
}
func TestUnroutedRetryAppliesChangedContentImmediately(t *testing.T) {
	for _, refile := range []bool{false, true} {
		t.Run(fmt.Sprint("auto-refile=", refile), func(t *testing.T) {
			f, fl, _ := updateRig(t)
			r := testReq()
			r.Ask["ask_key"] = "repair-and-edit"
			r.Move = prMove()
			deriveRoutingTestKey(t, &r)
			if refile {
				if _, err := fl.FileOrUpdateCard(context.Background(), r, ""); err != nil {
					t.Fatal(err)
				}
				f.tasks[0].Status = "done"
			}
			original := f.fail
			f.fail = func(args []string) *BBResult {
				if args[0] == "tasks" && args[1] == "comment" {
					return badRes(1, "comment unavailable")
				}
				return original(args)
			}
			_, err := fl.FileOrUpdateCard(context.Background(), r, "")
			if !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatal(err)
			}
			pending := f.tasks[len(f.tasks)-1]
			stored, _ := ParseCard(pending.Desc)
			f.fail = original
			r.Title = "Updated title"
			r.Ask["question"] = "Updated question?"
			r.Ask["options"] = []any{map[string]any{"id": "ship", "label": "Ship this", "kind": "ruling-only"}, map[string]any{"id": "wait", "label": "Wait for review", "kind": "ruling-only"}}
			r.Move = scriptMove()
			deriveRoutingTestKey(t, &r)
			res, err := fl.FileOrUpdateCard(context.Background(), r, "")
			back, parseErr := ParseCard(pending.Desc)
			if err != nil || parseErr != nil || res.ID != pending.ID || !res.Updated || back.Question != "Updated question?" || pending.Title != r.Title || back.Request != stored.Request || !moveEqual(back.Move, r.Move) || AskingThread(pending.Comments) != r.Thread || len(pending.Comments) != 1 || f.count("tasks", "update") != 1 {
				t.Fatalf("res=%+v err=%v card=%+v comments=%v", res, err, back, pending.Comments)
			}
			opts := back.Ask["options"].([]any)
			if opts[0].(map[string]any)["label"] != "Ship this" {
				t.Fatalf("options=%v", opts)
			}
		})
	}
}
