package status

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/pkg/tui"
)

// EventsPane displays the event stream for the selected run.
type EventsPane struct {
	events []Event
	width  int
	height int
}

// NewEventsPane creates a new events pane.
func NewEventsPane() *EventsPane {
	return &EventsPane{}
}

// SetEvents updates the event list.
func (p *EventsPane) SetEvents(events []Event) {
	p.events = events
}

// SetSize updates pane dimensions.
func (p *EventsPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// View renders the events pane.
func (p *EventsPane) View() string {
	title := fmt.Sprintf("EVENTS (last %d)", len(p.events))
	if len(p.events) == 0 {
		title = "EVENTS"
	}
	header := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		Render(title)

	if len(p.events) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(tui.ColorMuted).
			Render("  No events")
		return header + "\n" + empty
	}

	var lines []string
	lines = append(lines, header)

	maxRows := p.height - 1
	if maxRows < 1 {
		maxRows = 1
	}

	// Show newest events last (tail), so start from the end if we have more than maxRows
	start := 0
	if len(p.events) > maxRows {
		start = len(p.events) - maxRows
	}

	for i := start; i < len(p.events); i++ {
		lines = append(lines, renderEventRow(p.events[i], p.width))
	}

	return strings.Join(lines, "\n")
}

func renderEventRow(ev Event, width int) string {
	// Parse and format timestamp
	ts := formatEventTime(ev.Timestamp)

	// Event type with color
	evType := fmt.Sprintf("%-16s", ev.Type)
	typeStyle := eventTypeStyle(ev.Type)

	// State transition
	transition := ""
	if ev.FromState != "" && ev.ToState != "" {
		transition = fmt.Sprintf("%s → %s", ev.FromState, ev.ToState)
	} else if ev.ToState != "" {
		transition = ev.ToState
	}
	if len(transition) > 24 {
		transition = transition[:24]
	}

	tsStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)
	transStyle := lipgloss.NewStyle().Foreground(tui.ColorFgDim)

	row := fmt.Sprintf("  %s  %s  %s",
		tsStyle.Render(ts),
		typeStyle.Render(evType),
		transStyle.Render(transition),
	)

	// Add reason if space permits
	if ev.Reason != "" {
		remaining := width - lipgloss.Width(row) - 2
		if remaining > 10 {
			reason := ev.Reason
			if len(reason) > remaining {
				reason = reason[:remaining-1] + "…"
			}
			row += "  " + lipgloss.NewStyle().Foreground(tui.ColorMuted).Render(reason)
		}
	}

	return row
}

func formatEventTime(timestamp string) string {
	// Try parsing as RFC3339
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		// Try RFC3339Nano
		t, err = time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			// Fall back to showing raw (truncated)
			if len(timestamp) > 8 {
				return timestamp[:8]
			}
			return timestamp
		}
	}
	return t.Local().Format("15:04:05")
}

func eventTypeStyle(evType string) lipgloss.Style {
	switch {
	case strings.Contains(evType, "error"), strings.Contains(evType, "fail"), strings.Contains(evType, "block"):
		return lipgloss.NewStyle().Foreground(tui.ColorError)
	case strings.Contains(evType, "complete"), strings.Contains(evType, "done"), strings.Contains(evType, "pass"):
		return lipgloss.NewStyle().Foreground(tui.ColorSuccess)
	case strings.Contains(evType, "advance"), strings.Contains(evType, "start"):
		return lipgloss.NewStyle().Foreground(tui.ColorWarning)
	default:
		return lipgloss.NewStyle().Foreground(tui.ColorFg)
	}
}
