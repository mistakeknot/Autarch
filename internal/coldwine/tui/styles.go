package tui

import (
	"strings"

	shared "github.com/mistakeknot/autarch/pkg/tui"
)

var (
	BaseStyle     = shared.BaseStyle
	PanelStyle    = shared.PanelStyle
	TitleStyle    = shared.TitleStyle
	SubtitleStyle = shared.SubtitleStyle
	LabelStyle    = shared.LabelStyle

	SelectedStyle   = shared.SelectedStyle
	UnselectedStyle = shared.UnselectedStyle

	HelpKeyStyle  = shared.HelpKeyStyle
	HelpDescStyle = shared.HelpDescStyle

	TabStyle       = shared.TabStyle
	ActiveTabStyle = shared.ActiveTabStyle

	StatusRunningStyle = shared.StatusRunning
	StatusWaitingStyle = shared.StatusWaiting
	StatusIdleStyle    = shared.StatusIdle
	StatusErrorStyle   = shared.StatusError

	// Use shared pane styles
	PaneFocusedStyle   = shared.PaneFocusedStyle
	PaneUnfocusedStyle = shared.PaneUnfocusedStyle
)

// StatusSymbol returns just the symbol for a status (re-exported from shared)
var StatusSymbol = shared.StatusSymbol

// Card styles for overlays
var (
	CardStyle        = shared.CardStyle
	CardFocusedStyle = shared.CardFocusedStyle
)

// keyDesc pairs a key binding with its description.
type keyDesc struct {
	Key  string
	Desc string
}

// renderKeyHelpLine renders styled key•desc pairs for the footer.
func renderKeyHelpLine(keys []keyDesc) string {
	parts := make([]string, len(keys))
	for i, kd := range keys {
		parts[i] = HelpKeyStyle.Render(kd.Key) + " " + HelpDescStyle.Render(kd.Desc)
	}
	return strings.Join(parts, HelpDescStyle.Render(" • "))
}

// renderOverlayCard wraps content in a CardStyle overlay with key help.
func renderOverlayCard(title, body string, keys []keyDesc) string {
	lines := []string{
		TitleStyle.Render(title),
		"",
		HelpDescStyle.Render(body),
		"",
		renderKeyHelpLine(keys),
	}
	return CardStyle.Copy().Width(60).Render(strings.Join(lines, "\n"))
}

// renderHelpOverlay renders the full help overlay matching Vauxhall pattern.
func renderHelpOverlay() string {
	lines := []string{
		TitleStyle.Render("Help"),
		"",
		HelpKeyStyle.Render("j/k") + HelpDescStyle.Render(" move") +
			HelpDescStyle.Render(" • ") +
			HelpKeyStyle.Render("tab") + HelpDescStyle.Render(" switch focus") +
			HelpDescStyle.Render(" • ") +
			HelpKeyStyle.Render("/") + HelpDescStyle.Render(" search"),
		HelpKeyStyle.Render("n") + HelpDescStyle.Render(" new task") +
			HelpDescStyle.Render(" • ") +
			HelpKeyStyle.Render("s") + HelpDescStyle.Render(" start") +
			HelpDescStyle.Render(" • ") +
			HelpKeyStyle.Render("x") + HelpDescStyle.Render(" stop"),
		HelpKeyStyle.Render("r") + HelpDescStyle.Render(" review") +
			HelpDescStyle.Render(" • ") +
			HelpKeyStyle.Render("R") + HelpDescStyle.Render(" review view") +
			HelpDescStyle.Render(" • ") +
			HelpKeyStyle.Render("c") + HelpDescStyle.Render(" coord"),
		HelpKeyStyle.Render("a/o/v/d") + HelpDescStyle.Render(" filter") +
			HelpDescStyle.Render(" • ") +
			HelpKeyStyle.Render("i") + HelpDescStyle.Render(" init") +
			HelpDescStyle.Render(" • ") +
			HelpKeyStyle.Render("?") + HelpDescStyle.Render(" help"),
		HelpKeyStyle.Render("ctrl+c") + HelpDescStyle.Render(" ×2 quit") +
			HelpDescStyle.Render(" • ") +
			HelpKeyStyle.Render("ctrl+k") + HelpDescStyle.Render(" palette") +
			HelpDescStyle.Render(" • ") +
			HelpKeyStyle.Render(",") + HelpDescStyle.Render(" settings"),
	}
	return CardStyle.Copy().Width(60).Render(strings.Join(lines, "\n"))
}
