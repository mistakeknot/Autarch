package tui

import (
	"strings"

	sharedtui "github.com/mistakeknot/autarch/pkg/tui"
)

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
