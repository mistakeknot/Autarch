package homeask

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestExemptVolumeDoesNotConsumeBudget(t *testing.T) {
	ctx := context.Background()
	f := newFake(t, registryOnCreate).filer()
	f.WeeklyBudget = 1
	r := testReq()
	r.NonBlocking = true
	r.Urgent = true
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	r.Key = "ordinary"
	r.Urgent = false
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	count, err := f.BudgetStatus("AUTARCH")
	if err != nil || count.Ordinary != 1 || count.Exempt != 1 {
		t.Fatal(count, err)
	}
}

func TestQueuedAttachmentsUseAbsolutePaths(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	r := testReq()
	r.NonBlocking = true
	p := filepath.Join(t.TempDir(), "flow.png")
	os.WriteFile(p, []byte("image"), 0600)
	cwd, _ := os.Getwd()
	relative, _ := filepath.Rel(cwd, p)
	r.Attachments = []string{relative}
	if _, err := f.FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	state, _, unlock, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if !filepath.IsAbs(state[r.Key].Request.Attachments[0]) {
		t.Fatal("queued a relative attachment")
	}
}

func TestDefiniteRefusalRefundsBudget(t *testing.T) {
	ctx := context.Background()
	f := newFake(t, registryOnCreate).filer()
	f.WeeklyBudget = 1
	r := testReq()
	r.Thread = ""
	if _, err := f.FileCard(ctx, r); err == nil {
		t.Fatal("accepted missing thread")
	}
	r.Key = "corrected"
	r.Thread = "thr_a"
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseContinuesAfterBadEntry(t *testing.T) {
	ctx := context.Background()
	f := newFake(t, registryOnCreate).filer()
	r := testReq()
	r.NonBlocking = true
	r.Type = "play"
	r.UI = true
	p := filepath.Join(t.TempDir(), "shot.png")
	os.WriteFile(p, []byte("image"), 0600)
	r.Attachments = []string{p}
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	os.Remove(p)
	good := testReq()
	good.Key = "good"
	good.NonBlocking = true
	good.Type = "play"
	if _, err := f.FileCard(ctx, good); err != nil {
		t.Fatal(err)
	}
	groups, err := f.Release(ctx)
	if err == nil || len(groups["play"]) != 1 {
		t.Fatal(groups, err)
	}
}

func TestConcurrentBudgetReservations(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	f.WeeklyBudget = 3
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := testReq()
			r.Key = fmt.Sprintf("held-%d", i)
			r.NonBlocking = true
			if _, err := f.FileCard(context.Background(), r); err == nil {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if accepted != 3 {
		t.Fatalf("accepted %d, want 3", accepted)
	}
}

func TestBudgetProjectAndKeepList(t *testing.T) {
	ctx := context.Background()
	f := newFake(t, registryOnCreate).filer()
	f.WeeklyBudget = 1
	r := testReq()
	r.NonBlocking = true
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	r.Key = "other-project"
	r.Ask["project"] = "another"
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	r.Key = "keep"
	r.KeepList = "credentials"
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	r.Key = "bad-marker"
	r.KeepList = "anything"
	if _, err := f.FileCard(ctx, r); err == nil {
		t.Fatal("accepted invented keep-list marker")
	}
}

func TestBudgetQueueAndRelease(t *testing.T) {
	ctx := context.Background()
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.WeeklyBudget = 1
	r := testReq()
	r.NonBlocking = true
	got, err := f.FileCard(ctx, r)
	if err != nil || !got.Queued || bb.count("tasks", "create") != 0 {
		t.Fatalf("queued=%+v err=%v", got, err)
	}
	if _, err = f.FileCard(ctx, r); err != nil {
		t.Fatal("queue replay", err)
	}
	second := testReq()
	second.Key = "second"
	if _, err = f.FileCard(ctx, second); err == nil {
		t.Fatal("budget was not enforced")
	}
	groups, err := f.Release(ctx)
	if err != nil || len(groups["other"]) != 1 {
		t.Fatalf("release=%v err=%v", groups, err)
	}
	groups, err = f.Release(ctx)
	if err != nil || len(groups) != 0 {
		t.Fatal("duplicate release", groups, err)
	}
	second.Urgent = true
	if _, err = f.FileCard(ctx, second); err != nil {
		t.Fatal(err)
	}
	f.Now = func() time.Time { return time.Date(2030, 1, 7, 0, 0, 0, 0, time.UTC) }
	third := testReq()
	third.Key = "third"
	if _, err = f.FileCard(ctx, third); err != nil {
		t.Fatal("ISO week reset", err)
	}
}

func TestCancelHeldRefunds(t *testing.T) {
	ctx := context.Background()
	f := newFake(t, registryOnCreate).filer()
	f.WeeklyBudget = 1
	r := testReq()
	r.NonBlocking = true
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := f.CancelHeld(r.Key); err != nil {
		t.Fatal(err)
	}
	r.Key = "replacement"
	if _, err := f.FileCard(ctx, r); err != nil {
		t.Fatal(err)
	}
}
