package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ChatSettingsPanel renders a small settings overlay for chat behavior.
type ChatSettingsPanel struct {
	Settings ChatSettings
	Index    int
}

// NewChatSettingsPanel creates a settings panel with current settings.
func NewChatSettingsPanel(settings ChatSettings) *ChatSettingsPanel {
	return &ChatSettingsPanel{Settings: settings}
}

// Update handles key input and returns whether settings changed.
func (p *ChatSettingsPanel) Update(msg tea.KeyMsg) bool {
	changed := false
	switch msg.String() {
	case "j", "down":
		if p.Index < 2 {
			p.Index++
		}
	case "k", "up":
		if p.Index > 0 {
			p.Index--
		}
	case " ", "enter":
		changed = p.toggleCurrent()
	}
	return changed
}

func (p *ChatSettingsPanel) toggleCurrent() bool {
	switch p.Index {
	case 0:
		p.Settings.AutoScroll = !p.Settings.AutoScroll
	case 1:
		p.Settings.ShowHistoryOnNewChat = !p.Settings.ShowHistoryOnNewChat
	case 2:
		p.Settings.GroupMessages = !p.Settings.GroupMessages
	default:
		return false
	}
	return true
}

// View renders the settings panel.
func (p *ChatSettingsPanel) View() string {
	items := []struct {
		label string
		value bool
	}{
		{label: "Auto-scroll", value: p.Settings.AutoScroll},
		{label: "Show history on new chat", value: p.Settings.ShowHistoryOnNewChat},
		{label: "Group messages", value: p.Settings.GroupMessages},
	}

	var lines []string
	titleStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	lines = append(lines, titleStyle.Render("Chat Settings"))
	lines = append(lines, "")

	for i, item := range items {
		marker := "○"
		if item.value {
			marker = "●"
		}
		line := fmt.Sprintf("%s %s", marker, item.label)
		if i == p.Index {
			line = SelectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	lines = append(lines, "")
	hintStyle := lipgloss.NewStyle().Foreground(ColorMuted).Italic(true)
	lines = append(lines, hintStyle.Render("j/k move  space toggle  esc close"))

	return strings.Join(lines, "\n")
}
