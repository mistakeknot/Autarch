package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// AgentOption represents a selectable agent option.
type AgentOption struct {
	Name   string
	Source string
}

// AgentSelectedMsg is emitted when the user selects an agent.
type AgentSelectedMsg struct {
	Name string
}

// AgentSelector renders a small selector line and handles selection keys.
type AgentSelector struct {
	Options      []AgentOption
	Open         bool
	Index        int
	openingIndex int
}

// NewAgentSelector creates a new selector with options.
func NewAgentSelector(opts []AgentOption) *AgentSelector {
	return &AgentSelector{Options: opts}
}

// Toggle opens the selector or cancels navigation back to the opening selection.
func (s *AgentSelector) Toggle() {
	if s.Open {
		s.Index = s.openingIndex
	} else {
		s.openingIndex = s.Index
	}
	s.Open = !s.Open
}

// Update handles key input for toggling and selecting agents.
// Responds to ctrl+g (MacBook-friendly) and F2 (external keyboard).
func (s *AgentSelector) Update(msg tea.KeyMsg) (tea.Msg, tea.Cmd) {
	switch msg.Type {
	case tea.KeyF2:
		s.Toggle()
		return nil, nil
	}
	// ctrl+g is more reliable on MacBooks where F-keys are bound to system functions
	if msg.String() == "ctrl+g" {
		s.Toggle()
		return nil, nil
	}
	if !s.Open {
		return nil, nil
	}

	switch msg.String() {
	case "esc":
		s.Toggle()
	case "up":
		if s.Index > 0 {
			s.Index--
		}
	case "down":
		if s.Index < len(s.Options)-1 {
			s.Index++
		}
	case "enter":
		if len(s.Options) > 0 {
			opt := s.Options[s.Index]
			s.Index = s.openingIndex
			s.Open = false
			return AgentSelectedMsg{Name: opt.Name}, nil
		}
	}

	return nil, nil
}

// View renders the selector line.
// An optional width limits the selector to the caller's allocated columns.
func (s *AgentSelector) View(width ...int) string {
	if len(s.Options) == 0 {
		return ""
	}
	if !s.Open {
		name := s.currentLabel()
		label := fmt.Sprintf("Dispatch: %s (ctrl+g)", name)
		label = lipgloss.NewStyle().Foreground(ColorMuted).Render(label)
		if len(width) > 0 {
			label = ansi.Truncate(label, max(0, width[0]), "…")
		}
		return label
	}

	var parts []string
	for i, opt := range s.Options {
		label := fmt.Sprintf("  %s", opt.Name)
		if i == s.Index {
			label = SelectedStyle.Render("> " + opt.Name)
		} else {
			label = UnselectedStyle.Render(label)
		}
		parts = append(parts, label)
	}

	label := lipgloss.NewStyle().Foreground(ColorFgDim).Render("Dispatch: ") + strings.Join(parts, "  ")
	if len(width) > 0 && lipgloss.Width(label) > max(0, width[0]) {
		// Prioritize the highlighted option over the prefix and other options.
		// Omitting the ellipsis preserves the highlight even in a single column.
		for i, part := range parts {
			if i == s.Index {
				return ansi.Truncate(part, max(0, width[0]), "")
			}
		}
		label = ansi.Truncate(label, max(0, width[0]), "…")
	}
	return label
}

func (s *AgentSelector) currentName() string {
	if len(s.Options) == 0 {
		return "Unknown"
	}
	if s.Index < 0 || s.Index >= len(s.Options) {
		return s.Options[0].Name
	}
	return s.Options[s.Index].Name
}

func (s *AgentSelector) currentLabel() string {
	name := s.currentName()
	if s.Index >= 0 && s.Index < len(s.Options) && s.Options[s.Index].Source != "" {
		name += " · " + s.Options[s.Index].Source
	}
	return name
}
