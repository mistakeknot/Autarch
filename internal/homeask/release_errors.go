package homeask

import (
	"errors"
	"fmt"
)

// ReleaseEntryErrors retains per-request diagnostics separately from batch failures.
// Callers can present bad entries without failing an otherwise healthy release.
type ReleaseEntryErrors struct{ Errors []error }

func (e *ReleaseEntryErrors) Error() string   { return errors.Join(e.Errors...).Error() }
func (e *ReleaseEntryErrors) Unwrap() []error { return e.Errors }

type releaseOwnerError struct {
	owner       string
	cause       error
	remediation string
}

func (e *releaseOwnerError) Error() string {
	return fmt.Sprintf("owner %s needs attention: %v; %s", e.owner, e.cause, e.remediation)
}
func (e *releaseOwnerError) Unwrap() error { return e.cause }

// DataOnly suppresses only recognized request faults, never unknown or mixed operational errors.
func (e *ReleaseEntryErrors) DataOnly() bool {
	if len(e.Errors) == 0 {
		return false
	}
	for _, err := range e.Errors {
		allowed, data := releaseDataError(err)
		if !allowed || !data {
			return false
		}
	}
	return true
}
func releaseDataError(err error) (bool, bool) {
	if err == nil {
		return true, false
	}
	if err == ErrInvalid {
		return true, true
	}
	if err == errNoCardWritten {
		return true, false
	}
	if many, ok := err.(interface{ Unwrap() []error }); ok {
		data := false
		for _, child := range many.Unwrap() {
			allowed, d := releaseDataError(child)
			if !allowed {
				return false, false
			}
			data = data || d
		}
		return true, data
	}
	if one, ok := err.(interface{ Unwrap() error }); ok {
		return releaseDataError(one.Unwrap())
	}
	return false, false
}
