package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/intercore"
	"github.com/mistakeknot/autarch/pkg/signals"
)

// EventWatcher subscribes to Intercore's event stream and publishes
// events as signals through the broker. Complements DispatchWatcher
// by providing real-time notifications for phase changes, gate failures,
// budget warnings, and other sprint lifecycle events.
type EventWatcher struct {
	iclient *intercore.Client
	broker  *signals.Broker
	cancel  context.CancelFunc
}

// NewEventWatcher creates an event watcher.
// If broker or iclient is nil, Start() is a no-op.
func NewEventWatcher(iclient *intercore.Client, broker *signals.Broker) *EventWatcher {
	return &EventWatcher{
		iclient: iclient,
		broker:  broker,
	}
}

// eventWatcherTickMsg triggers event polling for active runs.
type eventWatcherTickMsg struct{}

// Start begins watching events. Returns a tea.Cmd that starts the
// event stream subscription for all active runs.
func (w *EventWatcher) Start() tea.Cmd {
	if w.iclient == nil || w.broker == nil {
		return nil
	}
	return w.poll()
}

// Stop cancels the event stream subscription.
func (w *EventWatcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

// poll fetches recent events for active runs and publishes as signals.
// Uses a polling approach (simpler than per-run follow streams) since
// runs may start/stop dynamically.
func (w *EventWatcher) poll() tea.Cmd {
	ic := w.iclient
	broker := w.broker
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Get active runs.
		runs, err := ic.RunList(ctx, true)
		if err != nil || len(runs) == 0 {
			// Wait and retry.
			time.Sleep(10 * time.Second)
			return eventWatcherTickMsg{}
		}

		// Fetch recent events for each active run (last 5).
		for _, run := range runs {
			events, err := ic.RunEvents(ctx, run.ID)
			if err != nil {
				continue
			}

			// Only process events from the last 30 seconds (avoid replays).
			cutoff := time.Now().Add(-30 * time.Second)
			for _, ev := range events {
				if ev.EventTime().Before(cutoff) {
					continue
				}
				sig := eventToSignal(ev, run)
				if sig != nil {
					broker.Publish(*sig)
				}
			}
		}

		// Wait before next poll.
		time.Sleep(10 * time.Second)
		return eventWatcherTickMsg{}
	}
}

// Tick returns the next poll command. Called from UnifiedApp.Update.
func (w *EventWatcher) Tick() tea.Cmd {
	if w.iclient == nil || w.broker == nil {
		return nil
	}
	return w.poll()
}

// eventToSignal converts an Intercore event to a signals.Signal.
// Returns nil for events that aren't worth surfacing.
func eventToSignal(ev intercore.Event, run intercore.Run) *signals.Signal {
	now := time.Now()

	switch ev.Type {
	case "phase_change":
		return &signals.Signal{
			ID:        fmt.Sprintf("ic-phase-%d", ev.ID),
			Type:      signals.SignalType("sprint_phase_change"),
			Source:    "intercore",
			Severity:  signals.SeverityInfo,
			Title:     fmt.Sprintf("Phase: %s → %s", ev.FromState, ev.ToState),
			Detail:    fmt.Sprintf("Sprint %s advanced", run.ID),
			CreatedAt: now,
		}

	case "gate_blocked":
		return &signals.Signal{
			ID:        fmt.Sprintf("ic-gate-%d", ev.ID),
			Type:      signals.SignalType("gate_blocked"),
			Source:    "intercore",
			Severity:  signals.SeverityWarning,
			Title:     fmt.Sprintf("Gate blocked: %s", ev.Reason),
			Detail:    fmt.Sprintf("Sprint %s gate check failed at %s", run.ID, run.Phase),
			CreatedAt: now,
		}

	case "budget_exceeded":
		return &signals.Signal{
			ID:        fmt.Sprintf("ic-budget-%d", ev.ID),
			Type:      signals.SignalType("budget_exceeded"),
			Source:    "intercore",
			Severity:  signals.SeverityCritical,
			Title:     "Token budget exceeded",
			Detail:    fmt.Sprintf("Sprint %s exceeded token budget", run.ID),
			CreatedAt: now,
		}

	case "dispatch_completed":
		sev := signals.SeverityInfo
		title := fmt.Sprintf("Dispatch completed: %s", ev.Source)
		if ev.Reason != "" {
			title += " — " + ev.Reason
		}
		return &signals.Signal{
			ID:        fmt.Sprintf("ic-dispatch-%d", ev.ID),
			Type:      signals.SignalType("dispatch_completed"),
			Source:    "intercore",
			Severity:  sev,
			Title:     title,
			Detail:    fmt.Sprintf("Sprint %s", run.ID),
			CreatedAt: now,
		}

	case "dispatch_failed":
		return &signals.Signal{
			ID:        fmt.Sprintf("ic-dispatch-%d", ev.ID),
			Type:      signals.SignalType("dispatch_failed"),
			Source:    "intercore",
			Severity:  signals.SeverityWarning,
			Title:     fmt.Sprintf("Dispatch failed: %s", ev.Source),
			Detail:    ev.Reason,
			CreatedAt: now,
		}

	case "run_cancelled":
		return &signals.Signal{
			ID:        fmt.Sprintf("ic-cancel-%d", ev.ID),
			Type:      signals.SignalType("sprint_cancelled"),
			Source:    "intercore",
			Severity:  signals.SeverityWarning,
			Title:     "Sprint cancelled",
			Detail:    fmt.Sprintf("Sprint %s: %s", run.ID, ev.Reason),
			CreatedAt: now,
		}
	}

	return nil
}
