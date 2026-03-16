package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/Masaq/breadcrumb"
	"github.com/mistakeknot/Masaq/theme"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// BreadcrumbStep represents a step in the breadcrumb
type BreadcrumbStep struct {
	ID       string
	Label    string
	State    OnboardingState
	Unlocked bool // Whether this step has been reached
}

// Breadcrumb displays navigable steps in the onboarding flow.
// Uses masaq/breadcrumb for base rendering and adds interactive
// keyboard navigation on top.
type Breadcrumb struct {
	steps    []BreadcrumbStep
	current  int
	selected int // For keyboard navigation (-1 means not navigating)
	width    int
	keys     pkgtui.CommonKeys
	crumbs   breadcrumb.Model // Masaq breadcrumb for rendering
}

// NewBreadcrumb creates a new breadcrumb with the onboarding steps
func NewBreadcrumb() *Breadcrumb {
	// Derive steps from OnboardingState enum - single source of truth
	states := AllOnboardingStates()
	steps := make([]BreadcrumbStep, len(states))
	for i, state := range states {
		steps[i] = BreadcrumbStep{
			ID:       state.ID(),
			Label:    state.Label(),
			State:    state,
			Unlocked: i == 0, // Only first step unlocked initially
		}
	}

	bc := breadcrumb.New(80)
	b := &Breadcrumb{
		steps:    steps,
		current:  0,
		selected: -1,
		keys:     pkgtui.NewCommonKeys(),
		crumbs:   bc,
	}
	b.syncMasaq()
	return b
}

// LabelsForTest exposes breadcrumb labels for tests.
func (b *Breadcrumb) LabelsForTest() []string {
	labels := make([]string, 0, len(b.steps))
	for _, step := range b.steps {
		labels = append(labels, step.Label)
	}
	return labels
}

// SetWidth sets the available width
func (b *Breadcrumb) SetWidth(w int) {
	b.width = w
	b.crumbs = breadcrumb.New(w)
	b.syncMasaq()
}

// SetCurrent sets the current step and unlocks all steps up to it
func (b *Breadcrumb) SetCurrent(state OnboardingState) {
	for i, step := range b.steps {
		if step.State == state {
			b.current = i
			// Unlock all steps up to and including current
			for j := 0; j <= i; j++ {
				b.steps[j].Unlocked = true
			}
			break
		}
	}
	b.selected = -1 // Reset selection when changing current
	b.syncMasaq()
}

// StartNavigation enables keyboard navigation mode
func (b *Breadcrumb) StartNavigation() {
	b.selected = b.current
}

// StopNavigation disables keyboard navigation mode
func (b *Breadcrumb) StopNavigation() {
	b.selected = -1
}

// IsNavigating returns true if in navigation mode
func (b *Breadcrumb) IsNavigating() bool {
	return b.selected >= 0
}

// Update handles keyboard navigation
func (b *Breadcrumb) Update(msg tea.Msg) (*Breadcrumb, tea.Cmd) {
	if !b.IsNavigating() {
		return b, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "left":
			// Find previous unlocked step
			for i := b.selected - 1; i >= 0; i-- {
				if b.steps[i].Unlocked {
					b.selected = i
					break
				}
			}
		case msg.String() == "right":
			// Find next unlocked step
			for i := b.selected + 1; i < len(b.steps); i++ {
				if b.steps[i].Unlocked {
					b.selected = i
					break
				}
			}
		case key.Matches(msg, b.keys.Select):
			if b.selected >= 0 && b.selected < len(b.steps) && b.steps[b.selected].Unlocked {
				targetState := b.steps[b.selected].State
				b.selected = -1
				return b, func() tea.Msg {
					return NavigateToStepMsg{State: targetState}
				}
			}
		case key.Matches(msg, b.keys.Back):
			b.selected = -1
		}
	}

	return b, nil
}

// View renders the breadcrumb. When not navigating, delegates to the Masaq
// breadcrumb renderer. When navigating, renders with selection highlights.
func (b *Breadcrumb) View() string {
	if !b.IsNavigating() {
		return b.crumbs.View()
	}

	// Navigation mode: render with selection overlay
	sem := theme.Current().Semantic()
	var parts []string

	separatorStyle := lipgloss.NewStyle().Foreground(sem.FgDim.Color()).Padding(0, 1)
	separator := separatorStyle.Render("→")

	for i, step := range b.steps {
		var style lipgloss.Style

		if i == b.current {
			style = lipgloss.NewStyle().
				Foreground(sem.Primary.Color()).
				Bold(true).
				Padding(0, 1)
		} else if step.Unlocked {
			style = lipgloss.NewStyle().
				Foreground(sem.FgDim.Color()).
				Padding(0, 1)
		} else {
			style = lipgloss.NewStyle().
				Foreground(sem.Muted.Color()).
				Padding(0, 1)
		}

		// Selection indicator
		if b.selected == i {
			style = style.Underline(true)
			if step.Unlocked && i != b.current {
				style = style.Background(sem.BgLight.Color())
			}
		}

		parts = append(parts, style.Render(step.Label))
		if i < len(b.steps)-1 {
			parts = append(parts, separator)
		}
	}

	return strings.Join(parts, "")
}

// syncMasaq updates the Masaq breadcrumb model to reflect current state.
func (b *Breadcrumb) syncMasaq() {
	masaqSteps := make([]breadcrumb.Step, len(b.steps))
	for i, step := range b.steps {
		var status breadcrumb.Status
		switch {
		case i < b.current && step.Unlocked:
			status = breadcrumb.Done
		case i == b.current:
			status = breadcrumb.Active
		default:
			status = breadcrumb.Pending
		}
		masaqSteps[i] = breadcrumb.Step{Label: step.Label, Status: status}
	}
	b.crumbs.SetSteps(masaqSteps)
}

// NavigateToStepMsg is sent when user navigates to a step via breadcrumb
type NavigateToStepMsg struct {
	State OnboardingState
}
