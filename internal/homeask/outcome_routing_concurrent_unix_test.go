//go:build !windows

package homeask

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestOutcomeConcurrentHeldReleasesRouteOnce(t *testing.T) {
	bb, f, req, card := heldOutcomeWithInterruptedRouting(t)
	lockPath := filepath.Join(f.StateDir, "delivery-"+req.Key+".lock")
	// With the per-key delivery lock removed, the shared Request lock still
	// serializes routing. Only the handshake observes the lock-order change.
	// The assertions below guard the delivery outcome.
	cards := runOverlappingHeldReleases(t, f, req.Key, req.Key, lockPath)
	if !cards[0].Replay || cards[0].ID != card.ID || cards[1].ID != "" {
		t.Fatalf("expected exactly one replay of the existing card, got %+v", cards)
	}
	assertOneHeldRoutingComment(t, bb, req, cards[:]...)
	assertHeldDeliveries(t, f, card, req.Key)
}

func TestOutcomeConcurrentHeldEntriesForSameCardRouteOnce(t *testing.T) {
	bb, f, req, card := heldOutcomeWithInterruptedRouting(t)
	otherKey := addHeldEntryForSameCard(t, f, req.Key)

	lockPath, err := f.lockPath(req.Key)
	if err != nil {
		t.Fatal(err)
	}
	cards := runOverlappingHeldReleases(t, f, req.Key, otherKey, lockPath)
	for _, result := range cards {
		if !result.Replay || result.ID != card.ID || result.Key != req.Key {
			t.Fatalf("expected both held entries to recover the same card, got %+v", cards)
		}
	}
	assertOneHeldRoutingComment(t, bb, req, cards[:]...)
	assertHeldDeliveries(t, f, card, req.Key, otherKey)
}

func TestOutcomeHeldEntriesRecoverRoutingAfterLostReply(t *testing.T) {
	bb, f, req, card := heldOutcomeWithInterruptedRouting(t)
	otherKey := addHeldEntryForSameCard(t, f, req.Key)
	loseHeldRoutingReply(t, bb, f, req)

	var cards [2]CardResult
	for i, key := range []string{req.Key, otherKey} {
		result, _, err := f.releaseOne(t.Context(), key)
		if err != nil || !result.Replay || result.ID != card.ID || result.Key != req.Key {
			t.Fatalf("expected held entry %d to recover the existing card, got %+v, %v", i+1, result, err)
		}
		cards[i] = result
	}
	assertOneHeldRoutingComment(t, bb, req, cards[:]...)
	assertHeldDeliveries(t, f, card, req.Key, otherKey)
}

func TestOutcomeHeldRoutingLostReplyRecoversAfterRestart(t *testing.T) {
	bb, f, req, card := heldOutcomeWithInterruptedRouting(t)
	loseHeldRoutingReply(t, bb, f, req)

	// Start from configuration and the persisted directories, with the same
	// fake remote card and none of the previous filer's runtime hooks.
	restarted := &CardFiler{StateDir: f.StateDir, LockDir: f.LockDir, Run: bb.run, Now: filingTestNow}
	report, err := restarted.ReleaseDetailed(t.Context())
	delivered := report.Groups["other"]
	if err != nil || len(delivered) != 1 || !delivered[0].Replay || delivered[0].ID != card.ID || delivered[0].Key != req.Key {
		t.Fatalf("expected the persisted hold to recover the existing card after restart, got %+v, %v", report, err)
	}
	assertOneHeldRoutingComment(t, bb, req, delivered...)
	assertHeldDeliveries(t, restarted, card, req.Key)
}

func loseHeldRoutingReply(t *testing.T, bb *memBB, f *CardFiler, req CardRequest) {
	t.Helper()
	ctx := t.Context()
	run := f.Run
	defer func() { f.Run = run }()
	f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		result := run(ctx, env, args...)
		if len(args) >= 2 && args[0] == "tasks" && args[1] == "comment" && !result.failed() {
			return BBResult{Err: errors.New("fixture reply lost")}
		}
		return result
	}
	_, _, err := f.releaseOne(ctx, req.Key)
	if !errors.Is(err, ErrOutcomeUnknown) || bb.count("tasks", "comment") != 1 ||
		len(bb.tasks[0].Comments) != 1 || AskingThread(bb.tasks[0].Comments) != req.Thread {
		t.Fatalf("expected one accepted comment and an uncertain reply, got %v, calls %v", err, bb.calls)
	}
}

func addHeldEntryForSameCard(t *testing.T, f *CardFiler, requestKey string) string {
	t.Helper()
	state, path, unlock, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	// The second delivery was attempted under a different filing key, with
	// its creation using the existing card's canonical Request key.
	otherKey := "fixture-other"
	entry := state[requestKey]
	entry.Request.Key = otherKey
	entry.CreationKey = requestKey
	state[otherKey] = entry
	if err := saveFilingState(path, state); err != nil {
		t.Fatal(err)
	}
	return otherKey
}

func assertHeldDeliveries(t *testing.T, f *CardFiler, card CardResult, keys ...string) {
	t.Helper()
	state, _, unlock, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if len(state) != len(keys) {
		t.Fatalf("expected %d delivery records, got %d", len(keys), len(state))
	}
	for _, key := range keys {
		entry := state[key]
		if entry.Held || entry.DeliveredCard != card.ID || (key != card.Key && entry.DeliveredKey != card.Key) {
			t.Fatalf("held entry %s did not record the delivery: %+v", key, entry)
		}
	}
}

func heldOutcomeWithInterruptedRouting(t *testing.T) (*memBB, *CardFiler, CardRequest, CardResult) {
	t.Helper()
	ctx := t.Context()
	bb, f, req, capability := outcomeFilingFixture(t)
	req.NonBlocking = true
	req.Ask["review_by"] = "2026-01-01T00:00:00Z"
	if result, err := f.FileCard(ctx, req); err != nil || !result.Queued {
		t.Fatalf("expected a held request, got %+v, %v", result, err)
	}
	restore := failFixtureRouting(bb)
	card, _, err := f.releaseOne(ctx, req.Key)
	restore()
	if !errors.Is(err, ErrOutcomeUnknown) || card.ID == "" || len(bb.tasks) != 1 || len(bb.tasks[0].Comments) != 0 {
		t.Fatalf("expected a held card with interrupted routing, got %+v, %v", card, err)
	}
	*capability = BBResult{Stdout: []byte(`{"schema":1}`)}
	// Keep only the calls made during recovery.
	bb.calls = nil
	return bb, f, req, card
}

func assertOneHeldRoutingComment(t *testing.T, bb *memBB, req CardRequest, cards ...CardResult) {
	t.Helper()
	desc, _, err := req.Description()
	if err != nil {
		t.Fatal(err)
	}
	if bb.count("tasks", "comment") != 1 ||
		len(bb.tasks) != 1 || len(bb.tasks[0].Comments) != 1 || AskingThread(bb.tasks[0].Comments) != req.Thread ||
		bb.count("tasks", "create") != 0 || bb.count("tasks", "update") != 0 || bb.tasks[0].Desc != desc ||
		bb.count("home", "capabilities") != 0 {
		t.Fatalf("expected one comment on the unchanged card, got %+v, calls %v", cards, bb.calls)
	}
}

func runOverlappingHeldReleases(t *testing.T, f *CardFiler, firstKey, secondKey, waitedPath string) [2]CardResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	type attempt struct {
		card CardResult
		err  error
	}
	comments := make(chan struct{}, 2)
	finishComment := make(chan struct{})
	resume := sync.OnceFunc(func() { close(finishComment) })
	var workers []<-chan attempt
	defer func() {
		resume()
		// A worker may be stuck in flock, which cannot be canceled by ctx.
		for _, done := range workers {
			select {
			case <-done:
			case <-ctx.Done():
				return
			}
		}
	}()
	contention := make(chan error, 2)
	reportContention := func(err error) {
		select {
		case contention <- err:
		case <-ctx.Done():
		}
	}
	first := *f
	first.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if len(args) >= 2 && args[0] == "tasks" && args[1] == "comment" {
			select {
			case comments <- struct{}{}:
			case <-ctx.Done():
				return BBResult{Err: ctx.Err()}
			}
			select {
			case <-finishComment:
			case <-ctx.Done():
				return BBResult{Err: ctx.Err()}
			}
		}
		if err := ctx.Err(); err != nil {
			return BBResult{Err: err}
		}
		return f.Run(ctx, env, args...)
	}
	second := first
	secondDeliveryPath := filepath.Join(f.StateDir, "delivery-"+secondKey+".lock")
	second.testBeforeReleaseLock = func(path string) {
		switch path {
		case waitedPath:
			reportContention(requireBusyLock(path))
		case secondDeliveryPath:
			// Distinct entries own separate delivery locks before meeting at the Request lock.
		default:
			reportContention(fmt.Errorf("reached %s first", filepath.Base(path)))
		}
	}
	start := func(filer *CardFiler, key string) <-chan attempt {
		done := make(chan attempt, 1)
		workers = append(workers, done)
		go func() {
			defer close(done)
			card, _, err := filer.releaseOne(ctx, key)
			// This worker sends once to its own one-slot channel.
			done <- attempt{card, err}
		}()
		return done
	}
	firstDone := start(&first, firstKey)
	select {
	case <-comments:
	case result := <-firstDone:
		t.Fatalf("first release finished before routing: %+v", result)
	case <-ctx.Done():
		t.Fatalf("first release did not reach routing: %v", ctx.Err())
	}

	secondDone := start(&second, secondKey)
	lockName := filepath.Base(waitedPath)
	select {
	case err := <-contention:
		if err != nil {
			t.Fatalf("second release did not contend on %s: %v", lockName, err)
		}
	case <-comments:
		t.Fatalf("second release reached routing before contending on %s", lockName)
	case result := <-secondDone:
		t.Fatalf("second release finished before contending on %s: %+v", lockName, result)
	case <-ctx.Done():
		t.Fatalf("second release did not reach %s", lockName)
	}
	resume()

	var cards [2]CardResult
	for i, done := range []<-chan attempt{firstDone, secondDone} {
		select {
		case result, ok := <-done:
			if !ok {
				t.Fatalf("release %d did not finish before %v", i+1, ctx.Err())
			}
			if result.err != nil {
				t.Fatalf("release %d: %v", i+1, result.err)
			}
			cards[i] = result.card
		case <-ctx.Done():
			t.Fatalf("release %d did not finish", i+1)
		}
	}
	return cards
}

// A lock-attempt signal alone does not prove contention. The first release
// stays parked until the second proves the kernel would make it wait.
func requireBusyLock(path string) error {
	fd, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer fd.Close()
	err = syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return errors.New("lock was immediately available")
	}
	if !errors.Is(err, syscall.EWOULDBLOCK) {
		return err
	}
	return nil
}
