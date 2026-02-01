package views

import (
	"fmt"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// SprintDocPanel wraps DocPanel with sprint-specific content rendering.
type SprintDocPanel struct {
	panel       *pkgtui.DocPanel
	showDetails bool // toggle for evidence/quality/consistency details
}

// NewSprintDocPanel creates a new sprint doc panel.
func NewSprintDocPanel() *SprintDocPanel {
	panel := pkgtui.NewDocPanel()
	panel.SetTitle("Spec Draft")
	return &SprintDocPanel{panel: panel}
}

// SetSize sets the panel dimensions.
func (d *SprintDocPanel) SetSize(width, height int) {
	d.panel.SetSize(width, height)
}

// ToggleDetails toggles the details section visibility.
func (d *SprintDocPanel) ToggleDetails() {
	d.showDetails = !d.showDetails
}

// ScrollUp scrolls the panel up.
func (d *SprintDocPanel) ScrollUp() { d.panel.ScrollUp() }

// ScrollDown scrolls the panel down.
func (d *SprintDocPanel) ScrollDown() { d.panel.ScrollDown() }

// Update refreshes the panel content from sprint state.
func (d *SprintDocPanel) Update(state *arbiter.SprintState) {
	d.panel.ClearSections()

	if state == nil {
		d.panel.SetSubtitle("No active sprint")
		return
	}

	d.panel.SetSubtitle(fmt.Sprintf("Phase: %s", phaseLabel(state.Phase)))

	section, ok := state.Sections[state.Phase]
	if !ok || section == nil {
		d.panel.AddSection(pkgtui.DocSection{
			Title:   "Draft",
			Content: "(no draft yet)",
		})
		return
	}

	// Main draft content
	d.panel.AddSection(pkgtui.DocSection{
		Title:   phaseLabel(state.Phase) + " Draft",
		Content: section.Content,
	})

	// Status indicator
	statusText := "Proposed"
	switch section.Status {
	case arbiter.DraftAccepted:
		statusText = "✓ Accepted"
	case arbiter.DraftNeedsRevision:
		statusText = "⚠ Needs Revision"
	case arbiter.DraftPending:
		statusText = "○ Pending"
	}
	d.panel.AddSection(pkgtui.DocSection{
		Title:   "Status",
		Content: statusText,
	})

	// Options
	if len(section.Options) > 0 {
		var opts string
		for i, opt := range section.Options {
			opts += fmt.Sprintf("%d. %s\n", i+1, opt)
		}
		d.panel.AddSection(pkgtui.DocSection{
			Title:   "Options",
			Content: opts,
		})
	}

	// Collapsible details
	if d.showDetails {
		// Confidence scores
		conf := state.Confidence
		d.panel.AddSection(pkgtui.DocSection{
			Title: "Confidence",
			Content: fmt.Sprintf(
				"Completeness: %.0f%%  Consistency: %.0f%%\nSpecificity: %.0f%%  Research: %.0f%%",
				conf.Completeness*100, conf.Consistency*100,
				conf.Specificity*100, conf.Research*100,
			),
		})

		// Conflicts
		if len(state.Conflicts) > 0 {
			var conflictText string
			for _, c := range state.Conflicts {
				conflictText += fmt.Sprintf("• [%v] %s\n", c.Severity, c.Message)
			}
			d.panel.AddSection(pkgtui.DocSection{
				Title:   "Conflicts",
				Content: conflictText,
			})
		}
	}
}

// View renders the panel.
func (d *SprintDocPanel) View() string {
	return d.panel.View()
}
