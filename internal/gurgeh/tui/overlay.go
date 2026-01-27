package tui

import (
	"strings"

	sharedtui "github.com/mistakeknot/autarch/pkg/tui"
)

func renderHelpOverlay() string {
	lines := []string{
		sharedtui.TitleStyle.Render("Help"),
		"",
		sharedtui.HelpKeyStyle.Render("j/k") + sharedtui.HelpDescStyle.Render(" move") +
			sharedtui.HelpDescStyle.Render(" • ") +
			sharedtui.HelpKeyStyle.Render("enter") + sharedtui.HelpDescStyle.Render(" toggle group") +
			sharedtui.HelpDescStyle.Render(" • ") +
			sharedtui.HelpKeyStyle.Render("/") + sharedtui.HelpDescStyle.Render(" search"),
		sharedtui.HelpKeyStyle.Render("n") + sharedtui.HelpDescStyle.Render(" new sprint") +
			sharedtui.HelpDescStyle.Render(" • ") +
			sharedtui.HelpKeyStyle.Render("g") + sharedtui.HelpDescStyle.Render(" sprint from PRD"),
		sharedtui.HelpKeyStyle.Render("r") + sharedtui.HelpDescStyle.Render(" research") +
			sharedtui.HelpDescStyle.Render(" • ") +
			sharedtui.HelpKeyStyle.Render("p") + sharedtui.HelpDescStyle.Render(" suggestions") +
			sharedtui.HelpDescStyle.Render(" • ") +
			sharedtui.HelpKeyStyle.Render("s") + sharedtui.HelpDescStyle.Render(" review"),
		sharedtui.HelpKeyStyle.Render("a") + sharedtui.HelpDescStyle.Render(" archive") +
			sharedtui.HelpDescStyle.Render(" • ") +
			sharedtui.HelpKeyStyle.Render("d") + sharedtui.HelpDescStyle.Render(" delete") +
			sharedtui.HelpDescStyle.Render(" • ") +
			sharedtui.HelpKeyStyle.Render("u") + sharedtui.HelpDescStyle.Render(" undo") +
			sharedtui.HelpDescStyle.Render(" • ") +
			sharedtui.HelpKeyStyle.Render("h") + sharedtui.HelpDescStyle.Render(" archived"),
		sharedtui.HelpKeyStyle.Render("tab") + sharedtui.HelpDescStyle.Render(" switch focus") +
			sharedtui.HelpDescStyle.Render(" • ") +
			sharedtui.HelpKeyStyle.Render("?") + sharedtui.HelpDescStyle.Render(" help") +
			sharedtui.HelpDescStyle.Render(" • ") +
			sharedtui.HelpKeyStyle.Render("q") + sharedtui.HelpDescStyle.Render(" quit"),
	}
	return sharedtui.CardStyle.Copy().Width(60).Render(strings.Join(lines, "\n"))
}

func renderTutorialOverlay() string {
	lines := []string{
		sharedtui.TitleStyle.Render("Tutorial"),
		"",
		sharedtui.HelpDescStyle.Render("1. Navigate PRDs with j/k"),
		sharedtui.HelpDescStyle.Render("2. Press enter to expand/collapse groups"),
		sharedtui.HelpDescStyle.Render("3. Press tab to switch list ↔ detail"),
		sharedtui.HelpDescStyle.Render("4. Press n for new sprint, g for existing PRD"),
		sharedtui.HelpDescStyle.Render("5. Press r to launch research"),
		sharedtui.HelpDescStyle.Render("6. Press ? for keyboard shortcuts"),
	}
	return sharedtui.CardStyle.Copy().Width(60).Render(strings.Join(lines, "\n"))
}

func renderConfirmOverlay(message string) string {
	lines := []string{
		sharedtui.TitleStyle.Render("⚠  Confirm"),
		"",
		sharedtui.HelpDescStyle.Render(message),
		"",
		sharedtui.HelpKeyStyle.Render("enter") + sharedtui.HelpDescStyle.Render(" confirm") +
			sharedtui.HelpDescStyle.Render(" • ") +
			sharedtui.HelpKeyStyle.Render("esc") + sharedtui.HelpDescStyle.Render(" cancel"),
	}
	return sharedtui.CardFocusedStyle.Copy().Width(50).Render(strings.Join(lines, "\n"))
}
