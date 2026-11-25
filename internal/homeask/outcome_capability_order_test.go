package homeask

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutcomeAskKeyRetryWithoutReader(t *testing.T) {
	ctx := context.Background()
	bb, f, req, capability := outcomeFilingFixture(t)
	read := bb.fail
	bb.fail = func(args []string) *BBResult {
		if len(args) >= 4 && strings.Join(args[:3], " ") == "home get --card" {
			result := okRes(CardView{TaskID: args[3], ProjectID: "P1", State: "open"})
			return &result
		}
		return read(args)
	}
	req.Ask["ask_key"] = "fixture-choice"
	req.Ask["review_by"] = "2026-01-01T00:00:00Z"
	if _, err := f.FileOrUpdateCard(ctx, req, ""); err != nil {
		t.Fatal(err)
	}
	req.Key = "fixture-edit"
	req.Ask["question"] = "Proceed with the revised choice?"
	updated, err := f.FileOrUpdateCard(ctx, req, "")
	if err != nil || !updated.Updated {
		t.Fatalf("want updated card, got %+v, %v", updated, err)
	}
	original := bb.tasks[0].Desc
	statePath := filepath.Join(f.StateDir, "filings.json")
	originalState, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	*capability = BBResult{Stdout: []byte(`{"schema":1}`)}
	probes := bb.count("home", "capabilities")
	replay, err := f.FileOrUpdateCard(ctx, req, "")
	if err != nil || !replay.Replay || replay.Updated || replay.ID != updated.ID || bb.count("home", "capabilities") != probes {
		t.Fatalf("want exact retry without a capability probe, got %+v, %v", replay, err)
	}
	req.Key = "fixture-new-edit"
	req.Ask["review_by"] = "2026-01-02T00:00:00Z"
	if _, err := f.FileOrUpdateCard(ctx, req, ""); !errors.Is(err, ErrHomeDown) {
		t.Fatalf("want reader refusal for changed bytes, got %v", err)
	}
	currentState, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(originalState, currentState) || bb.tasks[0].Desc != original ||
		bb.count("tasks", "create") != 1 || bb.count("tasks", "update") != 1 || bb.count("tasks", "comment") != 1 ||
		bb.count("home", "capabilities") != probes+1 {
		t.Fatalf("retry or refusal changed the card or filing state: %v", bb.calls)
	}
}

func TestOutcomeHeldReleaseRepairsRoutingWithoutReader(t *testing.T) {
	ctx := context.Background()
	bb, f, req, capability := outcomeFilingFixture(t)
	req.NonBlocking = true
	req.Ask["review_by"] = "2026-01-01T00:00:00Z"
	fresh := req
	fresh.Key = "fixture-fresh"
	for _, r := range []CardRequest{req, fresh} {
		if result, err := f.FileCard(ctx, r); err != nil || !result.Queued {
			t.Fatalf("want held request: %+v, %v", result, err)
		}
	}
	read := bb.fail
	bb.fail = func(args []string) *BBResult {
		if len(args) > 1 && args[0] == "tasks" && args[1] == "comment" {
			return badRes(1, "fixture comment unavailable")
		}
		return read(args)
	}
	if _, _, err := f.releaseOne(ctx, req.Key); !errors.Is(err, ErrOutcomeUnknown) || len(bb.tasks) != 1 || len(bb.tasks[0].Comments) != 0 {
		t.Fatalf("want one created card with missing routing, got %v", err)
	}
	original := bb.tasks[0].Desc
	bb.fail = read
	*capability = BBResult{Stdout: []byte(`{"schema":1}`)}
	probes := bb.count("home", "capabilities")
	got, _, err := f.releaseOne(ctx, req.Key)
	if err != nil || !got.Replay || got.ID != bb.tasks[0].ID || bb.count("home", "capabilities") != probes ||
		len(bb.tasks[0].Comments) != 1 || AskingThread(bb.tasks[0].Comments) != req.Thread {
		t.Fatalf("want comment-only recovery, got %+v, %v, calls %v", got, err, bb.calls)
	}
	if _, _, err := f.releaseOne(ctx, fresh.Key); !errors.Is(err, ErrHomeDown) {
		t.Fatalf("want reader refusal for a new held card, got %v", err)
	}
	state, _, unlock, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if state[req.Key].Held || state[req.Key].DeliveredCard != got.ID ||
		!state[fresh.Key].Held || state[fresh.Key].Attempted ||
		bb.count("tasks", "create") != 1 || bb.count("tasks", "update") != 0 || bb.tasks[0].Desc != original ||
		bb.count("home", "capabilities") != probes+1 {
		t.Fatalf("unexpected card writes or delivery state: %v", bb.calls)
	}
}
