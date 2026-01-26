// Package research provides research run coordination for Pollard.
// It handles run identity for race prevention and topic-scoped updates.
package research

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Run represents a single research execution with stable identity.
// The RunID prevents stale updates from affecting UI state when
// users switch projects or cancel research mid-flight.
type Run struct {
	RunID     string
	ProjectID string
	StartedAt time.Time
	Context   context.Context
	Cancel    context.CancelFunc

	mu       sync.RWMutex
	updates  []Update
	hunters  map[string]HunterStatus
	done     bool
	doneAt   time.Time
}

// HunterStatus tracks the state of a single hunter within a run.
type HunterStatus struct {
	Name       string
	Status     Status
	StartedAt  time.Time
	FinishedAt time.Time
	Findings   int
	Error      string
}

// Status represents hunter execution state.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusComplete  Status = "complete"
	StatusError     Status = "error"
	StatusCancelled Status = "cancelled"
)

// Update represents a scoped research finding for a specific topic.
// Updates include the RunID to prevent stale data from affecting UI.
type Update struct {
	RunID      string    // Must match active run
	HunterName string    // Which hunter produced this
	TopicKey   string    // e.g., "platform", "sync", "auth"
	Findings   []Finding
	Timestamp  time.Time
}

// Finding represents a single research insight.
type Finding struct {
	ID          string   // Stable InsightID for later reference
	Title       string
	Summary     string
	Source      string   // URL or reference
	SourceType  string   // github, arxiv, hackernews, etc.
	Relevance   float64  // 0.0-1.0 score
	Tags        []string
	CollectedAt time.Time
}

// TradeoffOption represents a suggestion with pros/cons for interview questions.
type TradeoffOption struct {
	Label      string   // Display label
	Pros       []string // ✓ items
	Cons       []string // ✗ items
	InsightID  string   // Stable reference to source insight
	Sources    []string // Source URLs
	Popularity string   // e.g., "73% of projects"
}

// NewRun creates a new research run with a fresh UUID.
func NewRun(projectID string) *Run {
	ctx, cancel := context.WithCancel(context.Background())
	return &Run{
		RunID:     uuid.New().String(),
		ProjectID: projectID,
		StartedAt: time.Now(),
		Context:   ctx,
		Cancel:    cancel,
		hunters:   make(map[string]HunterStatus),
	}
}

// NewRunWithContext creates a run with a parent context.
func NewRunWithContext(ctx context.Context, projectID string) *Run {
	runCtx, cancel := context.WithCancel(ctx)
	return &Run{
		RunID:     uuid.New().String(),
		ProjectID: projectID,
		StartedAt: time.Now(),
		Context:   runCtx,
		Cancel:    cancel,
		hunters:   make(map[string]HunterStatus),
	}
}

// RegisterHunter adds a hunter to be tracked in this run.
func (r *Run) RegisterHunter(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hunters[name] = HunterStatus{
		Name:   name,
		Status: StatusPending,
	}
}

// StartHunter marks a hunter as running.
func (r *Run) StartHunter(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.hunters[name]; ok {
		h.Status = StatusRunning
		h.StartedAt = time.Now()
		r.hunters[name] = h
	}
}

// CompleteHunter marks a hunter as complete with findings count.
func (r *Run) CompleteHunter(name string, findingsCount int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.hunters[name]; ok {
		h.Status = StatusComplete
		h.FinishedAt = time.Now()
		h.Findings = findingsCount
		r.hunters[name] = h
	}
}

// ErrorHunter marks a hunter as failed.
func (r *Run) ErrorHunter(name string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.hunters[name]; ok {
		h.Status = StatusError
		h.FinishedAt = time.Now()
		h.Error = err.Error()
		r.hunters[name] = h
	}
}

// AddUpdate records a topic-scoped update from a hunter.
func (r *Run) AddUpdate(update Update) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate RunID matches
	if update.RunID != r.RunID {
		return // Stale update, ignore
	}

	update.Timestamp = time.Now()
	r.updates = append(r.updates, update)
}

// GetUpdatesForTopic returns all updates matching the given topic key.
func (r *Run) GetUpdatesForTopic(topicKey string) []Update {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Update
	for _, u := range r.updates {
		if u.TopicKey == topicKey {
			result = append(result, u)
		}
	}
	return result
}

// GetAllUpdates returns all updates for this run.
func (r *Run) GetAllUpdates() []Update {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Update{}, r.updates...)
}

// GetHunterStatuses returns the current status of all hunters.
func (r *Run) GetHunterStatuses() map[string]HunterStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]HunterStatus)
	for k, v := range r.hunters {
		result[k] = v
	}
	return result
}

// RunningCount returns the number of currently running hunters.
func (r *Run) RunningCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, h := range r.hunters {
		if h.Status == StatusRunning {
			count++
		}
	}
	return count
}

// IsComplete returns true if all hunters have finished (complete or error).
func (r *Run) IsComplete() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.hunters) == 0 {
		return false
	}

	for _, h := range r.hunters {
		if h.Status == StatusPending || h.Status == StatusRunning {
			return false
		}
	}
	return true
}

// IsCancelled returns true if the run has been cancelled.
func (r *Run) IsCancelled() bool {
	select {
	case <-r.Context.Done():
		return true
	default:
		return false
	}
}

// MarkDone marks the run as complete.
func (r *Run) MarkDone() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = true
	r.doneAt = time.Now()
}

// Duration returns how long the run has been active.
func (r *Run) Duration() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.done {
		return r.doneAt.Sub(r.StartedAt)
	}
	return time.Since(r.StartedAt)
}

// TotalFindings returns the total number of findings across all hunters.
func (r *Run) TotalFindings() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := 0
	for _, h := range r.hunters {
		total += h.Findings
	}
	return total
}

// IsDone returns true if the run has been marked as done via MarkDone().
func (r *Run) IsDone() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.done
}
