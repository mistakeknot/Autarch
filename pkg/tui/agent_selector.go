package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	Options []AgentOption
	Open    bool
	Index   int
}

// NewAgentSelector creates a new selector with options.
func NewAgentSelector(opts []AgentOption) *AgentSelector {
	return &AgentSelector{Options: opts}
}

// Update handles key input for toggling and selecting agents.
// Responds to ctrl+g (MacBook-friendly) and F2 (external keyboard).
func (s *AgentSelector) Update(msg tea.KeyMsg) (tea.Msg, tea.Cmd) {
	switch msg.Type {
	case tea.KeyF2:
		s.Open = !s.Open
		return nil, nil
	}
	// ctrl+g is more reliable on MacBooks where F-keys are bound to system functions
	if msg.String() == "ctrl+g" {
		s.Open = !s.Open
		return nil, nil
	}
	if !s.Open {
		return nil, nil
	}

	switch msg.String() {
	case "esc":
		s.Open = false
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
			s.Open = false
			return AgentSelectedMsg{Name: opt.Name}, nil
		}
	}

	return nil, nil
}

// View renders the selector line.
func (s *AgentSelector) View() string {
	if len(s.Options) == 0 {
		return ""
	}
	if !s.Open {
		label := fmt.Sprintf("Model: %s (ctrl+g)", s.currentName())
		return lipgloss.NewStyle().Foreground(ColorMuted).Render(label)
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

	return lipgloss.NewStyle().Foreground(ColorFgDim).Render("Model: ") + strings.Join(parts, "  ")
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
