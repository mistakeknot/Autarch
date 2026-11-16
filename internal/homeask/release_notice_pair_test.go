package homeask

import (
	"errors"
	"fmt"
	"os"
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
	provedInvalid := errors.Join(errNoCardWritten, ErrInvalid)
	provedRuled := errors.Join(errNoCardWritten, ErrAlreadyRuled)
	race := fmt.Errorf("update race: %w", ErrAlreadyRuled)
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"unrelated proof", errors.Join(race, provedInvalid), false},
		{"wrapped unrelated proof", fmt.Errorf("request: %w", errors.Join(provedInvalid, race)), false},
		{"proved and unproved ruled outcomes", errors.Join(provedRuled, race), false},
		{"outer proof does not cover nested join", errors.Join(errNoCardWritten, errors.Join(ErrInvalid, race)), false},
		{"wrapped siblings", errors.Join(fmt.Errorf("proof: %w", errNoCardWritten), race), true},
		{"wrapped pair", fmt.Errorf("request: %w", provedRuled), true},
		{"independently proved branches", errors.Join(provedInvalid, fmt.Errorf("request: %w", provedRuled)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (&ReleaseEntryErrors{Errors: []error{tc.err}}).DataOnly(); got != tc.want {
				t.Fatalf("joined branches classified data-only=%v, want %v: %v", got, tc.want, tc.err)
			}
		})
	}
}
