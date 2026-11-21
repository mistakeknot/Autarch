package homeask

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestAlreadyRuledNeedsNoWriteProof(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{{ErrAlreadyRuled, false}, {errors.Join(errNoCardWritten, ErrAlreadyRuled), true}, {errors.Join(errNoCardWritten, ErrAlreadyRuled, os.ErrPermission), false}, {errors.Join(errNoCardWritten, ErrAlreadyRuled, ErrOutcomeUnknown), false}} {
		if got := (&ReleaseEntryErrors{Errors: []error{tc.err}}).DataOnly(); got != tc.want {
			t.Fatalf("%v: data=%v want %v", tc.err, got, tc.want)
		}
	}
}

func TestAlreadyRuledProofStaysInItsJoinedBranch(t *testing.T) {
	provedInvalid := releaseUpdateEntryError(t, "closed")
	provedRuled := releaseUpdateEntryError(t, "ruled")
	race := releaseUpdateEntryError(t, "race")
	resolveFailure := releaseUpdateEntryError(t, "resolve-failure")
	nilResolveOperational := releaseUpdateEntryError(t, "nil-resolve-operational")
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"real ruled release", provedRuled, true},
		{"real invalid release", provedInvalid, true},
		{"real post-write race", race, false},
		{"real resolve failure", resolveFailure, false},
		{"operational cause with nil resolver error", nilResolveOperational, false},
		{"unrelated proof", errors.Join(race, provedInvalid), false},
		{"wrapped unrelated proof", fmt.Errorf("request: %w", errors.Join(provedInvalid, race)), false},
		{"proved and unproved ruled outcomes", errors.Join(provedRuled, race), false},
		{"outer proof does not cover nested join", errors.Join(errNoCardWritten, errors.Join(ErrInvalid, race)), false},
		{"wrapped siblings", errors.Join(fmt.Errorf("proof: %w", errNoCardWritten), race), true},
		{"wrapped pair", fmt.Errorf("request: %w", provedRuled), true},
		{"independently proved branches", errors.Join(provedInvalid, fmt.Errorf("request: %w", provedRuled)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := (&ReleaseEntryErrors{Errors: []error{tc.err}}).DataOnly()
			t.Logf("DataOnly=%t", got)
			if got != tc.want {
				t.Fatalf("joined branches classified data-only=%v, want %v: %v", got, tc.want, tc.err)
			}
		})
	}
}

// releaseUpdateEntryError obtains the actual update -> releaseOne join with
// resolveErr -> ReleaseDetailed request wrapper, using only a fake BB runner.
func releaseUpdateEntryError(t *testing.T, outcome string) error {
	t.Helper()
	bb, f, cardState := updateRig(t)
	f.StateDir = t.TempDir()
	r := testReq()
	r.Key = "original"
	r.Ask["ask_key"] = "original-ask"
	first, err := f.FileOrUpdateCard(context.Background(), r, "")
	if err != nil {
		t.Fatal(err)
	}
	held := testReq()
	held.Key, held.NonBlocking = "held-update", true
	held.Ask["ask_key"] = "replacement-ask"
	if result, err := f.FileOrUpdateCard(context.Background(), held, first.ID); err != nil || !result.Queued {
		t.Fatalf("could not queue update: %+v %v", result, err)
	}
	switch outcome {
	case "closed":
		*cardState = "closed"
	case "ruled", "resolve-failure":
		*cardState = "ruled"
	case "race", "nil-resolve-operational":
		// Leave Home open until the fake Tasks update runs below.
	default:
		t.Fatalf("unknown release outcome %q", outcome)
	}
	if outcome == "nil-resolve-operational" {
		f.Check = func(string, string) error { return errors.Join(ErrInvalid, ErrHomeDown) }
	}
	resolveLookups := 0
	originalFail := bb.fail
	bb.fail = func(args []string) *BBResult {
		if outcome == "nil-resolve-operational" && strings.Join(args[:min(len(args), 3)], " ") == "home binding P1" {
			result := okRes(map[string]any{"home_project": "autarch", "state": "confirmed"})
			return &result
		}
		if len(args) > 3 && args[0] == "home" && args[1] == "get" && args[2] == "--request" && args[3] == held.Key {
			resolveLookups++
			if outcome == "resolve-failure" {
				return badRes(1, "registry unavailable")
			}
		}
		if outcome == "race" && len(args) > 1 && args[0] == "tasks" && args[1] == "update" {
			*cardState = "ruled"
		}
		return originalFail(args)
	}
	report, err := f.ReleaseDetailed(context.Background())
	var entries *ReleaseEntryErrors
	if !errors.As(err, &entries) || len(entries.Errors) != 1 || report.Failures[held.Key] == "" || len(report.Groups) != 0 {
		t.Fatalf("missing actual release entry error: %+v %v", report, err)
	}
	entryErr := entries.Errors[0]
	if !strings.HasPrefix(entryErr.Error(), "request "+held.Key+": ") {
		t.Fatalf("missing ReleaseDetailed request wrapper: %v", entryErr)
	}
	if outcome == "race" {
		if !errors.Is(entryErr, ErrAlreadyRuled) || errors.Is(entryErr, errNoCardWritten) || bb.count("tasks", "update") != 1 || resolveLookups != 0 {
			t.Fatalf("fixture did not reach the post-write ruled race: %v; resolve lookups=%d", entryErr, resolveLookups)
		}
		return entryErr
	}
	wantCause := ErrAlreadyRuled
	if outcome == "closed" || outcome == "nil-resolve-operational" {
		wantCause = ErrInvalid
	}
	if !errors.Is(entryErr, wantCause) || !errors.Is(entryErr, errNoCardWritten) || bb.count("tasks", "update") != 0 || resolveLookups == 0 {
		t.Fatalf("fixture did not reach refusal and terminal resolution: %v; resolve lookups=%d", entryErr, resolveLookups)
	}
	if outcome == "resolve-failure" || outcome == "nil-resolve-operational" {
		// Normalize unary joins retained by some Go versions when resolveErr is
		// nil, then check that the operational sibling never gains write proof.

		node := errors.Unwrap(entryErr)
		for {
			single, ok := node.(interface{ Unwrap() []error })
			if !ok || len(single.Unwrap()) != 1 {
				break
			}
			node = single.Unwrap()[0]
		}
		joined, ok := node.(interface{ Unwrap() []error })
		if !ok {
			t.Fatalf("missing join with terminal lookup failure: %v", entryErr)
		}
		refusal, lookupFailure := false, false
		proofSibling := false
		for _, cause := range joined.Unwrap() {
			proofSibling = proofSibling || errors.Is(cause, errNoCardWritten)
			if outcome == "nil-resolve-operational" {
				refusal = refusal || errors.Is(cause, wantCause)
			} else {
				refusal = refusal || (errors.Is(cause, errNoCardWritten) && errors.Is(cause, wantCause))
			}
			if errors.Is(cause, ErrHomeDown) {
				if errors.Is(cause, errNoCardWritten) {
					t.Fatalf("operational resolveErr branch contains no-write proof: %v", cause)
				}
				lookupFailure = true
			}
		}
		if !refusal || !lookupFailure || !proofSibling {
			t.Fatalf("missing no-write refusal or operational resolveErr branch: %v", entryErr)
		}
	}
	return entryErr
}
