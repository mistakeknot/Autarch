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
	if err == ErrInvalid || err == ErrProjectNotFound {
		return true, true
	}
	if err == errNoCardWritten {
		return true, false
	}
	if many, ok := err.(interface{ Unwrap() []error }); ok {
		// Proof applies only to sibling causes in this join. Nested joins must
		// establish their own proof, so an unrelated branch cannot hide a race.
		noWrite := false
		for _, child := range many.Unwrap() {
			noWrite = noWrite || releaseBranchIs(child, errNoCardWritten)
		}
		data := false
		for _, child := range many.Unwrap() {
			if noWrite && releaseBranchIs(child, ErrAlreadyRuled) {
				data = true
				continue
			}
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

// releaseBranchIs follows single-error wrappers without crossing another join.
func releaseBranchIs(err, target error) bool {
	if err == target {
		return true
	}
	if one, ok := err.(interface{ Unwrap() error }); ok {
		return releaseBranchIs(one.Unwrap(), target)
	}
	return false
}
