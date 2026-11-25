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
	cards := runOverlappingHeldReleases(t, f, req.Key, req.Key, lockPath)
	if !cards[0].Replay || cards[0].ID != card.ID || cards[1].ID != "" {
		t.Fatalf("expected exactly one replay of the existing card, got %+v", cards)
	}
	assertOneHeldRoutingComment(t, bb, req, cards)
}

func TestOutcomeConcurrentHeldEntriesForSameCardRouteOnce(t *testing.T) {
	bb, f, req, card := heldOutcomeWithInterruptedRouting(t)
	// The second delivery was attempted under a different filing key, with
	// its creation using the existing card's canonical Request key.
	otherKey := "fixture-other"
	func() {
		state, path, unlock, err := f.openState()
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		entry := state[req.Key]
		entry.Request.Key = otherKey
		entry.CreationKey = req.Key
		state[otherKey] = entry
		if err := saveFilingState(path, state); err != nil {
			t.Fatal(err)
		}
	}()

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
	assertOneHeldRoutingComment(t, bb, req, cards)
	state, _, unlock, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if entry := state[otherKey]; entry.Held || entry.DeliveredCard != card.ID || entry.DeliveredKey != req.Key {
		t.Fatalf("second held entry did not record the shared card: %+v", entry)
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
	// Keep only the calls made by the overlapping releases.
	bb.calls = nil
	return bb, f, req, card
}

func assertOneHeldRoutingComment(t *testing.T, bb *memBB, req CardRequest, cards [2]CardResult) {
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
	comments := make(chan struct{}, 2)
	finishComment := make(chan struct{})
	resume := sync.OnceFunc(func() { close(finishComment) })
	var workers sync.WaitGroup
	defer func() {
		resume()
		workers.Wait()
	}()
	first := *f
	first.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if len(args) >= 2 && args[0] == "tasks" && args[1] == "comment" {
			comments <- struct{}{}
			<-finishComment
		}
		return f.Run(ctx, env, args...)
	}
	second := first
	contention := make(chan error, 2)
	secondDeliveryPath := filepath.Join(f.StateDir, "delivery-"+secondKey+".lock")
	second.testBeforeReleaseLock = func(path string) {
		switch path {
		case waitedPath:
			contention <- requireBusyLock(path)
		case secondDeliveryPath:
			// Distinct entries own separate delivery locks before meeting at the Request lock.
		default:
			contention <- fmt.Errorf("reached %s first", filepath.Base(path))
		}
	}
	type attempt struct {
		card CardResult
		err  error
	}
	start := func(filer *CardFiler, key string) <-chan attempt {
		done := make(chan attempt, 1)
		workers.Go(func() {
			card, _, err := filer.releaseOne(ctx, key)
			done <- attempt{card, err}
		})
		return done
	}
	firstDone := start(&first, firstKey)
	select {
	case <-comments:
	case result := <-firstDone:
		t.Fatalf("first release finished before routing: %+v", result)
	case <-ctx.Done():
		t.Fatal("first release did not reach routing")
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
		case result := <-done:
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
