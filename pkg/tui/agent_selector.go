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
func (s *AgentSelector) Update(msg tea.KeyMsg) (tea.Msg, tea.Cmd) {
	switch msg.Type {
	case tea.KeyF2:
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
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(msg.String()[0] - '1')
		if idx >= 0 && idx < len(s.Options) {
			s.Index = idx
			opt := s.Options[idx]
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
		label := fmt.Sprintf("Model: %s (F2)", s.currentName())
		return lipgloss.NewStyle().Foreground(ColorMuted).Render(label)
	}

	var parts []string
	for i, opt := range s.Options {
		label := fmt.Sprintf("[%d] %s", i+1, opt.Name)
		if i == s.Index {
			label = SelectedStyle.Render(label)
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
