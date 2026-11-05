//go:build unix

package homerun

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Review round 2 of the verifier core (mk-21nfo): one group of tests per finding, all in recovery.

var inv32 = strings.Repeat("e", 32)

func noInvAnswer(context.Context, string) (EndInfo, bool, error) { return EndInfo{}, false, nil }

// startedCheck consumes a check on card, records its launch intent and STARTED, and returns the attempt.
func startedCheck(t *testing.T, s *Store, now time.Time, card string) *Attempt {
	t.Helper()
	a, _, _, err := submit(t, s, now, card, OpCheck, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := recStarted(s, bg, a.ID, StartInfo{InvocationID: inv32, CASSHA: a.ScriptSHA256}, now); err != nil {
		t.Fatal(err)
	}
	return a
}

func lastKind(t *testing.T, s *Store, attempt string) Kind {
	t.Helper()
	recs, err := s.Records(bg)
	if err != nil {
		t.Fatal(err)
	}
	var k Kind
	for _, r := range recs {
		if r.Attempt == attempt {
			k = r.Kind
		}
	}
	return k
}

// ---- finding 1: an unknown outcome never frees capacity ------------------------------------------

// A STARTED unit that the InvocationID lookup cannot answer for, but that the unit-name lookup still
// reports running, stays STARTED and keeps the card.
func TestStartedUnitStillRunningStaysHeld(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	a := startedCheck(t, s, t0, "AUT-1")
	running := func(_ context.Context, u string) (UnitState, error) {
		if u != UnitName(a.ID) {
			t.Errorf("asked about %q, want %q", u, UnitName(a.ID))
		}
		return UnitState{Found: true, InvocationID: inv32}, nil
	}
	if _, err := s.RecoverUnits(bg, noInvAnswer, running, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if k := lastKind(t, s, a.ID); k != KindStarted {
		t.Fatalf("a unit still running was recorded as %s", k)
	}
	if _, _, _, err := submit(t, s, t0.Add(2*time.Minute), "AUT-1", OpCheck, "", nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("card released while the unit runs: %v", err)
	}
}

// The unit-name lookup also settles a STARTED attempt whose InvocationID lookup has nothing.
func TestStartedPathConsultsUnitNameLookup(t *testing.T) {
	t0 := time.Unix(1_800_000_000, 0)
	end := okEnd()
	end.EndedAt = t0.Add(30 * time.Second)
	for _, c := range []struct {
		name  string
		state UnitState
		err   error
		want  Kind
	}{
		{"ended under the recorded invocation", UnitState{Found: true, InvocationID: inv32, Ended: &end}, nil, KindEnded},
		{"another invocation of the unit name", UnitState{Found: true, InvocationID: strings.Repeat("a", 32), Ended: &end}, nil, KindUncertain},
		{"unit gone", UnitState{}, nil, KindUncertain},
		{"lookup error", UnitState{}, errors.New("dbus down"), KindUncertain},
		{"garbled end info", UnitState{Found: true, InvocationID: inv32, Ended: &EndInfo{Result: ResultSuccess, ExitStatus: 9}}, nil, KindUncertain},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newStore(t)
			a := startedCheck(t, s, t0, "AUT-1")
			byU := func(context.Context, string) (UnitState, error) { return c.state, c.err }
			if _, err := s.RecoverUnits(bg, noInvAnswer, byU, t0.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			if k := lastKind(t, s, a.ID); k != c.want {
				t.Fatalf("attempt is %s, want %s", k, c.want)
			}
		})
	}
}

// UNCERTAIN holds the card and the privilege's cap; only an operator resolution frees them.
func TestUncertainHoldsCardAndPrivilegeUntilResolved(t *testing.T) {
	lim := DefaultLimits()
	lim.MaxActiveAsMk = 1
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), lim)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	mir := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := s.SetMirror(bg, mir); err != nil {
		t.Fatal(err)
	}
	t0 := time.Unix(1_800_000_000, 0)
	a := startedCheck(t, s, t0, "AUT-1")
	if _, err := s.Recover(bg, noInvAnswer, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if k := lastKind(t, s, a.ID); k != KindUncertain {
		t.Fatalf("attempt is %s, want UNCERTAIN", k)
	}
	later := t0.Add(2 * time.Minute)
	if _, _, _, err := submit(t, s, later, "AUT-1", OpCheck, "", nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("same card accepted while an attempt is UNCERTAIN: %v", err)
	}
	if _, _, _, err := submit(t, s, later, "AUT-2", OpCheck, "", nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("privilege cap freed by an UNCERTAIN attempt: %v", err)
	}
	// Nothing automatic: later recoveries, even with answers that look conclusive, leave it alone.
	end := okEnd()
	end.EndedAt = t0.Add(time.Second)
	for _, byU := range []UnitLookup{
		nil,
		func(context.Context, string) (UnitState, error) { return UnitState{}, nil },
		func(context.Context, string) (UnitState, error) {
			return UnitState{Found: true, InvocationID: inv32, Ended: &end}, nil
		},
	} {
		if _, err := s.RecoverUnits(bg, noInvAnswer, byU, later); err != nil {
			t.Fatal(err)
		}
		if k := lastKind(t, s, a.ID); k != KindUncertain {
			t.Fatalf("recovery moved an UNCERTAIN attempt to %s", k)
		}
	}
	// A settled-state check still passes: UNCERTAIN is settled for the audit, but not free for capacity.
	if err := s.CheckAuditChain(bg, true); err != nil {
		t.Fatal(err)
	}

	res := Resolution{Operator: "mk", Outcome: ResolutionTerminated, Reason: "unit gone after the host reboot; output reviewed"}
	for name, bad := range map[string]Resolution{
		"no operator":     {Outcome: ResolutionTerminated, Reason: "x"},
		"no reason":       {Operator: "mk", Outcome: ResolutionTerminated},
		"unknown outcome": {Operator: "mk", Outcome: "probably", Reason: "x"},
		"control char":    {Operator: "mk\nroot", Outcome: ResolutionTerminated, Reason: "x"},
	} {
		if err := s.ResolveUncertain(bg, a.ID, bad, later); err == nil {
			t.Errorf("%s: resolution accepted", name)
		}
	}
	if err := s.ResolveUncertain(bg, "01ARZ3NDEKTSV4RRFFQ69G5FAV", res, later); !errors.Is(err, ErrBadTransition) {
		t.Errorf("unknown attempt: %v", err)
	}
	if err := s.ResolveUncertain(bg, a.ID, res, later); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveUncertain(bg, a.ID, res, later); !errors.Is(err, ErrBadTransition) {
		t.Errorf("a resolved attempt resolved again: %v", err)
	}
	recs, _ := s.Records(bg)
	last := recs[len(recs)-1]
	if last.Kind != KindResolved || last.Operator != "mk" || last.Resolution != ResolutionTerminated || !strings.Contains(last.Note, "host reboot") || last.Nonce == "" {
		t.Fatalf("resolution audit record: %+v", last)
	}
	if got := kindsOf(recs, a.ID); got != "AUTHORIZED STARTED UNCERTAIN RESOLVED" {
		t.Fatalf("audit sequence %q", got)
	}
	mrecs, err := ReadMirror(mir)
	if err != nil || len(mrecs) != len(recs) || mrecs[len(mrecs)-1].Operator != "mk" {
		t.Fatalf("mirror %v %v", err, mrecs)
	}
	if err := s.CheckAuditChain(bg, true); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := submit(t, s, later, "AUT-1", OpCheck, "", nil); err != nil {
		t.Fatalf("card still held after resolution: %v", err)
	}
}

// Only an UNCERTAIN attempt can be resolved.
func TestResolveRefusedUnlessUncertain(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	res := Resolution{Operator: "mk", Outcome: ResolutionTerminated, Reason: "checked"}
	a, _, _, _ := submit(t, s, t0, "AUT-1", OpCheck, "", nil)
	if err := s.ResolveUncertain(bg, a.ID, res, t0); !errors.Is(err, ErrBadTransition) {
		t.Errorf("AUTHORIZED resolved: %v", err)
	}
	if err := recStarted(s, bg, a.ID, StartInfo{InvocationID: inv32, CASSHA: a.ScriptSHA256}, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveUncertain(bg, a.ID, res, t0); !errors.Is(err, ErrBadTransition) {
		t.Errorf("STARTED resolved: %v", err)
	}
}

// ---- finding 2: recovery never invents a completion time ------------------------------------------

func TestRecoveryKeepsRealCompletionTime(t *testing.T) {
	t0 := time.Unix(1_800_000_000, 0)
	setup := func(t *testing.T, endedAt time.Time) (*Store, *Attempt) {
		s, _ := newStore(t)
		a := startedCheck(t, s, t0, "AUT-1")
		end := okEnd()
		end.EndedAt = endedAt
		byInv := func(context.Context, string) (EndInfo, bool, error) { return end, true, nil }
		recAt := t0.Add(2 * time.Hour)
		if _, err := s.Recover(bg, byInv, recAt); err != nil {
			t.Fatal(err)
		}
		return s, a
	}
	t.Run("check finished two hours before recovery", func(t *testing.T) {
		s, a := setup(t, t0.Add(time.Minute))
		if k := lastKind(t, s, a.ID); k != KindEnded {
			t.Fatalf("attempt is %s", k)
		}
		_, _, nonce, err := submit(t, s, t0.Add(2*time.Hour), "AUT-1", OpApply, a.ID, nil)
		if !errors.Is(err, ErrCheckReceipt) || nonceSpent(t, s, nonce) {
			t.Fatalf("apply on a two-hour-old check: %v", err)
		}
		recs, _ := s.Records(bg)
		if got := recs[len(recs)-1].EndedAt; got != t0.Add(time.Minute).UTC().Format(time.RFC3339Nano) {
			t.Errorf("ENDED record carries ended_at %q", got)
		}
	})
	t.Run("recovered late but the check itself is still fresh", func(t *testing.T) {
		s, _ := newStore(t)
		a := startedCheck(t, s, t0, "AUT-1")
		end := okEnd()
		end.EndedAt = t0.Add(time.Minute)
		byInv := func(context.Context, string) (EndInfo, bool, error) { return end, true, nil }
		if _, err := s.Recover(bg, byInv, t0.Add(20*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := submit(t, s, t0.Add(25*time.Minute), "AUT-1", OpApply, a.ID, nil); err != nil {
			t.Fatalf("a check inside its 30 minutes: %v", err)
		}
	})
	t.Run("completion time unknown", func(t *testing.T) {
		s, a := setup(t, time.Time{})
		if k := lastKind(t, s, a.ID); k != KindEnded {
			t.Fatalf("attempt is %s", k)
		}
		_, _, nonce, err := submit(t, s, t0.Add(2*time.Hour), "AUT-1", OpApply, a.ID, nil)
		if !errors.Is(err, ErrCheckReceipt) || !strings.Contains(err.Error(), "unknown") || nonceSpent(t, s, nonce) {
			t.Fatalf("apply on a check of unknown age: %v", err)
		}
		// not even right after the recovery
		s2, a2 := func() (*Store, *Attempt) {
			s, _ := newStore(t)
			a := startedCheck(t, s, t0, "AUT-1")
			end := okEnd()
			if _, err := s.Recover(bg, func(context.Context, string) (EndInfo, bool, error) { return end, true, nil }, t0.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			return s, a
		}()
		if _, _, _, err := submit(t, s2, t0.Add(2*time.Minute), "AUT-1", OpApply, a2.ID, nil); !errors.Is(err, ErrCheckReceipt) {
			t.Fatalf("unknown completion time accepted as fresh: %v", err)
		}
	})
	t.Run("implausible completion time", func(t *testing.T) {
		for name, at := range map[string]time.Time{
			"after the recovery":         t0.Add(3 * time.Hour),
			"before the attempt existed": t0.Add(-time.Hour),
		} {
			s, a := setup(t, at)
			if k := lastKind(t, s, a.ID); k != KindUncertain {
				t.Errorf("%s: attempt is %s, want UNCERTAIN", name, k)
			}
		}
	})
	t.Run("live RecordEnded checks the time too", func(t *testing.T) {
		s, _ := newStore(t)
		a := startedCheck(t, s, t0, "AUT-1")
		end := okEnd()
		end.EndedAt = t0.Add(time.Hour)
		if err := s.RecordEnded(bg, a.ID, end, t0.Add(time.Minute)); err == nil {
			t.Fatal("a completion time after the recording time was accepted")
		}
		end.EndedAt = t0.Add(-time.Second)
		if err := s.RecordEnded(bg, a.ID, end, t0.Add(time.Minute)); err == nil {
			t.Fatal("a completion time before the attempt was accepted")
		}
		end.EndedAt = t0.Add(30 * time.Second)
		if err := s.RecordEnded(bg, a.ID, end, t0.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	})
}

// ---- finding 3: a launch intent plus a missing unit is not proof the script never ran -----------

func TestAbsentUnitAfterIntentIsUncertainAndStillCounted(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	chk := goodCheck(t, s, t0, "AUT-1")
	a, _, _, err := submit(t, s, t0, "AUT-1", OpApply, chk, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordLaunchIntent(bg, a.ID, t0); err != nil {
		t.Fatal(err)
	}
	// the verifier dies after launch and before STARTED; the unit ran and was collected
	gone := func(context.Context, string) (UnitState, error) { return UnitState{}, nil }
	if _, err := s.RecoverUnits(bg, noInvAnswer, gone, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if k := lastKind(t, s, a.ID); k != KindUncertain {
		t.Fatalf("attempt is %s, want UNCERTAIN", k)
	}
	recs, _ := s.Records(bg)
	if n := recs[len(recs)-1].Note; !strings.Contains(n, "check by hand") {
		t.Errorf("UNCERTAIN lacks the by-hand note: %q", n)
	}
	// the card stays reserved
	if _, _, _, err := submit(t, s, t0.Add(2*time.Minute), "AUT-1", OpCheck, "", nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("card released: %v", err)
	}
	// and after an operator resolves it, the execution still counts toward the apply interval
	if err := s.ResolveUncertain(bg, a.ID, Resolution{Operator: "mk", Outcome: ResolutionNeverRan, Reason: "unit logs show no start"}, t0.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	chk2 := goodCheck(t, s, t0.Add(4*time.Minute), "AUT-1")
	var rl *RateLimitError
	if _, _, _, err := submit(t, s, t0.Add(5*time.Minute), "AUT-1", OpApply, chk2, nil); !errors.As(err, &rl) || rl.Limit != "apply-interval" {
		t.Fatalf("apply interval forgot the possible execution: %v", err)
	}
}

func TestAbsentUnitAfterIntentCountsTowardDailyCap(t *testing.T) {
	lim := DefaultLimits()
	lim.MaxExecutionsPerDay = 1
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), lim)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	t0 := time.Unix(1_800_000_000, 0)
	a, _, _, _ := submit(t, s, t0, "AUT-1", OpCheck, "", nil)
	if _, err := s.RecordLaunchIntent(bg, a.ID, t0); err != nil {
		t.Fatal(err)
	}
	gone := func(context.Context, string) (UnitState, error) { return UnitState{}, nil }
	if _, err := s.RecoverUnits(bg, noInvAnswer, gone, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveUncertain(bg, a.ID, Resolution{Operator: "mk", Outcome: ResolutionNeverRan, Reason: "r"}, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := submit(t, s, t0.Add(2*time.Minute), "AUT-2", OpCheck, "", nil); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("daily cap forgot the possible execution: %v", err)
	}
}

// An attempt with no launch intent is the only one that can be ABORTED_BEFORE_START, and recovery does
// not ask systemd about it.
func TestAbortedOnlyWithoutLaunchIntent(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Unix(1_800_000_000, 0)
	a, _, _, _ := submit(t, s, t0, "AUT-1", OpCheck, "", nil)
	asked := false
	byU := func(context.Context, string) (UnitState, error) { asked = true; return UnitState{}, nil }
	if _, err := s.RecoverUnits(bg, noInvAnswer, byU, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if asked || lastKind(t, s, a.ID) != KindAborted {
		t.Fatalf("asked=%v kind=%s", asked, lastKind(t, s, a.ID))
	}
}
