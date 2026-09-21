// Package components provides reusable TUI components for Autarch.
package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// Teaser displays a fixed-height research summary inline with interview questions.
// The fixed height prevents layout shift as research results arrive.
type Teaser struct {
	runID    string // Must match active run
	topicKey string // Scoped to current question
	height   int    // Fixed height (usually 3-4 lines)
	width    int

	// Cached findings for this topic
	findings []research.Finding
	summary  string
}

// TeaserHeight is the default fixed height for teasers.
const TeaserHeight = 4

// NewTeaser creates a new teaser for a specific topic.
func NewTeaser(topicKey string, height int) *Teaser {
	if height <= 0 {
		height = TeaserHeight
	}
	return &Teaser{
		topicKey: topicKey,
		height:   height,
	}
}

// SetSize updates the teaser width.
func (t *Teaser) SetSize(width int) {
	t.width = width
}

// UpdateFromRun updates the teaser with findings from a research run.
// Returns false if the runID doesn't match (stale update).
func (t *Teaser) UpdateFromRun(runID string, updates []research.Update) bool {
	if t.runID != "" && t.runID != runID {
		return false // Stale update, ignore
	}

	t.runID = runID
	t.findings = nil

	// Filter updates for this topic
	for _, u := range updates {
		if u.RunID != runID {
			continue
		}
		if u.TopicKey == t.topicKey || t.topicKey == "" {
			t.findings = append(t.findings, u.Findings...)
		}
	}

	// Generate summary
	t.generateSummary()
	return true
}

// UpdateFindings directly updates findings (useful for testing).
func (t *Teaser) UpdateFindings(findings []research.Finding) {
	t.findings = findings
	t.generateSummary()
}

func (t *Teaser) generateSummary() {
	if len(t.findings) == 0 {
		t.summary = ""
		return
	}

	// Combine top finding summaries
	var summaries []string
	for i, f := range t.findings {
		if i >= 2 {
			break // Limit to 2 findings in teaser
		}
		summaries = append(summaries, f.Summary)
	}

	t.summary = strings.Join(summaries, " • ")
}

// HasFindings returns true if there are findings to display.
func (t *Teaser) HasFindings() bool {
	return len(t.findings) > 0
}

// FindingsCount returns the number of findings.
func (t *Teaser) FindingsCount() int {
	return len(t.findings)
}

// GetTopFinding returns the highest relevance finding, if any.
func (t *Teaser) GetTopFinding() *research.Finding {
	if len(t.findings) == 0 {
		return nil
	}

	best := &t.findings[0]
	for i := 1; i < len(t.findings); i++ {
		if t.findings[i].Relevance > best.Relevance {
			best = &t.findings[i]
		}
	}
	return best
}

// View renders the teaser with fixed height.
func (t *Teaser) View() string {
	width := t.width
	if width <= 0 {
		width = 60
	}

	lines := make([]string, t.height)

	// Border style
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(pkgtui.ColorMuted).
		Width(width-2).
		Padding(0, 1)

	if !t.HasFindings() {
		// Empty state - show placeholder
		lines[0] = pkgtui.LabelStyle.Render("Research: running… [Ctrl+R]")
		for i := 1; i < t.height; i++ {
			lines[i] = ""
		}
	} else {
		// Header with count
		headerStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorSuccess).
			Bold(true)

		header := headerStyle.Render(fmt.Sprintf("★ %d findings", len(t.findings)))
		ctrlR := pkgtui.LabelStyle.Render(" [Ctrl+R for details]")
		lines[0] = header + ctrlR

		// Summary (wrapped to fit)
		if t.summary != "" {
			summaryStyle := pkgtui.LabelStyle.Width(width - 6)
			wrapped := summaryStyle.Render(t.summary)
			summaryLines := strings.Split(wrapped, "\n")
			for i, sl := range summaryLines {
				if i+1 < t.height {
					lines[i+1] = sl
				}
			}
		}

		// Top source
		if top := t.GetTopFinding(); top != nil && t.height >= 4 {
			sourceStyle := lipgloss.NewStyle().
				Foreground(pkgtui.ColorSecondary)
			source := fmt.Sprintf("via %s", top.SourceType)
			if len(source) > width-8 {
				source = source[:width-11] + "..."
			}
			lines[t.height-1] = sourceStyle.Render(source)
		}
	}

	// Ensure all lines exist
	for i := range lines {
		if lines[i] == "" {
			lines[i] = " "
		}
	}

	content := strings.Join(lines, "\n")
	return borderStyle.Render(content)
}

// ViewCompact renders a single-line teaser for narrow layouts.
func (t *Teaser) ViewCompact() string {
	if !t.HasFindings() {
		return pkgtui.LabelStyle.Render("Research: running…")
	}

	countStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorSuccess).
		Bold(true)

	return countStyle.Render(fmt.Sprintf("★ %d findings [Ctrl+R]", len(t.findings)))
}

// TeaserState represents the state type for a teaser.
type TeaserState int

const (
	TeaserStateLoading TeaserState = iota
	TeaserStateEmpty
	TeaserStateAvailable
	TeaserStateError
)

// State returns the current teaser state.
func (t *Teaser) State() TeaserState {
	if t.runID == "" {
		return TeaserStateLoading
	}
	if len(t.findings) == 0 {
		return TeaserStateEmpty
	}
	return TeaserStateAvailable
}
