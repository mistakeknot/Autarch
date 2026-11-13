package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func updateRig(t *testing.T) (*memBB, *CardFiler, *string) {
	f := newFake(t)
	f.threads = map[string]string{"thr_a": "live"}
	state := "open"
	f.fail = func(args []string) *BBResult {
		if strings.Join(args[:min(3, len(args))], " ") == "home get --card" {
			if argFlag(args, "--project") != "" {
				return badRes(2, "unknown option --project")
			}
			r := okRes(CardView{TaskID: args[3], ProjectID: "P1", State: state, DecisionState: state, CardKey: "P1-1"})
			return &r
		}
		return nil
	}
	return f, f.filer(), &state
}
func TestAskKeyUpdatePreservesRoutingAndRequest(t *testing.T) {
	f, fl, _ := updateRig(t)
	ctx := context.Background()
	r := testReq()
	r.Ask["ask_key"] = "ask-one"
	first, e := fl.FileOrUpdateCard(ctx, r, "")
	if e != nil || first.Replay || first.Updated || f.count("tasks", "create") != 1 {
		t.Fatalf("first=%+v err=%v", first, e)
	}
	original, _ := ParseCard(f.tasks[0].Desc)
	r.Key = "different-derived-request"
	r.Title = "Refreshed title"
	r.Ask["question"] = "Refreshed question?"
	r.Ask["options"] = []any{map[string]any{"id": "ship", "label": "Ship now", "kind": "ruling-only"}, map[string]any{"id": "wait", "label": "Wait", "kind": "ruling-only"}}
	res, e := fl.FileOrUpdateCard(ctx, r, "")
	c, _ := ParseCard(f.tasks[0].Desc)
	if e != nil || !res.Updated || res.ID != first.ID || f.cards() != 1 || c.Question != "Refreshed question?" || c.Request != original.Request || AskingThread(f.tasks[0].Comments) != "thr_a" || f.count("tasks", "comment") != 1 {
		t.Fatalf("res=%+v card=%+v err=%v", res, c, e)
	}
	if !strings.Contains(f.tasks[0].Desc, "Ship now") {
		t.Fatal("options did not update")
	}
}
func TestSupersedesGuardsAndChangedAskKey(t *testing.T) {
	for _, tc := range []struct {
		name, state, status, project, thread, owner string
		want                                        error
	}{
		{name: "changed key"},
		{name: "picked", state: "ruled", want: ErrAlreadyRuled},
		{name: "closed", state: "closed", want: ErrInvalid},
		{name: "canceled", status: "canceled", want: ErrInvalid},
		{name: "foreign project", project: "P2", want: ErrInvalid},
		{name: "foreign thread", thread: "thr_b", want: ErrInvalid},
		{name: "explicit owner", thread: "thr_b", owner: "thr_a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, fl, state := updateRig(t)
			r := testReq()
			r.Ask["ask_key"] = "old"
			first, e := fl.FileOrUpdateCard(context.Background(), r, "")
			if e != nil {
				t.Fatal(e)
			}
			if tc.state != "" {
				*state = tc.state
			}
			f.tasks[0].Status = tc.status
			if tc.project != "" {
				f.tasks[0].Project = tc.project
			}
			if tc.thread != "" {
				r.Thread = tc.thread
			}
			r.Owner = tc.owner
			r.Ask["ask_key"] = "new"
			r.Ask["question"] = "New question?"
			r.Key = "new-request"
			res, e := fl.FileOrUpdateCard(context.Background(), r, first.ID)
			if tc.want != nil {
				if !errors.Is(e, tc.want) || f.count("tasks", "update") != 0 {
					t.Fatalf("err=%v updates=%d", e, f.count("tasks", "update"))
				}
			} else if e != nil || !res.Updated || f.cards() != 1 || f.count("tasks", "comment") != 1 {
				t.Fatalf("res=%+v err=%v", res, e)
			}
		})
	}
}
func TestAskKeyClosedCreatesAndPickedRefuses(t *testing.T) {
	for _, state := range []string{"closed", "ruled"} {
		t.Run(state, func(t *testing.T) {
			f, fl, s := updateRig(t)
			r := testReq()
			r.Ask["ask_key"] = "same"
			if _, e := fl.FileOrUpdateCard(context.Background(), r, ""); e != nil {
				t.Fatal(e)
			}
			*s = state
			res, e := fl.FileOrUpdateCard(context.Background(), r, "")
			if state == "ruled" {
				if !errors.Is(e, ErrAlreadyRuled) || f.cards() != 1 {
					t.Fatalf("res=%+v err=%v cards=%d", res, e, f.cards())
				}
			} else if e != nil || f.cards() != 2 || res.Replay {
				t.Fatalf("res=%+v err=%v cards=%d", res, e, f.cards())
			}
		})
	}
}

func TestAskKeyUpdatesMove(t *testing.T) {
	f, fl, _ := updateRig(t)
	r := testReq()
	r.Ask["ask_key"] = "move-update"
	r.Move = prMove()
	first, e := fl.FileOrUpdateCard(context.Background(), r, "")
	if e != nil {
		t.Fatal(e)
	}
	r.Move = scriptMove()
	r.Key = "refreshed-move"
	res, e := fl.FileOrUpdateCard(context.Background(), r, "")
	c, parseErr := ParseCard(f.tasks[0].Desc)
	if e != nil || parseErr != nil || res.ID != first.ID || !res.Updated || !moveEqual(c.Move, r.Move) || f.count("tasks", "comment") != 1 {
		t.Fatalf("res=%+v err=%v parse=%v", res, e, parseErr)
	}
}

func TestAskKeyClosedPickedCardStillAlreadyRuled(t *testing.T) {
	f, fl, _ := updateRig(t)
	r := testReq()
	r.Ask["ask_key"] = "picked-then-closed"
	if _, err := fl.FileOrUpdateCard(context.Background(), r, ""); err != nil {
		t.Fatal(err)
	}
	f.fail = func(args []string) *BBResult {
		if strings.Join(args[:min(3, len(args))], " ") == "home get --card" {
			v := okRes(CardView{TaskID: args[3], ProjectID: "P1", State: "closed", DecisionState: "picked"})
			return &v
		}
		return nil
	}
	_, err := fl.FileOrUpdateCard(context.Background(), r, "")
	if !errors.Is(err, ErrAlreadyRuled) || f.cards() != 1 {
		t.Fatalf("err=%v cards=%d", err, f.cards())
	}
}

func TestStoppedSameKeyRefileUpdatesOneLegacyCard(t *testing.T) {
	f, fl, _ := updateRig(t)
	req := testReq()
	req.Ask["urgency"] = "blocker"
	req.Ask["ask_key"] = "release"
	desc, _, err := req.Description()
	if err != nil {
		t.Fatal(err)
	}
	f.tasks = append(f.tasks, &fakeTask{ID: "T1", Project: "P1", Title: req.Title, Desc: desc, Status: "todo", Created: "2026-09-01T00:00:00Z", Comments: []Comment{{Kind: "agent", ThreadID: "thr_a", CreatedAt: "2026-09-01T00:00:01Z"}}})
	req.Ask["stopped"] = "Release waits"
	result, err := fl.FileOrUpdateCard(context.Background(), req, "")
	if err != nil || !result.Updated || result.ID != "T1" || f.cards() != 1 || f.count("tasks", "create") != 0 {
		t.Fatalf("result=%+v err=%v cards=%d creates=%d", result, err, f.cards(), f.count("tasks", "create"))
	}
	card, err := ParseCard(f.tasks[0].Desc)
	if err != nil || card.Ask["stopped"] != "Release waits" {
		t.Fatalf("stopped not updated: %+v %v", card, err)
	}
}

func TestAskKeyUpdateWithLegacyPlugin(t *testing.T) {
	f, fl, _ := updateRig(t)
	fl.ReadbackTimeout = 20 * time.Millisecond
	fl.ReadbackInterval = time.Millisecond
	r := testReq()
	r.Ask["ask_key"] = "legacy-update"
	for _, phase := range []string{"create", "noop", "edit"} {
		if phase == "edit" {
			r.Title = "Updated question"
			r.Ask["question"] = "Updated question?"
		}
		res, err := fl.FileOrUpdateCard(context.Background(), r, "")
		if err != nil || res.CardKey != "P1-1" || f.count("tasks", "create") != 1 {
			t.Fatalf("phase=%s res=%+v err=%v creates=%d", phase, res, err, f.count("tasks", "create"))
		}
	}
}
