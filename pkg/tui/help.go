package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// HelpBindingFromKey converts a key.Binding to a HelpBinding
// (the HelpBinding type is defined in view.go).
func HelpBindingFromKey(k key.Binding) HelpBinding {
	h := k.Help()
	return HelpBinding{Key: h.Key, Description: h.Desc}
}

// HelpOverlay renders a help panel from CommonKeys plus any extra bindings.
// width is the available terminal width (used for centering/padding).
type HelpOverlay struct {
	Visible bool
}

// NewHelpOverlay returns a HelpOverlay in its default (hidden) state.
func NewHelpOverlay() HelpOverlay {
	return HelpOverlay{}
}

// Toggle flips the overlay visibility.
func (h *HelpOverlay) Toggle() {
	h.Visible = !h.Visible
}

// commonBindings returns the standard bindings from CommonKeys as HelpBindings.
func commonBindings(keys CommonKeys) []HelpBinding {
	bindings := []HelpBinding{
		HelpBindingFromKey(keys.Quit),
		HelpBindingFromKey(keys.Help),
		HelpBindingFromKey(keys.Search),
		HelpBindingFromKey(keys.Back),
		HelpBindingFromKey(keys.NavUp),
		HelpBindingFromKey(keys.NavDown),
		HelpBindingFromKey(keys.Top),
		HelpBindingFromKey(keys.Bottom),
		HelpBindingFromKey(keys.Next),
		HelpBindingFromKey(keys.Prev),
		HelpBindingFromKey(keys.Refresh),
		HelpBindingFromKey(keys.TabCycle),
		HelpBindingFromKey(keys.Select),
		HelpBindingFromKey(keys.Toggle),
	}
	if len(keys.Sections) > 0 {
		bindings = append(bindings, HelpBinding{Key: "1-9", Description: "sections"})
	}
	return bindings
}

// Render produces the help overlay string. extras are appended after the
// common bindings under a "Tool" heading.
func (h HelpOverlay) Render(keys CommonKeys, extras []HelpBinding, width int) string {
	if !h.Visible {
		return ""
	}

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		MarginBottom(1)

	b.WriteString(titleStyle.Render("Keyboard Shortcuts"))
	b.WriteString("\n\n")

	bindings := commonBindings(keys)
	writeBindings(&b, bindings)

	if len(extras) > 0 {
		sectionStyle := lipgloss.NewStyle().
			Foreground(ColorFg).
			Bold(true).
			MarginTop(1)
		b.WriteString("\n")
		b.WriteString(sectionStyle.Render("Tool"))
		b.WriteString("\n")
		writeBindings(&b, extras)
	}

	overlay := lipgloss.NewStyle().
		Background(ColorBgDark).
		Foreground(ColorFg).
		Padding(1, 3).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Width(min(50, width-4))

	return overlay.Render(b.String())
}

func writeBindings(b *strings.Builder, bindings []HelpBinding) {
	for _, bind := range bindings {
		b.WriteString(HelpKeyStyle.Render(bind.Key))
		b.WriteString("  ")
		b.WriteString(HelpDescStyle.Render(bind.Description))
		b.WriteString("\n")
	}
}
