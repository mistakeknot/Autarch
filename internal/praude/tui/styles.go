package tui

import (
	"strings"

	sharedtui "github.com/mistakeknot/vauxpraudemonium/pkg/tui"
)

func renderHeader(title, focus string) string {
	label := "PRAUDE | " + title + " | [" + focus + "]"
	return sharedtui.TitleStyle.Render(label)
}

func renderFooter(keys, status string) string {
	if strings.TrimSpace(status) == "" {
		status = "ready"
	}
	label := "KEYS: " + keys + " | " + status
	return sharedtui.HelpDescStyle.Render(label)
}

func renderPanelTitle(title string, width int) string {
	line := strings.Repeat("─", max(0, width))
	return sharedtui.TitleStyle.Render(title) + "\n" + sharedtui.LabelStyle.Render(line)
}

func renderComposerTitle(title string) string {
	return sharedtui.TitleStyle.Render(title)
}
