package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type ListScreen struct{}

func (s *ListScreen) Update(msg tea.Msg, state *SharedState) (Screen, Intent) {
	return s, Intent{}
}

func (s *ListScreen) View(state *SharedState) string {
	return joinLines(renderList(state))
}

func (s *ListScreen) Title() string {
	return "LIST"
}

func renderList(state *SharedState) []string {
	lines := []string{"PRDs"}
	if state == nil {
		return lines
	}
	items := filterSummaries(state.Summaries, state.Filter)
	if len(items) == 0 {
		return append(lines, "No PRDs yet.")
	}
	for i, s := range items {
		prefix := "  "
		if i == state.Selected {
			prefix = "> "
		}
		lines = append(lines, prefix+s.ID+" "+s.Title)
	}
	return lines
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}
