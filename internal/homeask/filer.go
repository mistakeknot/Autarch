package homeask

import (
	"context"
	"errors"
)

// Typed outcomes of a filing. The autarch CLI maps them to distinct exit codes.
var (
	// ErrInvalid: the ask is malformed, or its request id belongs to a different ask.
	ErrInvalid = errors.New("invalid ask")
	// ErrHomeDown: bb is missing or its server is not reachable; nothing was filed.
	ErrHomeDown = errors.New("home is down, nothing filed")
	// ErrProjectNotFound identifies a request for a Tasks project bb no longer knows.
	ErrProjectNotFound = errors.New("project no longer exists in bb")
	// ErrOutcomeUnknown: the request may have been written; re-run the same command.
	ErrOutcomeUnknown = errors.New("outcome unknown, re-run the same command")
	// ErrAlreadyRuled: the decision has already been ruled on.
	ErrAlreadyRuled = errors.New("already ruled")
	// ErrAlreadyDelivered distinguishes a delivered request from a cancellation.
	ErrAlreadyDelivered = errors.New("request already delivered")
)

// Filer files an ask with Home and returns the decision id.
type Filer interface {
	File(ctx context.Context, a Ask) (string, error)
}

// HeldError reports an accepted queue entry without claiming a card was filed.
// Legacy string-ID consumers can distinguish it from a delivered card.
type HeldError struct {
	RequestKey string
	Reason     string
}

func (e *HeldError) Error() string {
	message := "is queued until needs-mk release"
	switch e.Reason {
	case "weekday-window":
		message = "is queued for the Thursday batch"
	case "non-blocking":
		message = "is queued by --non-blocking until needs-mk release"
	}
	return "request " + e.RequestKey + " " + message + "; no card has been filed"
}

// ListRow is one waiting ask as `bb home list --json` prints it.
type ListRow struct {
	TaskID   string `json:"task_id,omitempty"`
	ID       string `json:"id"`
	Subject  string `json:"subject"`
	Project  string `json:"project"`
	Thread   string `json:"thread"`
	Asker    string `json:"asker"`
	FiledAt  string `json:"filed_at"`
	Priority int    `json:"priority,omitempty"`
}

// Lister reads the asks waiting for mk, for one asker.
type Lister interface {
	List(ctx context.Context, asker string) ([]ListRow, error)
}
