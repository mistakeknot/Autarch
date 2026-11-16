package homeask

import (
	"errors"
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
