package views

import (
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// PhaseSidebar generates sidebar items from sprint state.
type PhaseSidebar struct{}

// NewPhaseSidebar creates a new PhaseSidebar.
func NewPhaseSidebar() *PhaseSidebar {
	return &PhaseSidebar{}
}

// Items returns sidebar items reflecting the current sprint state.
func (s *PhaseSidebar) Items(state *arbiter.SprintState) []pkgtui.SidebarItem {
	if state == nil {
		return defaultPhaseItems()
	}

	var items []pkgtui.SidebarItem
	for _, phase := range arbiter.AllPhases() {
		icon := "○" // pending
		if phase == state.Phase {
			icon = "●" // current
		} else if section, ok := state.Sections[phase]; ok {
			switch section.Status {
			case arbiter.DraftAccepted:
				icon = "✓"
			case arbiter.DraftNeedsRevision:
				icon = "⚠"
			}
		}

		// Check for conflicts on this phase
		for _, c := range state.Conflicts {
			for _, cp := range c.Sections {
				if cp == phase {
					icon = "⚠"
					break
				}
			}
		}

		items = append(items, pkgtui.SidebarItem{
			ID:    phase.String(),
			Label: phaseLabel(phase),
			Icon:  icon,
		})
	}
	return items
}

func phaseLabel(p arbiter.Phase) string {
	switch p {
	case arbiter.PhaseVision:
		return "Vision"
	case arbiter.PhaseProblem:
		return "Problem"
	case arbiter.PhaseUsers:
		return "Users"
	case arbiter.PhaseFeaturesGoals:
		return "Features"
	case arbiter.PhaseRequirements:
		return "Requirements"
	case arbiter.PhaseScopeAssumptions:
		return "Scope"
	case arbiter.PhaseCUJs:
		return "CUJs"
	case arbiter.PhaseAcceptanceCriteria:
		return "Acceptance"
	default:
		return p.String()
	}
}

func defaultPhaseItems() []pkgtui.SidebarItem {
	var items []pkgtui.SidebarItem
	for _, phase := range arbiter.AllPhases() {
		items = append(items, pkgtui.SidebarItem{
			ID:    phase.String(),
			Label: phaseLabel(phase),
			Icon:  "○",
		})
	}
	return items
}
