package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	sharedtui "github.com/mistakeknot/autarch/pkg/tui"
)

// Gurgeh nav tabs
type navTab int

const (
	navList navTab = iota
	navDetail
	navSprint
)

func (t navTab) String() string {
	switch t {
	case navList:
		return "PRDs"
	case navDetail:
		return "Detail"
	case navSprint:
		return "Sprint"
	default:
		return "Unknown"
	}
}

func renderHeader(title, focus string) string {
	appTitle := sharedtui.TitleStyle.Render("⚡ Gurgeh")

	// Nav pills matching Vauxhall tab bar
	tabs := []string{}
	for _, tab := range []navTab{navList, navDetail, navSprint} {
		style := sharedtui.TabStyle
		if strings.EqualFold(tab.String(), title) || strings.EqualFold(tab.String(), focus) {
			style = sharedtui.ActiveTabStyle
		}
		tabs = append(tabs, style.Render(tab.String()))
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Center, tabs...)

	return lipgloss.JoinHorizontal(lipgloss.Center,
		appTitle,
		strings.Repeat(" ", 4),
		tabBar,
	)
}

func renderFooter(keys, status string) string {
	if strings.TrimSpace(status) == "" {
		status = "ready"
	}
	// Parse key descriptions into styled key•desc pairs
	help := renderKeyHelp(keys)

	statusText := sharedtui.LabelStyle.Render(status)
	return lipgloss.JoinHorizontal(lipgloss.Center,
		help,
		"  ",
		statusText,
	)
}

func renderKeyHelp(keys string) string {
	// Split on double-space to get "key desc" pairs
	parts := strings.Fields(keys)
	var result []string
	for i := 0; i < len(parts)-1; i += 2 {
		k := sharedtui.HelpKeyStyle.Render(parts[i])
		d := sharedtui.HelpDescStyle.Render(parts[i+1])
		result = append(result, k+" "+d)
	}
	// Handle odd trailing word
	if len(parts)%2 == 1 {
		result = append(result, sharedtui.HelpKeyStyle.Render(parts[len(parts)-1]))
	}
	return strings.Join(result, sharedtui.HelpDescStyle.Render(" • "))
}

func renderPanelTitle(title string, width int) string {
	line := strings.Repeat("─", max(0, width))
	return sharedtui.TitleStyle.Render(title) + "\n" + sharedtui.LabelStyle.Render(line)
}

func renderComposerTitle(title string) string {
	return sharedtui.TitleStyle.Render(title)
}

