package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

type SearchOverlay struct {
	input   textinput.Model
	results []specs.Summary
	cursor  int
	visible bool
	items   []specs.Summary
}

func NewSearchOverlay() *SearchOverlay {
	ti := textinput.New()
	ti.Placeholder = "Search PRDs..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 50
	return &SearchOverlay{input: ti, results: []specs.Summary{}}
}

func (s *SearchOverlay) SetItems(items []specs.Summary) {
	s.items = items
	s.updateResults()
}

func (s *SearchOverlay) Show() {
	s.visible = true
	s.input.Focus()
}

func (s *SearchOverlay) Hide() {
	s.visible = false
	s.input.Blur()
}

func (s *SearchOverlay) Visible() bool { return s.visible }

func (s *SearchOverlay) Results() []specs.Summary { return s.results }

func (s *SearchOverlay) Selected() *specs.Summary {
	if len(s.results) == 0 {
		return nil
	}
	if s.cursor >= len(s.results) {
		s.cursor = len(s.results) - 1
	}
	return &s.results[s.cursor]
}

func (s *SearchOverlay) Update(msg tea.Msg) (*SearchOverlay, tea.Cmd) {
	if !s.visible {
		return s, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			s.Hide()
			return s, nil
		case "enter":
			s.Hide()
			return s, nil
		case "up", "ctrl+k":
			if s.cursor > 0 {
				s.cursor--
			}
			return s, nil
		case "down", "ctrl+j":
			if s.cursor < len(s.results)-1 {
				s.cursor++
			}
			return s, nil
		default:
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			s.updateResults()
			return s, cmd
		}
	}
	return s, nil
}

func (s *SearchOverlay) View(width int) string {
	if !s.visible {
		return ""
	}
	boxStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	content := "Search: " + s.input.View()
	if width > 0 {
		boxStyle = boxStyle.Width(width)
	}
	return boxStyle.Render(content)
}

func (s *SearchOverlay) updateResults() {
	needle := strings.ToLower(strings.TrimSpace(s.input.Value()))
	if needle == "" {
		s.results = s.items
		s.cursor = 0
		return
	}
	out := make([]specs.Summary, 0, len(s.items))
	for _, item := range s.items {
		if strings.Contains(strings.ToLower(item.ID), needle) || strings.Contains(strings.ToLower(item.Title), needle) {
			out = append(out, item)
		}
	}
	s.results = out
	s.cursor = 0
}
