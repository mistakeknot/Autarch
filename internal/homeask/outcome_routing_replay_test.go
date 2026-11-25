package homeask

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func failFixtureRouting(bb *memBB) func() {
	read := bb.fail
	bb.fail = func(args []string) *BBResult {
		if len(args) >= 2 && args[0] == "tasks" && args[1] == "comment" {
			return badRes(1, "fixture routing unavailable")
		}
		return read(args)
	}
	return func() { bb.fail = read }
}

func TestOutcomeUnchangedUpdateRepairsRoutingWithoutReader(t *testing.T) {
	ctx := context.Background()
	bb, f, req, capability := outcomeFilingFixture(t)
	req.Ask["ask_key"] = "fixture-choice"
	req.Ask["review_by"] = "2026-01-01T00:00:00Z"
	restore := failFixtureRouting(bb)
	first, err := f.FileOrUpdateCard(ctx, req, "")
	restore()
	if !errors.Is(err, ErrOutcomeUnknown) || first.ID == "" || len(bb.tasks) != 1 || len(bb.tasks[0].Comments) != 0 {
		t.Fatalf("expected a filed card with interrupted routing, got %+v, %v", first, err)
	}
	original := bb.tasks[0].Desc
	statePath := filepath.Join(f.StateDir, "filings.json")
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	*capability = BBResult{Stdout: []byte(`{"schema":1}`)}
	probes, comments := bb.count("home", "capabilities"), bb.count("tasks", "comment")
	retry, err := f.FileOrUpdateCard(ctx, req, "")
	if err != nil || !retry.Replay || retry.Updated || retry.ID != first.ID || retry.Key != first.Key {
		t.Fatalf("expected a routing-only replay, got %+v, %v", retry, err)
	}
	after, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(state, after) || bb.tasks[0].Desc != original ||
		bb.count("tasks", "create") != 1 || bb.count("tasks", "update") != 0 ||
		bb.count("home", "capabilities") != probes || bb.count("tasks", "comment") != comments+1 ||
		len(bb.tasks[0].Comments) != 1 || AskingThread(bb.tasks[0].Comments) != req.Thread {
		t.Fatalf("expected one routing comment and no other writes or capability probe: %v, calls %v", err, bb.calls)
	}
}

func TestOutcomeConcurrentHeldReleasesRouteOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bb, f, req, capability := outcomeFilingFixture(t)
	req.NonBlocking = true
	req.Ask["review_by"] = "2026-01-01T00:00:00Z"
	if result, err := f.FileCard(ctx, req); err != nil || !result.Queued {
		t.Fatalf("expected a held request, got %+v, %v", result, err)
	}
	restore := failFixtureRouting(bb)
	first, _, err := f.releaseOne(ctx, req.Key)
	restore()
	if !errors.Is(err, ErrOutcomeUnknown) || first.ID == "" || len(bb.tasks) != 1 || len(bb.tasks[0].Comments) != 0 {
		t.Fatalf("expected a held card with interrupted routing, got %+v, %v", first, err)
	}
	original := bb.tasks[0].Desc
	probes, comments := bb.count("home", "capabilities"), bb.count("tasks", "comment")
	*capability = BBResult{Stdout: []byte(`{"schema":1}`)}

	commentStarted := make(chan struct{}, 2)
	finishComment := make(chan struct{})
	run := f.Run
	f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if len(args) >= 2 && args[0] == "tasks" && args[1] == "comment" {
			commentStarted <- struct{}{}
			select {
			case <-finishComment:
			case <-ctx.Done():
				return BBResult{Err: ctx.Err()}
			}
		}
		return run(ctx, env, args...)
	}
	type attempt struct {
		card CardResult
		err  error
	}
	finished := make(chan attempt, 2)
	release := func() {
		card, _, err := f.releaseOne(ctx, req.Key)
		finished <- attempt{card, err}
	}
	go release()
	select {
	case <-commentStarted:
	case <-ctx.Done():
		t.Fatal("first release did not reach the routing comment")
	}
	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		release()
	}()
	<-secondStarted
	close(finishComment)

	deliveries := 0
	for range 2 {
		select {
		case result := <-finished:
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.card.ID != "" {
				deliveries++
				if result.card.ID != first.ID || !result.card.Replay {
					t.Fatalf("release returned a different card: %+v", result.card)
				}
			}
		case <-ctx.Done():
			t.Fatal("concurrent releases did not finish")
		}
	}
	if deliveries != 1 || bb.count("tasks", "comment") != comments+1 ||
		len(bb.tasks[0].Comments) != 1 || AskingThread(bb.tasks[0].Comments) != req.Thread ||
		bb.count("tasks", "create") != 1 || bb.count("tasks", "update") != 0 || bb.tasks[0].Desc != original ||
		bb.count("home", "capabilities") != probes {
		t.Fatalf("expected one comment and only the existing card, got %d deliveries, calls %v", deliveries, bb.calls)
	}
}
