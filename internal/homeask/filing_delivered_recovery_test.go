package homeask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestHeldDeliveredCardRecovery(t *testing.T) {
	for _, replacement := range []bool{true, false} {
		name := "delivered-project-changed"
		if replacement {
			name = "replacement-delivered-then-ruled"
		}
		t.Run(name, func(t *testing.T) {
			bb, f, _ := updateRig(t)
			ctx := context.Background()
			r := testReq()
			if replacement {
				r.Ask["ask_key"] = "recover-one"
				if _, err := f.FileOrUpdateCard(ctx, r, ""); err != nil {
					t.Fatal(err)
				}
				r.Key = "held-replacement"
				r.Ask["question"] = "Updated question?"
			}
			r.NonBlocking = true
			var err error
			if replacement {
				_, err = f.FileOrUpdateCard(ctx, r, "")
			} else {
				_, err = f.FileCard(ctx, r)
			}
			if err != nil {
				t.Fatal(err)
			}
			if replacement {
				bb.tasks[0].Status = "done"
			}
			cardID := "T1"
			wantCharged := 1
			if replacement {
				cardID = "T2"
				wantCharged = 2
			}
			originalRead := bb.fail
			bb.fail = func(args []string) *BBResult {
				if strings.Join(args[:min(3, len(args))], " ") == "home get --card" && args[3] == cardID {
					return badRes(1, "index unavailable")
				}
				return originalRead(args)
			}
			f.ReadbackTimeout = 10 * time.Millisecond
			f.ReadbackInterval = time.Millisecond
			if _, err = f.ReleaseDetailed(ctx); !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatalf("expected post-create uncertainty: %v", err)
			}
			if len(bb.tasks) != wantCharged {
				t.Fatalf("created %d cards, want %d", len(bb.tasks), wantCharged)
			}
			state, _, unlock, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			entry := state[r.Key]
			unlock()
			if !entry.Held || !entry.Attempted {
				t.Fatalf("probe did not leave attempted hold: %+v", entry)
			}
			if replacement && (entry.CreationKey == "" || entry.CreationKey == r.Key) {
				t.Fatal("replacement did not use refile key")
			}
			bb.fail = func(args []string) *BBResult {
				if strings.Join(args[:min(3, len(args))], " ") == "home get --card" {
					status := "closed"
					if args[3] == cardID {
						status = "ruled"
					}
					res := okRes(CardView{TaskID: args[3], ProjectID: "P1", State: status, DecisionState: status, CardKey: "P1-1"})
					return &res
				}
				return nil
			}
			if !replacement {
				f.Check = func(string, string) error { return fmt.Errorf("%w: project binding changed", ErrInvalid) }
			}
			for n := 1; n <= 3; n++ {
				report, err := f.ReleaseDetailed(ctx)
				t.Logf("release %d: groups=%v failures=%v err=%v", n, report.Groups, report.Failures, err)
				if err != nil {
					t.Errorf("delivered card stayed stuck: %v", err)
				}
				if n == 1 && (len(report.Groups["other"]) != 1 || report.Groups["other"][0].ID != cardID) {
					t.Errorf("delivered card absent from groups: %+v", report)
				}
				if n > 1 && len(report.Groups) != 0 {
					t.Errorf("delivery repeated: %+v", report)
				}
			}
			if err := f.CancelHeld(r.Key); err != nil {
				t.Errorf("finalized entry not clearable: %v", err)
			}
			state, _, unlock, err = f.openState()
			if err != nil {
				t.Fatal(err)
			}
			entry = state[r.Key]
			raw, _ := json.Marshal(entry)
			unlock()
			var recorded map[string]any
			json.Unmarshal(raw, &recorded)
			if entry.Held || entry.Refunded || recorded["delivered_card"] != cardID {
				t.Errorf("not finalized as charged delivery: %s", raw)
			}
			budget, err := f.BudgetStatus("autarch")
			if err != nil || budget.Ordinary != wantCharged {
				t.Errorf("delivery charge changed: %+v %v", budget, err)
			}
			if len(bb.tasks) != wantCharged {
				t.Fatal("recovery created a duplicate")
			}
		})
	}
}
