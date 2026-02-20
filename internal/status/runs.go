package status

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/pkg/tui"
)

// RunsPane displays a navigable list of runs with phase progress bars.
type RunsPane struct {
	runs   []Run
	cursor int
	width  int
	height int
}

// NewRunsPane creates a new runs pane.
func NewRunsPane() *RunsPane {
	return &RunsPane{}
}

// SetRuns updates the run list, preserving cursor position.
func (p *RunsPane) SetRuns(runs []Run) {
	p.runs = runs
	if p.cursor >= len(runs) {
		p.cursor = max(0, len(runs)-1)
	}
}

// SelectedRun returns the currently selected run, or nil if none.
func (p *RunsPane) SelectedRun() *Run {
	if len(p.runs) == 0 || p.cursor >= len(p.runs) {
		return nil
	}
	return &p.runs[p.cursor]
}

// CursorUp moves the cursor up.
func (p *RunsPane) CursorUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

// CursorDown moves the cursor down.
func (p *RunsPane) CursorDown() {
	if p.cursor < len(p.runs)-1 {
		p.cursor++
	}
}

// SetSize updates pane dimensions.
func (p *RunsPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// View renders the runs pane.
func (p *RunsPane) View() string {
	header := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		Render("RUNS")

	if len(p.runs) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(tui.ColorMuted).
			Render("  No active runs")
		return header + "\n" + empty
	}

	var lines []string
	lines = append(lines, header)

	// Calculate available width for each column
	maxRows := p.height - 1 // subtract header
	if maxRows < 1 {
		maxRows = 1
	}

	for i, run := range p.runs {
		if i >= maxRows {
			break
		}
		line := renderRunRow(run, p.width, i == p.cursor)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func renderRunRow(run Run, width int, selected bool) string {
	// Status symbol
	sym := tui.StatusSymbol(run.Status)

	// Run ID (8 chars)
	id := run.ID
	if len(id) > 8 {
		id = id[:8]
	}
	id = fmt.Sprintf("%-8s", id)

	// Phase progress bar
	progress := renderProgressBar(run.Phase, run.Phases, 8)

	// Phase name (truncated)
	phase := run.Phase
	if len(phase) > 14 {
		phase = phase[:14]
	}
	phase = fmt.Sprintf("%-14s", phase)

	// Goal takes remaining space
	idStyle := lipgloss.NewStyle().Foreground(tui.ColorSecondary)
	phaseStyle := lipgloss.NewStyle().Foreground(tui.ColorFgDim)

	goalWidth := width - 8 - 8 - 14 - 10 - 6 // sym + id + phase + progress + padding
	if goalWidth < 10 {
		goalWidth = 10
	}
	goal := run.Goal
	if len(goal) > goalWidth {
		goal = goal[:goalWidth-1] + "…"
	}
	goal = fmt.Sprintf("%-*s", goalWidth, goal)

	row := fmt.Sprintf("  %s %s %s %s %s",
		sym,
		idStyle.Render(id),
		goal,
		phaseStyle.Render(phase),
		progress,
	)

	if selected {
		row = lipgloss.NewStyle().
			Background(tui.ColorBgLighter).
			Width(width).
			Render(row)
	}

	return row
}

func renderProgressBar(currentPhase string, phases []string, barWidth int) string {
	if len(phases) == 0 {
		// No phase list available — show unknown
		return lipgloss.NewStyle().Foreground(tui.ColorMuted).Render(strings.Repeat("░", barWidth))
	}

	idx := -1
	for i, p := range phases {
		if p == currentPhase {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	}

	filled := ((idx + 1) * barWidth) / len(phases)
	if filled > barWidth {
		filled = barWidth
	}

	bar := lipgloss.NewStyle().Foreground(tui.ColorSuccess).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(tui.ColorMuted).Render(strings.Repeat("░", barWidth-filled))

	return bar
}
