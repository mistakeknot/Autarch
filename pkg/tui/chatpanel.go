package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ChatMessage represents a single message in the chat history.
type ChatMessage struct {
	Role    string // "user", "agent", "system"
	Content string
}

// ChatPanel combines a scrollable chat history with a composer at the bottom.
// This is the right-side panel in the Cursor-style split layout.
type ChatPanel struct {
	messages []ChatMessage
	composer *Composer
	width    int
	height   int
	scroll   int // Scroll offset for history (0 = bottom)
}

// NewChatPanel creates a new chat panel with default settings.
func NewChatPanel() *ChatPanel {
	composer := NewComposer(4)
	return &ChatPanel{
		messages: []ChatMessage{},
		composer: composer,
	}
}

// AddMessage adds a message to the chat history.
func (p *ChatPanel) AddMessage(role, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	p.messages = append(p.messages, ChatMessage{
		Role:    role,
		Content: content,
	})
	// Auto-scroll to bottom when new message added
	p.scroll = 0
}

// ClearMessages removes all messages from the chat history.
func (p *ChatPanel) ClearMessages() {
	p.messages = nil
	p.scroll = 0
}

// Messages returns a copy of all messages.
func (p *ChatPanel) Messages() []ChatMessage {
	result := make([]ChatMessage, len(p.messages))
	copy(result, p.messages)
	return result
}

// SetSize sets the dimensions of the chat panel.
func (p *ChatPanel) SetSize(width, height int) {
	p.width = width
	p.height = height

	// Composer gets fixed height at bottom
	composerHeight := 8 // 4 lines content + borders/decorations
	p.composer.SetSize(width, composerHeight)
}

// Update handles tea.Msg for the chat panel.
func (p *ChatPanel) Update(msg tea.Msg) (*ChatPanel, tea.Cmd) {
	// Pass messages to composer
	var cmd tea.Cmd
	p.composer, cmd = p.composer.Update(msg)
	return p, cmd
}

// View renders the complete chat panel (history + composer).
func (p *ChatPanel) View() string {
	if p.height <= 0 || p.width <= 0 {
		return ""
	}

	// Calculate heights
	composerHeight := 8 // Fixed height for composer area
	historyHeight := p.height - composerHeight - 1 // -1 for separator
	if historyHeight < 1 {
		historyHeight = 1
	}

	// Render history
	historyView := p.renderHistory(historyHeight)

	// Simple separator - don't try to match exact width, let SplitLayout handle padding
	separatorStyle := lipgloss.NewStyle().
		Foreground(ColorMuted)
	separator := separatorStyle.Render(strings.Repeat("─", 40))

	// Render composer
	composerView := p.composer.View()

	// Join vertically - don't add Width constraints here
	// The SplitLayout.ensureSize() handles width normalization
	return lipgloss.JoinVertical(lipgloss.Left,
		historyView,
		separator,
		composerView,
	)
}

// renderHistory renders the chat history area.
func (p *ChatPanel) renderHistory(height int) string {
	if height <= 0 {
		return ""
	}

	if len(p.messages) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)
		// Don't use ensureHeight here - SplitLayout.ensureSize handles height normalization
		return emptyStyle.Render("No messages yet.")
	}

	// Build message lines
	var lines []string
	for _, msg := range p.messages {
		// Role header
		roleStyle := p.roleStyle(msg.Role)
		lines = append(lines, roleStyle.Render(formatRole(msg.Role)+":"))

		// Content with indent
		contentStyle := lipgloss.NewStyle().
			Foreground(ColorFg).
			PaddingLeft(2)

		// Wrap content to fit width
		contentWidth := p.width - 4
		if contentWidth < 10 {
			contentWidth = 10
		}
		wrapped := wrapText(msg.Content, contentWidth)
		for _, line := range strings.Split(wrapped, "\n") {
			lines = append(lines, contentStyle.Render(line))
		}
		lines = append(lines, "") // Blank line between messages
	}

	// Apply scrolling - show most recent messages that fit
	if len(lines) > height {
		start := len(lines) - height - p.scroll
		if start < 0 {
			start = 0
		}
		end := start + height
		if end > len(lines) {
			end = len(lines)
			start = end - height
			if start < 0 {
				start = 0
			}
		}
		lines = lines[start:end]
	}

	// Don't use ensureHeight - SplitLayout.ensureSize handles height normalization
	return strings.Join(lines, "\n")
}

// roleStyle returns the style for a given role.
func (p *ChatPanel) roleStyle(role string) lipgloss.Style {
	switch strings.ToLower(role) {
	case "user":
		return lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)
	case "agent":
		return lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)
	case "system":
		return lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)
	default:
		return lipgloss.NewStyle().
			Foreground(ColorFg)
	}
}

// formatRole formats the role name for display.
func formatRole(role string) string {
	r := strings.TrimSpace(role)
	if r == "" {
		return "Agent"
	}
	if len(r) == 1 {
		return strings.ToUpper(r)
	}
	return strings.ToUpper(r[:1]) + r[1:]
}

// Value returns the current composer text.
func (p *ChatPanel) Value() string {
	return p.composer.Value()
}

// SetValue sets the composer text.
func (p *ChatPanel) SetValue(s string) {
	p.composer.SetValue(s)
}

// ClearComposer clears the composer input.
func (p *ChatPanel) ClearComposer() {
	p.composer.Reset()
}

// Focus focuses the composer.
func (p *ChatPanel) Focus() tea.Cmd {
	return p.composer.Focus()
}

// Blur blurs the composer.
func (p *ChatPanel) Blur() {
	p.composer.Blur()
}

// Focused returns whether the composer is focused.
func (p *ChatPanel) Focused() bool {
	return p.composer.Focused()
}

// SetComposerTitle sets the title for the composer.
func (p *ChatPanel) SetComposerTitle(title string) {
	p.composer.SetTitle(title)
}

// SetComposerHint sets the keyboard hint for the composer.
func (p *ChatPanel) SetComposerHint(hint string) {
	p.composer.SetHint(hint)
}

// SetComposerPlaceholder sets the placeholder text for the composer.
func (p *ChatPanel) SetComposerPlaceholder(placeholder string) {
	p.composer.SetPlaceholder(placeholder)
}

// ScrollUp scrolls the history up (shows older messages).
func (p *ChatPanel) ScrollUp() {
	p.scroll++
}

// ScrollDown scrolls the history down (shows newer messages).
func (p *ChatPanel) ScrollDown() {
	if p.scroll > 0 {
		p.scroll--
	}
}

// ScrollToBottom scrolls to the most recent messages.
func (p *ChatPanel) ScrollToBottom() {
	p.scroll = 0
}

// ensureHeight pads or truncates content to exactly n lines.
func ensureHeight(content string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// wrapText wraps text to the specified width.
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result []string
	for _, line := range strings.Split(text, "\n") {
		if len(line) <= width {
			result = append(result, line)
			continue
		}

		// Simple word wrap
		words := strings.Fields(line)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}

		var current string
		for _, word := range words {
			if current == "" {
				current = word
			} else if len(current)+1+len(word) <= width {
				current += " " + word
			} else {
				result = append(result, current)
				current = word
			}
		}
		if current != "" {
			result = append(result, current)
		}
	}
	return strings.Join(result, "\n")
}
