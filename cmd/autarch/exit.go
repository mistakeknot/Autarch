package main

import (
	"errors"

	"github.com/mistakeknot/autarch/internal/homeask"
)

// usageError is a validation failure of the command line itself (exit 2).
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// exitCode maps an error to the process exit status. The decide commands use the
// same codes as `bb home`: 2 validation, 3 Home down or not filed, 4 outcome
// unknown (re-run the same command), 5 already ruled, 1 anything else.
func exitCode(err error) int {
	var ue *usageError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ue), errors.Is(err, homeask.ErrInvalid):
		return 2
	case errors.Is(err, homeask.ErrHomeDown):
		return 3
	case errors.Is(err, homeask.ErrOutcomeUnknown):
		return 4
	case errors.Is(err, homeask.ErrAlreadyRuled):
		return 5
	default:
		return 1
	}
}
