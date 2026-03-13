// Package escalate handles user notifications and pending decision management.
package escalate

import (
	"fmt"
	"os/exec"
	"runtime"
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
	ID       int
	Agent    string
	BeadID   string
	BeadTitle string
	Priority int
	Reasoning string
}

// DecisionQueue manages pending dispatch decisions.
type DecisionQueue struct {
	decisions []PendingDecision
	nextID    int
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

// Len returns the number of pending decisions.
func (q *DecisionQueue) Len() int {
	return len(q.decisions)
}

// HighestSeverity returns the severity of the most urgent pending decision.
func (q *DecisionQueue) HighestSeverity() Severity {
	highest := SeverityLow
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
