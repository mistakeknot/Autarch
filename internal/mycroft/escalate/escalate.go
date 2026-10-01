// Package escalate handles user notifications and pending decision management.
package escalate

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mistakeknot/autarch/internal/homeask"
)

// Severity classifies the urgency of a notification.
type Severity int

const (
	SeverityLow    Severity = 0 // P2+ work pending
	SeverityMedium Severity = 1 // P1 work pending
	SeverityHigh   Severity = 2 // P0 work pending or failure detected
)

// Notify sends a desktop notification.
func Notify(title, body string, severity Severity) error {
	switch runtime.GOOS {
	case "linux":
		urgency := "normal"
		if severity >= SeverityHigh {
			urgency = "critical"
		}
		return exec.Command("notify-send", "-u", urgency, title, body).Run()
	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		return exec.Command("osascript", "-e", script).Run()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// Bell sends a terminal bell character.
func Bell() {
	fmt.Print("\a")
}

// Badge returns a severity-aware status badge string.
func Badge(pendingCount int, highestSeverity Severity) string {
	if pendingCount == 0 {
		return "✓ idle"
	}
	switch highestSeverity {
	case SeverityHigh:
		return fmt.Sprintf("⚠ %d pending", pendingCount)
	default:
		return fmt.Sprintf("● %d pending", pendingCount)
	}
}

// PendingDecision represents a dispatch suggestion awaiting user input.
type PendingDecision struct {
	Agent     string
	BeadID    string
	BeadTitle string
	Priority  int
	Reasoning string
	Labels    []string // bead labels; project:<name> picks the Home project
}

// PullFiler files Mycroft's threadless asks. It is the single threadless filing path
// (homeask.CardFiler.FileForPull, mk question 4): the queue never files with a thread.
type PullFiler interface {
	FileForPull(ctx context.Context, a homeask.Ask) (string, error)
}

// DecisionQueue files Mycroft's suggestions in Home as pull cards and reads their outcomes back.
// It keeps no list of its own: Home is the only list (Len, All, Get and HighestSeverity read
// `bb home list --pull mycroft`, which also covers pre-card asks).
type DecisionQueue struct {
	mu     sync.Mutex
	filer  PullFiler
	lister homeask.CardLister
	root   string
	roots  RootResolver
	filed  map[string]bool
	rows   []homeask.ListRow
	readAt time.Time
	stale  bool
}

// NewDecisionQueue creates a queue that is wired to Home with SetHome or SetHomeRoots.
func NewDecisionQueue() *DecisionQueue {
	return &DecisionQueue{}
}

// Get returns an open Home row by decision id or card id (last known when Home is down).
func (q *DecisionQueue) Get(id string) (homeask.ListRow, bool) {
	for _, r := range q.All() {
		if r.ID == id || (r.TaskID != "" && r.TaskID == id) {
			return r, true
		}
	}
	return homeask.ListRow{}, false
}

// Outcome reads one card's state in Home (`bb home get --card`): whether mk has ruled.
func (q *DecisionQueue) Outcome(ctx context.Context, taskID string) (homeask.CardView, error) {
	q.mu.Lock()
	l := q.lister
	q.mu.Unlock()
	if l == nil {
		return homeask.CardView{}, fmt.Errorf("the decision queue is not wired to Home")
	}
	return l.Card(ctx, taskID)
}

// Remove is a no-op. A suggestion leaves the queue when mk rules in Home; Mycroft has no
// private list to take it out of.
func (q *DecisionQueue) Remove(id string) {}

// All returns Mycroft's open asks in Home (last known when Home is down).
func (q *DecisionQueue) All() []homeask.ListRow {
	q.mu.Lock()
	l := q.lister
	q.mu.Unlock()
	if l == nil {
		return nil
	}
	q.refresh()
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]homeask.ListRow(nil), q.rows...)
}

// Len returns the number of Mycroft's open asks in Home (last known when Home is down).
func (q *DecisionQueue) Len() int { return len(q.All()) }

// HighestSeverity returns the severity of the most urgent open ask in Home.
func (q *DecisionQueue) HighestSeverity() Severity {
	highest := SeverityLow
	for _, r := range q.All() {
		if s := priorityToSeverity(r.Priority); s > highest {
			highest = s
		}
	}
	return highest
}

func priorityToSeverity(priority int) Severity {
	switch {
	case priority <= 0:
		return SeverityHigh
	case priority <= 1:
		return SeverityMedium
	default:
		return SeverityLow
	}
}

const homeCacheTTL = 10 * time.Second

// RootResolver maps a project label ("estate" for none) to the exact project name and
// root Home resolves for it. Home rejects an ask whose project or root differs.
type RootResolver func(project string) (name, root string, err error)

// SetHome connects the queue to Home with one fixed project_root for every ask (tests
// and single-project setups). Without a root nothing is filed: Home would reject a
// guessed one.
func (q *DecisionQueue) SetHome(f PullFiler, l homeask.CardLister, root string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.filer, q.lister, q.root, q.roots = f, l, root, nil
	if q.filed == nil {
		q.filed = map[string]bool{}
	}
}

// SetHomeRoots connects the queue to Home and resolves each ask's project name and
// root through roots, as Home does.
func (q *DecisionQueue) SetHomeRoots(f PullFiler, l homeask.CardLister, roots RootResolver) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.filer, q.lister, q.root, q.roots = f, l, "", roots
	if q.filed == nil {
		q.filed = map[string]bool{}
	}
}

// Filer returns the wired pull filer, or nil.
func (q *DecisionQueue) Filer() PullFiler {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.filer
}

// HasHome reports whether a filer is wired.
func (q *DecisionQueue) HasHome() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.filer != nil || q.lister != nil
}

// Stale reports that the last read of Home failed, so counts are last-known.
func (q *DecisionQueue) Stale() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.stale
}

func (q *DecisionQueue) expireCache() {
	q.mu.Lock()
	q.readAt = time.Time{}
	q.mu.Unlock()
}

func (q *DecisionQueue) refresh() {
	q.mu.Lock()
	if !q.readAt.IsZero() && time.Since(q.readAt) < homeCacheTTL {
		q.mu.Unlock()
		return
	}
	l := q.lister
	q.mu.Unlock()
	rows, err := l.ListPull(context.Background())
	q.mu.Lock()
	defer q.mu.Unlock()
	q.readAt = time.Now()
	if err != nil {
		q.stale = true
		return
	}
	q.rows, q.stale = rows, false
}

func projectOf(labels []string) string {
	for _, l := range labels {
		if strings.HasPrefix(l, "project:") && len(l) > len("project:") {
			return strings.ToLower(strings.TrimPrefix(l, "project:"))
		}
	}
	return "estate"
}

// AddPending files a suggestion in Home once per (project, bead, agent). A repeat is a no-op.
// Without a filer nothing is filed and an error says so: there is no local-only queue.
func (q *DecisionQueue) AddPending(p PendingDecision) error {
	q.mu.Lock()
	f, root, roots := q.filer, q.root, q.roots
	q.mu.Unlock()
	if f == nil {
		return fmt.Errorf("not filed: the decision queue is not wired to Home")
	}
	project := projectOf(p.Labels)
	rid := fmt.Sprintf("mycroft:%s:%s:%s", project, p.BeadID, p.Agent)
	q.mu.Lock()
	if q.filed[rid] {
		q.mu.Unlock()
		return nil
	}
	q.mu.Unlock()
	if roots != nil {
		name, r, err := roots(project)
		if err != nil {
			return fmt.Errorf("not filed: no Home root for project %q: %w", project, err)
		}
		project, root = name, r
	}
	if root == "" {
		return fmt.Errorf("not filed: no project_root for project %q", project)
	}
	q_ := fmt.Sprintf("Mycroft suggests %s on %s: %s. Should it?", p.Agent, p.BeadID, p.BeadTitle)
	if p.Reasoning != "" {
		q_ += "\nWhy: " + p.Reasoning
	}
	q_ += "\nThis records your ruling; Mycroft does not dispatch from it until step 5."
	_, err := f.FileForPull(context.Background(), homeask.Ask{
		V: 1, Kind: "decide", RequestID: rid,
		Subject:     fmt.Sprintf("mycroft/%s/%s", p.BeadID, p.Agent),
		Project:     project,
		ProjectRoot: root,
		Asker:       "mycroft",
		Question:    q_,
		Options: []homeask.Option{
			{ID: "yes", Label: "Yes, dispatch it", Kind: "ruling-only"},
			{ID: "no", Label: "No, leave it", Kind: "ruling-only"},
		},
	})
	if err != nil {
		return err
	}
	q.mu.Lock()
	q.filed[rid] = true
	q.mu.Unlock()
	return nil
}
