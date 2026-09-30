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
	ID        int
	Agent     string
	BeadID    string
	BeadTitle string
	Priority  int
	Reasoning string
	Labels    []string // bead labels; project:<name> picks the Home project
}

// DecisionQueue manages pending dispatch decisions.
type DecisionQueue struct {
	decisions []PendingDecision
	nextID    int

	mu     sync.Mutex
	filer  homeask.Filer
	lister homeask.Lister
	root   string
	filed  map[string]bool
	rows   []homeask.ListRow
	readAt time.Time
	stale  bool
}

// NewDecisionQueue creates an empty queue.
func NewDecisionQueue() *DecisionQueue {
	return &DecisionQueue{nextID: 1}
}

// Add queues a new decision.
func (q *DecisionQueue) Add(agent, beadID, beadTitle string, priority int, reasoning string) int {
	id := q.nextID
	q.nextID++
	q.decisions = append(q.decisions, PendingDecision{
		ID:        id,
		Agent:     agent,
		BeadID:    beadID,
		BeadTitle: beadTitle,
		Priority:  priority,
		Reasoning: reasoning,
	})
	return id
}

// Get returns a pending decision by ID.
func (q *DecisionQueue) Get(id int) (PendingDecision, bool) {
	for _, d := range q.decisions {
		if d.ID == id {
			return d, true
		}
	}
	return PendingDecision{}, false
}

// Remove removes a decision by ID (after approval or rejection).
func (q *DecisionQueue) Remove(id int) {
	for i, d := range q.decisions {
		if d.ID == id {
			q.decisions = append(q.decisions[:i], q.decisions[i+1:]...)
			return
		}
	}
}

// All returns all pending decisions.
func (q *DecisionQueue) All() []PendingDecision {
	return q.decisions
}

// Len returns the number of pending decisions. With a Lister it is the count
// of Mycroft's open asks in Home (last known when Home is down).
func (q *DecisionQueue) Len() int {
	if q.lister != nil {
		q.refresh()
		q.mu.Lock()
		defer q.mu.Unlock()
		return len(q.rows)
	}
	return len(q.decisions)
}

// HighestSeverity returns the severity of the most urgent pending decision.
func (q *DecisionQueue) HighestSeverity() Severity {
	highest := SeverityLow
	if q.lister != nil {
		q.refresh()
		q.mu.Lock()
		defer q.mu.Unlock()
		for _, r := range q.rows {
			if s := priorityToSeverity(r.Priority); s > highest {
				highest = s
			}
		}
		return highest
	}
	for _, d := range q.decisions {
		s := priorityToSeverity(d.Priority)
		if s > highest {
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

const (
	homeCacheTTL = 10 * time.Second
	defaultRoot  = "/home/mk/projects"
)

// SetHome connects the queue to Home. filer and lister may each be nil. root
// is the absolute project_root used for filed asks.
func (q *DecisionQueue) SetHome(f homeask.Filer, l homeask.Lister, root string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.filer, q.lister = f, l
	if root == "" {
		root = defaultRoot
	}
	q.root = root
	if q.filed == nil {
		q.filed = map[string]bool{}
	}
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
	rows, err := l.List(context.Background(), "mycroft")
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

// AddPending files a suggestion in Home once per (project, bead, agent). A
// repeat is a no-op. Without a filer it only queues locally.
func (q *DecisionQueue) AddPending(p PendingDecision) error {
	q.mu.Lock()
	f, root := q.filer, q.root
	q.mu.Unlock()
	if f == nil {
		q.Add(p.Agent, p.BeadID, p.BeadTitle, p.Priority, p.Reasoning)
		return nil
	}
	project := projectOf(p.Labels)
	rid := fmt.Sprintf("mycroft:%s:%s:%s", project, p.BeadID, p.Agent)
	q.mu.Lock()
	if q.filed[rid] {
		q.mu.Unlock()
		return nil
	}
	q.mu.Unlock()
	q_ := fmt.Sprintf("Mycroft suggests %s on %s: %s. Should it?", p.Agent, p.BeadID, p.BeadTitle)
	if p.Reasoning != "" {
		q_ += "\nWhy: " + p.Reasoning
	}
	q_ += "\nThis records your ruling; Mycroft does not dispatch from it until step 5."
	_, err := f.File(context.Background(), homeask.Ask{
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
