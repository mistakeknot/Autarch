package homeask

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
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
