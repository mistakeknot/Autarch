package homeask

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAskKeyRetryRepairsMissingRouting(t *testing.T) {
	for _, refile := range []bool{false, true} {
		t.Run(fmt.Sprint("auto-refile=", refile), func(t *testing.T) {
			f, fl, _ := updateRig(t)
			r := testReq()
			r.Ask["ask_key"] = "repair-routing"
			ctx := context.Background()
			if refile {
				if _, err := fl.FileOrUpdateCard(ctx, r, ""); err != nil {
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
			_, err := fl.FileOrUpdateCard(ctx, r, "")
			if !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatalf("first err=%v", err)
			}
			unrouted := f.tasks[len(f.tasks)-1]
			line, _ := requestOf(unrouted.Desc)
			f.fail = original
			res, err := fl.FileOrUpdateCard(ctx, r, "")
			if err != nil || res.ID != unrouted.ID || res.Key != line.Key || !res.Replay || AskingThread(unrouted.Comments) != r.Thread || len(unrouted.Comments) != 1 || f.cards() != 1+btoi(refile) {
				t.Fatalf("res=%+v err=%v cards=%d comments=%v", res, err, f.cards(), unrouted.Comments)
			}
		})
	}
}
func btoi(v bool) int {
	if v {
		return 1
	}
	return 0
}

func TestAskKeySkipsForeignClosedBeforeOwnership(t *testing.T) {
	for _, status := range []string{"done", "canceled"} {
		t.Run(status, func(t *testing.T) {
			f, fl, _ := updateRig(t)
			r := testReq()
			r.Ask["ask_key"] = "shared-key"
			if _, err := fl.FileOrUpdateCard(context.Background(), r, ""); err != nil {
				t.Fatal(err)
			}
			f.tasks[0].Status = status
			r.Thread = "thr_b"
			res, err := fl.FileOrUpdateCard(context.Background(), r, "")
			if err != nil || res.Replay || f.cards() != 2 {
				t.Fatalf("res=%+v err=%v cards=%d", res, err, f.cards())
			}
		})
	}
}

func TestAskKeySearchBoundsShowCallsAndSkipsVanished(t *testing.T) {
	for _, vanished := range []bool{false, true} {
		t.Run(fmt.Sprint("vanished=", vanished), func(t *testing.T) {
			f, fl, _ := updateRig(t)
			f.pageSize = 500
			for i := 0; i < 100; i++ {
				f.tasks = append(f.tasks, &fakeTask{ID: fmt.Sprintf("noise%d", i), Project: "P1", Desc: "unrelated task"})
			}
			f.tasks = append(f.tasks, &fakeTask{ID: "gone", Project: "P1", Desc: `ask_key matching-key`})
			old := f.fail
			f.fail = func(args []string) *BBResult {
				if args[0] == "tasks" && args[1] == "show" && args[2] == "gone" && vanished {
					return badRes(1, "task not found")
				}
				return old(args)
			}
			r := testReq()
			r.Ask["ask_key"] = "matching-key"
			res, err := fl.FileOrUpdateCard(context.Background(), r, "")
			if err != nil || res.ID == "" {
				t.Fatalf("res=%+v err=%v", res, err)
			}
			if n := f.count("tasks", "show"); n > 1 {
				t.Fatalf("show calls=%d; want at most the matching row", n)
			}
		})
	}
}
func TestSupersedesReadOutageRemainsUnavailable(t *testing.T) {
	f, fl, _ := updateRig(t)
	old := f.fail
	f.fail = func(args []string) *BBResult {
		if args[0] == "tasks" && args[1] == "show" {
			return badRes(1, "service unavailable")
		}
		return old(args)
	}
	_, err := fl.FileOrUpdateCard(context.Background(), testReq(), "T1")
	if !errors.Is(err, ErrHomeDown) || errors.Is(err, ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}
func TestIdenticalAskKeyRefilingDoesNotWrite(t *testing.T) {
	f, fl, _ := updateRig(t)
	r := testReq()
	r.Ask["ask_key"] = "no-op"
	first, err := fl.FileOrUpdateCard(context.Background(), r, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := fl.FileOrUpdateCard(context.Background(), r, "")
	if err != nil || res.ID != first.ID || !res.Replay || res.Updated || f.count("tasks", "update") != 0 || f.count("tasks", "comment") != 1 {
		t.Fatalf("res=%+v err=%v updates=%d", res, err, f.count("tasks", "update"))
	}
}
func TestAskKeyReportsPickDuringUpdate(t *testing.T) {
	f, fl, _ := updateRig(t)
	r := testReq()
	r.Ask["ask_key"] = "racing-pick"
	if _, err := fl.FileOrUpdateCard(context.Background(), r, ""); err != nil {
		t.Fatal(err)
	}
	updated := false
	old := f.fail
	f.fail = func(args []string) *BBResult {
		if args[0] == "tasks" && args[1] == "update" {
			updated = true
		}
		if updated && strings.Join(args[:min(3, len(args))], " ") == "home get --card" {
			v := okRes(CardView{TaskID: args[3], State: "ruled", DecisionState: "picked"})
			return &v
		}
		return old(args)
	}
	r.Ask["question"] = "A refreshed question?"
	_, err := fl.FileOrUpdateCard(context.Background(), r, "")
	if !errors.Is(err, ErrAlreadyRuled) || !strings.Contains(err.Error(), "updated") || f.count("tasks", "update") != 1 {
		t.Fatalf("err=%v updates=%d", err, f.count("tasks", "update"))
	}
}
