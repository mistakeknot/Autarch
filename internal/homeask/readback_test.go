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
