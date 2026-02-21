package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// LaneData holds the display data for a single lane row.
type LaneData struct {
	Name       string
	LaneType   string
	OpenBeads  int
	Closed     int
	Throughput float64
	Starvation float64
}

// LanePane displays lane progress in a table format.
type LanePane struct {
	viewport viewport.Model
	lanes    []LaneData
	width    int
	height   int
}

// NewLanePane creates a new lane dashboard pane.
func NewLanePane() *LanePane {
	return &LanePane{}
}

// SetSize updates pane dimensions.
func (p *LanePane) SetSize(width, height int) {
	if width < 4 || height < 2 {
		return
	}
	p.width = width
	p.height = height
	p.viewport = viewport.New(width-2, height-1)
	p.viewport.MouseWheelEnabled = true
	p.updateContent()
}

// SetLanes updates the lane data displayed.
func (p *LanePane) SetLanes(lanes []LaneData) {
	p.lanes = lanes
	p.updateContent()
}

func (p *LanePane) updateContent() {
	if p.width == 0 {
		return
	}

	var b strings.Builder

	if len(p.lanes) == 0 {
		dimStyle := lipgloss.NewStyle().Foreground(ColorMuted)
		b.WriteString(dimStyle.Render("No lanes configured. Use /clavain:lane discover"))
		p.viewport.SetContent(b.String())
		return
	}

	// Header
	headerStyle := lipgloss.NewStyle().
		Foreground(ColorFgDim).
		Bold(true)

	header := fmt.Sprintf("%-14s %-9s %5s %6s %6s  %s",
		"LANE", "TYPE", "OPEN", "DONE", "VEL", "STARVATION")
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// Separator
	sepStyle := lipgloss.NewStyle().Foreground(ColorBorder)
	b.WriteString(sepStyle.Render(strings.Repeat("─", min(p.width-4, 60))))
	b.WriteString("\n")

	// Rows
	for _, l := range p.lanes {
		nameStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		typeStyle := lipgloss.NewStyle().Foreground(ColorMuted)

		name := nameStyle.Render(fmt.Sprintf("%-14s", truncate(l.Name, 14)))
		ltype := typeStyle.Render(fmt.Sprintf("%-9s", l.LaneType))
		open := fmt.Sprintf("%5d", l.OpenBeads)
		done := fmt.Sprintf("%6d", l.Closed)
		vel := fmt.Sprintf("%6.1f", l.Throughput)

		// Starvation bar
		bar := starvationBar(l.Starvation, 4)
		starvNum := fmt.Sprintf(" %.1f", l.Starvation)

		b.WriteString(fmt.Sprintf("%s %s %s %s %s  %s%s\n",
			name, ltype, open, done, vel, bar, starvNum))
	}

	p.viewport.SetContent(b.String())
}

// View renders the lane pane.
func (p *LanePane) View() string {
	return p.viewport.View()
}

// starvationBar renders a 4-char bar visualization of starvation level.
func starvationBar(score float64, width int) string {
	// Score is unbounded; capped at width blocks for display.
	filled := int(score / 12.5)
	if filled > width {
		filled = width
	}

	highStyle := lipgloss.NewStyle().Foreground(ColorError)
	midStyle := lipgloss.NewStyle().Foreground(ColorWarning)
	lowStyle := lipgloss.NewStyle().Foreground(ColorSuccess)
	emptyStyle := lipgloss.NewStyle().Foreground(ColorMuted)

	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			switch {
			case filled >= 3:
				b.WriteString(highStyle.Render("█"))
			case filled == 2:
				b.WriteString(midStyle.Render("█"))
			default:
				b.WriteString(lowStyle.Render("█"))
			}
		} else {
			b.WriteString(emptyStyle.Render("░"))
		}
	}
	return b.String()
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

