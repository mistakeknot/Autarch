package homeask

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSameHeldRequestEscalates(t *testing.T) {
	for _, askKey := range []bool{false, true} {
		for _, kind := range []string{"urgent", "keep-list", "merge", "root-run", "noon"} {
			t.Run(kind+map[bool]string{false: "/no-ask-key", true: "/ask-key"}[askKey], func(t *testing.T) {
				bb := newFake(t, registryOnCreate)
				f := bb.filer()
				now := pacificTestTime(t, "2026-11-16 10:00:00")
				f.Now = func() time.Time { return now }
				first := testReq()
				if askKey {
					first.Ask["ask_key"] = "stable"
				}
				if got, err := f.FileOrUpdateCard(context.Background(), first, ""); err != nil || !got.Queued {
					t.Fatal(got, err)
				}
				next := first
				switch kind {
				case "urgent":
					next.Urgent = true
				case "keep-list":
					next.KeepList = "spend"
				case "merge":
					next.Type = "merge"
				case "root-run":
					next.RootRun = &RootRun{Script: "/sample/check.sh", SHA256: strings.Repeat("a", 64), Timeout: 30, Set: "sample"}
				case "noon":
					now = pacificTestTime(t, "2026-11-19 12:00:00")
				}
				result, err := f.FileOrUpdateCard(context.Background(), next, "")
				if err != nil || result.Queued || result.ID == "" || bb.count("tasks", "create") != 1 {
					t.Fatalf("same request held: %+v %v", result, err)
				}

				for _, req := range []CardRequest{first, next} {
					replay, err := f.FileOrUpdateCard(context.Background(), req, "")
					if err != nil || replay.ID != result.ID {
						t.Fatalf("request did not replay delivered card: %+v %v", replay, err)
					}
				}
				status, err := f.BudgetStatus("autarch")
				if err != nil {
					t.Fatal(err)
				}
				ordinary := 1
				exempt := 0
				if kind == "urgent" || kind == "keep-list" {
					ordinary = 0
					exempt = 1
				}
				if status.Ordinary != ordinary || status.Exempt != exempt {
					t.Fatal(status)
				}
			})
		}
	}
}

func TestRetiredRequestAndMergedRevisionReplayDeliveredCard(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
	first := testReq()
	first.Key = "old"
	first.Ask["ask_key"] = "stable"
	if _, err := f.FileOrUpdateCard(context.Background(), first, ""); err != nil {
		t.Fatal(err)
	}
	second := testReq()
	second.Key = "revision"
	second.Ask["ask_key"] = "stable"
	second.Ask["question"] = "Fresh facts?"
	if _, err := f.FileOrUpdateCard(context.Background(), second, ""); err != nil {
		t.Fatal(err)
	}
	immediate := second
	immediate.Key = "urgent"
	immediate.Urgent = true
	delivered, err := f.FileOrUpdateCard(context.Background(), immediate, "")
	if err != nil || delivered.ID == "" {
		t.Fatal(delivered, err)
	}
	for _, req := range []CardRequest{first, second} {
		got, err := f.FileOrUpdateCard(context.Background(), req, "")
		if err != nil || got.ID != delivered.ID || !got.Replay {
			t.Fatalf("retired replay: %+v %v", got, err)
		}
	}
	if bb.count("tasks", "create") != 1 {
		t.Fatal("duplicate creation")
	}
}

func TestConcurrentSameKeyEscalationsCreateOnce(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
	first := testReq()
	if _, err := f.FileOrUpdateCard(context.Background(), first, ""); err != nil {
		t.Fatal(err)
	}
	next := first
	next.Urgent = true
	var wg sync.WaitGroup
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := f.FileOrUpdateCard(context.Background(), next, "")
			if err != nil || got.ID != "T1" {
				failures <- fmt.Errorf("%+v %v", got, err)
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if bb.count("tasks", "create") != 1 {
		t.Fatal("duplicate card on escalation")
	}
}

func TestEscalatingOldRevisionKeepsLatestHeldPayload(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
	first := testReq()
	first.Key = "first"
	first.Ask["ask_key"] = "stable"
	if _, err := f.FileOrUpdateCard(context.Background(), first, ""); err != nil {
		t.Fatal(err)
	}
	latest := testReq()
	latest.Key = "latest"
	latest.Ask["ask_key"] = "stable"
	latest.Ask["question"] = "Updated facts?"
	if _, err := f.FileOrUpdateCard(context.Background(), latest, ""); err != nil {
		t.Fatal(err)
	}
	first.Urgent = true
	result, err := f.FileOrUpdateCard(context.Background(), first, "")
	if err != nil || result.ID == "" {
		t.Fatal(result, err)
	}
	card, err := ParseCard(bb.tasks[0].Desc)
	if err != nil || card.Ask["question"] != latest.Ask["question"] {
		t.Fatal(card, err)
	}
	replay, err := f.FileOrUpdateCard(context.Background(), latest, "")
	if err != nil || replay.ID != result.ID {
		t.Fatal(replay, err)
	}
}

func TestRetiredReplayAfterUpdatingExistingCard(t *testing.T) {
	bb, f, _ := updateRig(t)
	f.ReadbackTimeout = 10 * time.Millisecond
	now := pacificTestTime(t, "2026-11-13 10:00:00")
	f.Now = func() time.Time { return now }
	live := testReq()
	live.Key = "live"
	live.Ask["ask_key"] = "stable"
	if _, err := f.FileOrUpdateCard(context.Background(), live, ""); err != nil {
		t.Fatal(err)
	}
	now = pacificTestTime(t, "2026-11-16 10:00:00")
	queued := testReq()
	queued.Key = "held"
	queued.Ask["ask_key"] = "stable"
	queued.Ask["question"] = "Updated facts?"
	if result, err := f.FileOrUpdateCard(context.Background(), queued, ""); err != nil || !result.Queued {
		t.Fatal(result, err)
	}
	urgent := queued
	urgent.Key = "urgent"
	urgent.Urgent = true
	delivered, err := f.FileOrUpdateCard(context.Background(), urgent, "")
	if err != nil || !delivered.Updated {
		t.Fatal(delivered, err)
	}
	replay, err := f.FileOrUpdateCard(context.Background(), queued, "")
	if err != nil || replay.ID != delivered.ID || replay.CardKey == "" {
		t.Fatalf("updated card not replayed: %+v %v", replay, err)
	}
	if bb.count("tasks", "create") != 1 {
		t.Fatal("duplicate card")
	}
}

func TestRetiredReplayWhenExistingCardNeedsNoEdit(t *testing.T) {
	bb, f, _ := updateRig(t)
	f.ReadbackTimeout = 10 * time.Millisecond
	now := pacificTestTime(t, "2026-11-13 10:00:00")
	f.Now = func() time.Time { return now }
	live := testReq()
	live.Key = "live"
	live.Ask["ask_key"] = "stable"
	if _, err := f.FileOrUpdateCard(context.Background(), live, ""); err != nil {
		t.Fatal(err)
	}
	now = pacificTestTime(t, "2026-11-16 10:00:00")
	queued := testReq()
	queued.Key = "held"
	queued.Ask["ask_key"] = "stable"
	if result, err := f.FileOrUpdateCard(context.Background(), queued, ""); err != nil || !result.Queued {
		t.Fatal(result, err)
	}
	queued.Urgent = true
	delivered, err := f.FileOrUpdateCard(context.Background(), queued, "")
	if err != nil || delivered.ID != "T1" || delivered.Updated {
		t.Fatal(delivered, err)
	}
	queued.Urgent = false
	replay, err := f.FileOrUpdateCard(context.Background(), queued, "")
	if err != nil || replay.ID != "T1" || replay.CardKey == "" || bb.count("tasks", "create") != 1 {
		t.Fatalf("existing card not replayed: %+v %v", replay, err)
	}
}
