package homeask

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestHeldAskKeyReplacesPayloadWithoutNewCharge(t *testing.T) {
	for _, nonBlocking := range []bool{false, true} {
		t.Run(map[bool]string{false: "weekday", true: "nonblocking"}[nonBlocking], func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			f.WeeklyBudget = 1
			now := pacificTestTime(t, "2026-11-16 10:00:00")
			if nonBlocking {
				now = pacificTestTime(t, "2026-11-13 10:00:00")
			}
			f.Now = func() time.Time { return now }
			first := testReq()
			first.Key = "first"
			first.NonBlocking = nonBlocking
			first.Ask["ask_key"] = "stable-ask"
			original, err := f.FileOrUpdateCard(context.Background(), first, "")
			if err != nil || !original.Queued {
				t.Fatal(original, err)
			}
			next := testReq()
			next.Key = "next"
			next.NonBlocking = nonBlocking
			next.Ask["ask_key"] = "stable-ask"
			next.Ask["question"] = "Updated facts: ship now?"
			replacement, err := f.FileOrUpdateCard(context.Background(), next, "")
			if err != nil || !replacement.Queued || replacement.Key != original.Key {
				t.Fatalf("held update: %+v %v", replacement, err)
			}
			for _, req := range []CardRequest{first, next} {
				replay, err := f.FileOrUpdateCard(context.Background(), req, "")
				if err != nil || !replay.Queued || !replay.Replay {
					t.Fatalf("replay %+v: %+v %v", req, replay, err)
				}
			}
			reused := testReq()
			reused.Key = first.Key
			reused.Ask["ask_key"] = "stable-ask"
			reused.Ask["question"] = "Changed under the same request key?"
			if _, err := f.FileOrUpdateCard(context.Background(), reused, ""); !errors.Is(err, ErrInvalid) {
				t.Fatalf("request-key reuse accepted: %v", err)
			}
			state, _, done, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			done()
			if len(state) != 1 || state[original.Key].Request.Ask["question"] != next.Ask["question"] {
				t.Fatalf("wrong queued payload: %+v", state)
			}
			status, err := f.BudgetStatus("autarch")
			if err != nil || status.Ordinary != 1 {
				t.Fatal(status, err)
			}
			now = pacificTestTime(t, "2026-11-19 12:00:00")
			report, err := f.ReleaseDetailed(context.Background())
			if err != nil || len(report.Groups["other"]) != 1 || bb.count("tasks", "create") != 1 {
				t.Fatal(report, err)
			}
			for _, req := range []CardRequest{first, next} {
				replay, err := f.FileOrUpdateCard(context.Background(), req, "")
				if err != nil || replay.ID == "" || !replay.Replay {
					t.Fatalf("delivered replay: %+v %v", replay, err)
				}
			}
			if bb.count("tasks", "create") != 1 {
				t.Fatal("replay created another card")
			}
			card, err := ParseCard(bb.tasks[0].Desc)
			if err != nil || card.Ask["question"] != next.Ask["question"] {
				t.Fatalf("released stale payload: %+v %v", card, err)
			}
			bb.tasks[0].Deleted = true
			if _, err := f.FileOrUpdateCard(context.Background(), first, ""); err == nil {
				t.Fatal("revision replay accepted a deleted card")
			}

		})
	}
}

func TestFileCarriesExemptionsDuringHold(t *testing.T) {
	script := filepath.Join(t.TempDir(), "check.sh")
	body := []byte("#!/bin/sh\ntrue\n")
	if err := os.WriteFile(script, body, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	rootJSON, err := json.Marshal(RootRun{Script: script, SHA256: fmt.Sprintf("%x", digest), Timeout: 30, Set: "sample"})
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`,"keep_list":"spend"`, `,"urgent":true`, `,"root_run":` + string(rootJSON)} {
		t.Run(extra, func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
			a, err := Parse([]byte(`{"v":1,"kind":"decide","asker":"thread","thread":"thr_a","project":"autarch","project_root":"/sample","question":"Proceed?","options":[{"id":"yes","label":"Yes","kind":"ruling-only"},{"id":"no","label":"No","kind":"ruling-only"}]` + extra + `}`))
			if err != nil {
				t.Fatal(err)
			}
			id, err := f.File(context.Background(), a)
			if err != nil || id == "" || bb.count("tasks", "create") != 1 {
				t.Fatalf("exemption dropped: id=%q err=%v", id, err)
			}
		})
	}
}

func TestHeldAskUpdatesSerializeAndKeepProjectScope(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.WeeklyBudget = 1
	f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
	const count = 8
	var wg sync.WaitGroup
	failures := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := testReq()
			r.Key = fmt.Sprintf("edit-%d", i)
			r.Ask["ask_key"] = "stable"
			r.Ask["question"] = fmt.Sprintf("Facts %d?", i)
			res, err := f.FileOrUpdateCard(context.Background(), r, "")
			if err != nil || !res.Queued {
				failures <- fmt.Errorf("%+v %v", res, err)
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	state, _, done, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	done()
	if len(state) != 1 {
		t.Fatalf("concurrent submissions created %d reservations", len(state))
	}
	status, err := f.BudgetStatus("autarch")
	if err != nil || status.Ordinary != 1 {
		t.Fatal(status, err)
	}
	other := testReq()
	other.Key = "other-project"
	other.Ask["ask_key"] = "stable"
	other.Ask["project"] = "another"
	if _, err := f.FileOrUpdateCard(context.Background(), other, ""); err != nil {
		t.Fatal(err)
	}
	state, _, done, err = f.openState()
	if err != nil {
		t.Fatal(err)
	}
	done()
	if len(state) != 2 {
		t.Fatalf("merged different Home projects: %+v", state)
	}
}
