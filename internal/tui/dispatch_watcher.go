package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/intercore"
)

// DispatchCompletedMsg is sent when a dispatch transitions to a terminal state.
type DispatchCompletedMsg struct {
	Dispatch intercore.Dispatch
}

// dispatchTickMsg triggers the next poll cycle.
type dispatchTickMsg struct{}

// DispatchWatcher polls Intercore for dispatch status changes.
// It tracks known dispatches and emits DispatchCompletedMsg when one finishes.
type DispatchWatcher struct {
	ic       *intercore.Client
	interval time.Duration
	// known tracks dispatch IDs we've already seen in a terminal state,
	// so we don't emit duplicate completion messages.
	known map[string]string // dispatchID → last seen status
}

// NewDispatchWatcher creates a watcher that polls at the given interval.
func NewDispatchWatcher(ic *intercore.Client, interval time.Duration) *DispatchWatcher {
	return &DispatchWatcher{
		ic:       ic,
		interval: interval,
		known:    make(map[string]string),
	}
}

// Start returns the initial tick command to begin polling.
func (w *DispatchWatcher) Start() tea.Cmd {
	if w == nil || w.ic == nil {
		return nil
	}
	return w.tick()
}

// tick schedules the next poll after the configured interval.
func (w *DispatchWatcher) tick() tea.Cmd {
	return tea.Tick(w.interval, func(time.Time) tea.Msg {
		return dispatchTickMsg{}
	})
}

// Poll checks for dispatch completions and returns messages + the next tick.
func (w *DispatchWatcher) Poll() tea.Cmd {
	if w == nil || w.ic == nil {
		return nil
	}
	ic := w.ic
	known := w.known
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Fetch all dispatches (not just active — we need to see completions).
		dispatches, err := ic.DispatchList(ctx, false)
		if err != nil {
			// Silently retry on next tick.
			return dispatchTickMsg{}
		}

		var completed []DispatchCompletedMsg
		for _, d := range dispatches {
			prev, seen := known[d.ID]
			if isTerminal(d.Status) && (!seen || prev != d.Status) {
				known[d.ID] = d.Status
				completed = append(completed, DispatchCompletedMsg{Dispatch: d})
			} else if !isTerminal(d.Status) {
				// Track non-terminal so we detect the transition.
				known[d.ID] = d.Status
			}
		}

		if len(completed) > 0 {
			return dispatchBatchMsg{completed: completed}
		}
		return dispatchTickMsg{}
	}
}

// dispatchBatchMsg carries multiple completions from a single poll.
type dispatchBatchMsg struct {
	completed []DispatchCompletedMsg
}

func isTerminal(status string) bool {
	return status == "completed" || status == "failed" || status == "cancelled"
}
