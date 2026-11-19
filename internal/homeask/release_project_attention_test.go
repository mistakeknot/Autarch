package homeask

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMissingProjectAttentionForUnownedPull(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.StateDir = t.TempDir()
	r := testReq()
	r.Key = "unowned"
	r.Pull, r.NonBlocking = true, true
	r.Owner, r.Thread = "", ""
	if _, err := f.FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	// A prior failed label lookup left this reservation behind.
	state, path, done, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state[r.Key]
	entry.Attempted = true
	entry.OwnerAttention = "owner unowned request needs attention: project no longer exists in bb: \"P1\"; run needs-mk cancel-held unowned, then file the corrected ask against a live project"
	state[r.Key] = entry
	err = saveFilingState(path, state)
	done()
	if err != nil {
		t.Fatal(err)
	}
	f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if len(args) >= 3 && strings.Join(args[:3], " ") == "tasks label list" {
			return BBResult{Code: 1, Stderr: []byte("project not found: P1")}
		}
		return bb.run(ctx, env, args...)
	}
	report, err := f.ReleaseDetailed(context.Background())
	var entries *ReleaseEntryErrors
	if !errors.As(err, &entries) || !entries.DataOnly() {
		t.Fatalf("expected request fault: %v", err)
	}
	attention := report.OwnerAttention[r.Key]
	if attention != entry.OwnerAttention {
		t.Fatalf("fixture attention differed: before=%q after=%q", entry.OwnerAttention, attention)
	}
	if !strings.Contains(attention, "owner unowned request needs attention") || !strings.Contains(attention, "run needs-mk cancel-held "+r.Key) {
		t.Fatalf("unclear attention: %q", attention)
	}
	var footer bytes.Buffer
	if err := f.PrintReleaseOwnerAttention(&footer); err != nil || !strings.Contains(footer.String(), attention) {
		t.Fatalf("footer=%s err=%v", &footer, err)
	}
	if err := f.CancelHeld(r.Key); err != nil {
		t.Fatalf("advertised cancellation failed: %v", err)
	}
	state, _, done, err = f.openState()
	if err != nil {
		t.Fatal(err)
	}
	done()
	if _, ok := state[r.Key]; ok {
		t.Fatal("cancelled entry remains")
	}
}

func TestMissingProjectRetainsPossibleCreation(t *testing.T) {
	for _, kind := range []string{"creation", "update"} {
		t.Run(kind, func(t *testing.T) { testMissingProjectReservation(t, kind) })
	}
}

func testMissingProjectReservation(t *testing.T, kind string) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.StateDir = t.TempDir()
	r := testReq()
	r.Key, r.NonBlocking = "uncertain", true
	if _, err := f.FileCard(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	state, path, done, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state[r.Key]
	entry.Attempted = true
	if kind == "update" {
		entry.Update = true
	} else {
		entry.CreationKey = "prior-creation"
	}
	state[r.Key] = entry
	err = saveFilingState(path, state)
	done()
	if err != nil {
		t.Fatal(err)
	}
	f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
		if len(args) >= 3 && strings.Join(args[:3], " ") == "tasks label list" {
			return BBResult{Code: 1, Stderr: []byte("project not found: P1")}
		}
		return bb.run(ctx, env, args...)
	}
	report, err := f.ReleaseDetailed(context.Background())
	if err == nil || !strings.Contains(report.OwnerAttention[r.Key], "reconcile the prior delivery") {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if err := f.CancelHeld(r.Key); err == nil {
		t.Fatal("possible creation was cancelled")
	}
	state, _, done, err = f.openState()
	if err != nil {
		t.Fatal(err)
	}
	done()
	if !state[r.Key].Attempted || state[r.Key].Update != entry.Update || state[r.Key].CreationKey != entry.CreationKey {
		t.Fatal("lost creation reservation")
	}
}

func TestProjectAttentionRechecksFreshReservation(t *testing.T) {
	for _, field := range []string{"update", "creation", "card", "delivered", "pending"} {
		t.Run(field, func(t *testing.T) {
			f := newFake(t, registryOnCreate).filer()
			f.StateDir = t.TempDir()
			r := testReq()
			r.Key, r.NonBlocking = "fresh", true
			if _, err := f.FileCard(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			state, path, done, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			entry := state[r.Key]
			entry.Attempted = true
			switch field {
			case "update":
				entry.Update = true
			case "creation":
				entry.CreationKey = "prior-creation"
			case "card":
				entry.DeliveredCard = "prior-card"
			case "delivered":
				entry.DeliveredAt = "2026-01-01T00:00:00Z"
			case "pending":
				entry.DeliveryPendingAt = "2026-01-01T00:00:00Z"
			}
			state[r.Key] = entry
			err = saveFilingState(path, state)
			done()
			if err != nil {
				t.Fatal(err)
			}
			// The caller computed true before a writer added the marker above.
			if err := f.recordOwnerAttentionState(r.Key, "project needs attention", true); err != nil {
				t.Fatal(err)
			}
			state, _, done, err = f.openState()
			if err != nil {
				t.Fatal(err)
			}
			done()
			if !state[r.Key].Attempted {
				t.Fatal("stale proof cleared a fresh reservation")
			}
			if state[r.Key].OwnerAttention != "project needs attention" {
				t.Fatal("attention not recorded")
			}
		})
	}
}
