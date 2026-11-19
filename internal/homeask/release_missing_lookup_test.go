package homeask

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMissingProjectRequiresGlobalProof(t *testing.T) {
	for _, outcome := range []string{"found", "update-created", "delivered-key", "delivered-card", "outage", "timeout", "absent", "update-absent", "changed"} {
		t.Run(outcome, func(t *testing.T) {
			bb := newFake(t, registryAbsent)
			f := bb.filer()
			f.StateDir = t.TempDir()
			r := testReq()
			r.Key, r.NonBlocking = "legacy", true
			if _, err := f.FileCard(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			state, path, done, err := f.openState()
			if err != nil {
				t.Fatal(err)
			}
			e := state[r.Key]
			e.Attempted = true
			if outcome == "found" || outcome == "update-created" || outcome == "delivered-key" || outcome == "delivered-card" {
				e.DeliveryPendingAt = "2026-01-01T00:00:00Z"
			}
			if outcome == "delivered-key" {
				e.DeliveredKey = "recorded"
			}
			if outcome == "delivered-card" {
				e.DeliveredCard = "old-card"
			}
			e.Update = outcome == "update-absent" || outcome == "update-created"
			if outcome == "update-created" {
				e.CreationKey = "generated"
			}
			state[r.Key] = e
			err = saveFilingState(path, state)
			done()
			if err != nil {
				t.Fatal(err)
			}
			if outcome == "found" || outcome == "update-created" || outcome == "delivered-key" || outcome == "delivered-card" {
				cardRequest := r
				if outcome == "delivered-key" || outcome == "delivered-card" {
					cardRequest.Key = "recorded"
				}
				if outcome == "update-created" {
					cardRequest.Key = "generated"
				}
				desc, _, err := cardRequest.Description()
				if err != nil {
					t.Fatal(err)
				}
				bb.tasks = append(bb.tasks, &fakeTask{ID: "old-card", Project: "P2", Status: "open", Desc: desc})
			}
			lists, calls := 0, 0
			f.Run = func(ctx context.Context, env []string, args ...string) BBResult {
				calls++
				if len(args) >= 3 && strings.Join(args[:3], " ") == "tasks label list" {
					return BBResult{Code: 1, Stderr: []byte("project not found: P1")}
				}
				if len(args) >= 2 && strings.Join(args[:2], " ") == "tasks list" {
					lists++
					if outcome == "delivered-card" {
						return okRes(map[string]any{"tasks": []any{}, "nextCursor": nil})
					}
					if outcome == "changed" {
						state, path, done, err := f.openState()
						if err != nil {
							t.Fatal(err)
						}
						fresh := state[r.Key]
						fresh.CreationKey = "new-creation"
						state[r.Key] = fresh
						err = saveFilingState(path, state)
						done()
						if err != nil {
							t.Fatal(err)
						}
					}
					if argFlag(args, "--project") != "" {
						t.Fatal("scoped absence proof")
					}
					if outcome == "outage" {
						return BBResult{Code: 1, Stderr: []byte("connection refused")}
					}
					if outcome == "timeout" {
						return BBResult{TimedOut: true}
					}
				}
				return bb.run(ctx, env, args...)
			}
			report, releaseErr := f.ReleaseDetailed(context.Background())
			state, _, done, err = f.openState()
			if err != nil {
				t.Fatal(err)
			}
			done()
			if lists == 0 {
				t.Fatal("missing project cleared without global lookup")
			}
			switch outcome {
			case "found", "update-created", "delivered-key", "delivered-card":
				if state[r.Key].DeliveryPendingAt != "" {
					t.Fatal("resolved delivery kept its pending timestamp")
				}
				if state[r.Key].Held || state[r.Key].DeliveredCard != "old-card" || len(report.Groups["other"]) != 1 {
					t.Fatalf("lost moved card: %+v %+v %v", state[r.Key], report, releaseErr)
				}
			case "outage", "timeout":
				if !state[r.Key].Attempted || releaseErr == nil || (!errors.Is(releaseErr, ErrHomeDown) && !errors.Is(releaseErr, ErrOutcomeUnknown)) {
					t.Fatalf("lost uncertainty: %+v %v", state[r.Key], releaseErr)
				}
			case "changed":
				if !state[r.Key].Attempted || strings.Contains(report.OwnerAttention[r.Key], "cancel-held") {
					t.Fatal("stale proof admitted cancellation")
				}
			default:
				if state[r.Key].Attempted {
					t.Fatal("proven absence did not clear reservation")
				}
				statePath := filepath.Join(f.StateDir, "filings.json")
				stamp := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
				if err := os.Chtimes(statePath, stamp, stamp); err != nil {
					t.Fatal(err)
				}
				before, err := os.Stat(statePath)
				if err != nil {
					t.Fatal(err)
				}
				beforeCalls, beforeLists := calls, lists
				if _, err := f.ReleaseDetailed(context.Background()); err == nil {
					t.Fatal("missing project no longer reported")
				}
				after, err := os.Stat(statePath)
				if err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() {
					t.Fatal("unchanged retry rewrote state")
				}
				if calls-beforeCalls != 1 || lists != beforeLists {
					t.Fatalf("retry calls=%d lists=%d", calls-beforeCalls, lists-beforeLists)
				}
				if err := f.CancelHeld(r.Key); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestMissingProjectVanishedAdvice(t *testing.T) {
	f := newFake(t, registryAbsent).filer()
	f.StateDir = t.TempDir()
	attention, err := f.recordProjectOwnerAttention("gone", "thr_owner", ErrProjectNotFound)
	if err != nil || attention.remediation == "" {
		t.Fatalf("empty vanished advice: %v %v", attention, err)
	}
}
