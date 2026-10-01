package homeask

import (
	"context"
	"errors"
)

// Typed outcomes of a filing. The autarch CLI maps them to exit codes 2, 3, 4 and 5.
var (
	// ErrInvalid: the ask is malformed, or its request id belongs to a different ask.
	ErrInvalid = errors.New("invalid ask")
	// ErrHomeDown: bb is missing or its server is not reachable; nothing was filed.
	ErrHomeDown = errors.New("home is down, nothing filed")
	// ErrOutcomeUnknown: the request may have been written; re-run the same command.
	ErrOutcomeUnknown = errors.New("outcome unknown, re-run the same command")
	// ErrAlreadyRuled: the decision has already been ruled on.
	ErrAlreadyRuled = errors.New("already ruled")
)

// Filer files an ask with Home and returns the decision id.
type Filer interface {
	File(ctx context.Context, a Ask) (string, error)
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
