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
	owner string
	cause error
}

func (e *releaseOwnerError) Error() string {
	return fmt.Sprintf("owner %s needs attention: %v; request remains held; assign a live owner or cancel and refile", e.owner, e.cause)
}
func (e *releaseOwnerError) Unwrap() error { return e.cause }
