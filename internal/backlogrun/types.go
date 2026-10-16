// Package backlogrun is the recurring backlog run (bead mk-omogp.2): it snapshots every BB Tasks board's backlog and
// todo cards, labels the new or changed ones cheaply, cross-checks the mk-gated ones against Home (read-only), and
// sends each coordinator one message. The pipeline touches the outside world only through the interfaces below, so
// it can later move into Mycroft's scheduler unchanged (T1 proposes, T2 after the Mycroft spawner ships).
package backlogrun

import "context"

// Labels a triage gives a card.
const (
	LabelReady     = "ready"      // an executor could start it now
	LabelNeedsSpec = "needs-spec" // vague: needs a done-condition and a size before anyone builds it
	LabelMKGated   = "mk-gated"   // waits on a decision only mk can make
	LabelClose     = "close"      // stale, duplicate or already done
	LabelStanding  = "standing"   // an epic or steward card: left alone
)

// Card is one board card as the snapshot sees it.
type Card struct {
	Key         string   `json:"key"`
	Board       string   `json:"board"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority"`
	UpdatedAt   string   `json:"updatedAt"`
	Labels      []string `json:"labels,omitempty"`
	Working     int      `json:"agentsWorking"`
}

// Verdict is a triage result for one card.
type Verdict struct {
	Key         string `json:"key"`
	Board       string `json:"board"`
	Label       string `json:"label"`
	Size        string `json:"size,omitempty"`
	Seat        string `json:"seat,omitempty"`
	Reason      string `json:"reason"`
	Decision    string `json:"decision,omitempty"` // for mk-gated: the exact question mk must answer
	DuplicateOf string `json:"duplicate_of,omitempty"`
	// UpdatedAt is the card's updatedAt when it was triaged: a verdict only holds while the card still has it.
	UpdatedAt string `json:"updatedAt"`
}

// Seat is the triage seat routing gave, kept in the run receipt.
type Seat struct {
	Role       string `json:"role"`
	ProfileRef string `json:"profile_ref"`
	Backend    string `json:"backend"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
	PolicyHash string `json:"policy_hash"`
}

// BoardSource reads the boards.
type BoardSource interface {
	// Cards returns every card in backlog or todo, on all boards.
	Cards(ctx context.Context) ([]Card, error)
	// InFlight returns, per board prefix, the number of cards being worked (in progress, in review or with a live agent).
	InFlight(ctx context.Context) (map[string]int, error)
}

// Triager labels cards. It must fail closed: no routing seat, no triage.
type Triager interface {
	Triage(ctx context.Context, cards []Card) ([]Verdict, Seat, error)
}

// HomeReader reads Home's card states, read-only.
type HomeReader interface {
	// States returns, for each key that has a Home card, its state (open, display, ruled, closed). Keys with no card are absent.
	States(ctx context.Context, keys []string) (map[string]string, error)
}

// Notifier delivers messages.
type Notifier interface {
	// Coordinator returns the coordinator thread for a board prefix, or "" when none (or more than one) is known.
	Coordinator(ctx context.Context, board string) (string, error)
	Tell(ctx context.Context, thread, message string) error
	TellVizier(ctx context.Context, message string) error
}

// Pool says whether quota allows dispatching more work.
type Pool interface {
	Headroom(ctx context.Context) (ok bool, why string, err error)
}
