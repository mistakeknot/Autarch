package shell

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/vauxpraudemonium/pkg/tui"
)

// ToolTab represents a top-level tool tab
type ToolTab int

const (
	TabVauxhall ToolTab = iota
	TabPollard
	TabPraude
	TabTandemonium
)

func (t ToolTab) String() string {
	switch t {
	case TabVauxhall:
		return "Vauxhall"
	case TabPollard:
		return "Pollard"
	case TabPraude:
		return "Praude"
	case TabTandemonium:
		return "Tandemonium"
	default:
		return "Unknown"
	}
}

// Key returns the keyboard shortcut for the tab
func (t ToolTab) Key() string {
	switch t {
	case TabVauxhall:
		return "1"
	case TabPollard:
		return "2"
	case TabPraude:
		return "3"
	case TabTandemonium:
		return "4"
	default:
		return ""
	}
}

// TabCount returns the number of tool tabs
const TabCount = 4

// RenderTabBar renders the tool tab bar
func RenderTabBar(active ToolTab) string {
	tabs := make([]string, TabCount)
	for i := 0; i < TabCount; i++ {
		tab := ToolTab(i)
		style := tui.TabStyle
		if tab == active {
			style = tui.ActiveTabStyle
		}
		tabs[i] = style.Render(fmt.Sprintf("%s %s", tab.Key(), tab.String()))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, tabs...)
}

// RenderSubTabBar renders a tool's internal sub-tabs
func RenderSubTabBar(tabs []string, active int) string {
	if len(tabs) == 0 {
		return ""
	}

	rendered := make([]string, len(tabs))
	for i, tab := range tabs {
		style := tui.TabStyle
		if i == active {
			style = tui.ActiveTabStyle
		}
		rendered[i] = style.Render(tab)
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, rendered...)
}
