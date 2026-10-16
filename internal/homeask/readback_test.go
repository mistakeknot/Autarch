package homeask

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Home indexes a card after the filer exits; the filer polls briefly so the caller sees card_key.
func TestFileCardPollsUntilHomeHasTheCardKey(t *testing.T) {
	f := newFake(t)
	polls := 0
	f.fail = func(args []string) *BBResult {
		if strings.Join(args[:min(len(args), 3)], " ") != "home get --card" {
			return nil
		}
		polls++
		if polls < 3 {
			return badRes(1, "unknown card")
		}
		r := okRes(map[string]any{"task_id": args[3], "card_key": "ck-1"})
		return &r
	}
	fl := f.filer()
	fl.ReadbackTimeout, fl.ReadbackInterval = 2*time.Second, time.Millisecond
	res, err := fl.FileCard(context.Background(), testReq())
	if err != nil || res.CardKey != "ck-1" || polls != 3 {
		t.Fatalf("res=%+v err=%v polls=%d", res, err, polls)
	}
}

// The poll is bounded: a Home that never indexes the card leaves card_key empty and does not fail the filing.
func TestFileCardReadbackIsBounded(t *testing.T) {
	f := newFake(t)
	f.fail = func(args []string) *BBResult {
		if strings.Join(args[:min(len(args), 3)], " ") == "home get --card" {
			return badRes(1, "unknown card")
		}
		return nil
	}
	fl := f.filer()
	fl.ReadbackTimeout, fl.ReadbackInterval = 30*time.Millisecond, 5*time.Millisecond
	start := time.Now()
	res, err := fl.FileCard(context.Background(), testReq())
	if err != nil || res.ID != "T1" || res.CardKey != "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("poll not bounded: %v", time.Since(start))
	}
}

// A read that stalls is cut off at ReadbackTimeout, not left to the per-call timeout.
func TestFileCardReadbackBoundsAStalledRead(t *testing.T) {
	f := newFake(t)
	base := f.run
	fl := f.filer()
	fl.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if strings.Join(args[:min(len(args), 3)], " ") == "home get --card" {
			select {
			case <-ctx.Done():
				return BBResult{TimedOut: true}
			case <-time.After(10 * time.Second):
				return BBResult{Code: 1}
			}
		}
		return base(ctx, env, args...)
	}
	fl.ReadbackTimeout, fl.ReadbackInterval = 100*time.Millisecond, 10*time.Millisecond
	start := time.Now()
	res, err := fl.FileCard(context.Background(), testReq())
	if err != nil || res.ID != "T1" || res.CardKey != "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("stalled read ran %v, want about 100ms", d)
	}
}

// With no ReadbackTimeout the filer reads nothing back.
func TestFileCardWithoutReadbackTimeoutDoesNotPoll(t *testing.T) {
	f := newFake(t)
	if _, err := f.filer().FileCard(context.Background(), testReq()); err != nil {
		t.Fatal(err)
	}
	if n := f.count("home", "get", "--card"); n != 0 {
		t.Fatalf("%d card reads", n)
	}
}
