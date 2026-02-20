package status

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/pkg/tui"
)

// DispatchPane displays dispatches for the selected run.
type DispatchPane struct {
	dispatches []Dispatch
	runID      string
	width      int
	height     int
}

// NewDispatchPane creates a new dispatch pane.
func NewDispatchPane() *DispatchPane {
	return &DispatchPane{}
}

// SetDispatches updates the dispatch list.
func (p *DispatchPane) SetDispatches(runID string, dispatches []Dispatch) {
	p.runID = runID
	p.dispatches = dispatches
}

// SetSize updates pane dimensions.
func (p *DispatchPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// View renders the dispatch pane.
func (p *DispatchPane) View() string {
	title := "DISPATCHES"
	if p.runID != "" {
		title = fmt.Sprintf("DISPATCHES (%s)", p.runID)
	}
	header := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		Render(title)

	if len(p.dispatches) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(tui.ColorMuted).
			Render("  No dispatches")
		return header + "\n" + empty
	}

	var lines []string
	lines = append(lines, header)

	maxRows := p.height - 1
	if maxRows < 1 {
		maxRows = 1
	}

	for i, d := range p.dispatches {
		if i >= maxRows {
			break
		}
		lines = append(lines, renderDispatchRow(d, p.width))
	}

	return strings.Join(lines, "\n")
}

func renderDispatchRow(d Dispatch, width int) string {
	// Status symbol
	sym := tui.StatusSymbol(d.Status)

	// Dispatch ID (8 chars)
	id := d.ID
	if len(id) > 8 {
		id = id[:8]
	}

	// Name
	name := d.DisplayName()
	if len(name) > 20 {
		name = name[:20]
	}
	name = fmt.Sprintf("%-20s", name)

	// Status text
	status := fmt.Sprintf("%-10s", d.Status)

	// Duration
	dur := dispatchDuration(d)

	// Model
	model := d.DisplayModel()
	if len(model) > 12 {
		model = model[:12]
	}

	idStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)
	durStyle := lipgloss.NewStyle().Foreground(tui.ColorFgDim)
	modelStyle := lipgloss.NewStyle().Foreground(tui.ColorSecondary)

	return fmt.Sprintf("  %s %s %s %s %s %s",
		sym,
		idStyle.Render(fmt.Sprintf("%-8s", id)),
		name,
		status,
		durStyle.Render(fmt.Sprintf("%-7s", dur)),
		modelStyle.Render(model),
	)
}

func dispatchDuration(d Dispatch) string {
	if d.StartedAt == nil {
		return "—"
	}
	start := time.Unix(*d.StartedAt, 0)
	var end time.Time
	if d.CompletedAt != nil {
		end = time.Unix(*d.CompletedAt, 0)
	} else {
		end = time.Now()
	}
	dur := end.Sub(start)
	if dur < time.Minute {
		return fmt.Sprintf("%ds", int(dur.Seconds()))
	}
	if dur < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(dur.Minutes()), int(dur.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(dur.Hours()), int(dur.Minutes())%60)
}
