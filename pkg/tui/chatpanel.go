package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ParseSlashCommand checks if the text starts with '/' and parses it as a slash command.
// Returns (command, args, true) if it's a slash command, or ("", nil, false) otherwise.
func ParseSlashCommand(text string) (string, []string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", nil, false
	}
	// Remove the leading slash and split into parts
	parts := strings.Fields(text[1:])
	if len(parts) == 0 {
		return "", nil, false
	}
	return strings.ToLower(parts[0]), parts[1:], true
}

// ChatMessage represents a single message in the chat history.
type ChatMessage struct {
	Role    string // "user", "agent", "system"
	Content string
}

// ChatPanel combines a scrollable chat history with a composer at the bottom.
// This is the right-side panel in the Cursor-style split layout.
type ChatPanel struct {
	messages      []ChatMessage
	composer      *Composer
	selector      *AgentSelector
	commandPicker *CommandPicker
	settings      ChatSettings
	width         int
	height        int
	scroll        int // Scroll offset for history (0 = bottom)
}

// NewChatPanel creates a new chat panel with default settings.
func NewChatPanel() *ChatPanel {
	composer := NewComposer(4)
	picker := NewCommandPicker(GlobalCommands())
	return &ChatPanel{
		messages:      []ChatMessage{},
		composer:      composer,
		commandPicker: picker,
		settings:      DefaultChatSettings(),
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
	if p.settings.AutoScroll {
		p.scroll = 0
	}
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
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		// Handle command picker first if visible
		if p.commandPicker != nil && p.commandPicker.Visible() {
			selectedCmd, consumed := p.commandPicker.Update(keyMsg)
			if selectedCmd != "" {
				// User selected a command - put it in the composer
				p.composer.SetValue("/" + selectedCmd + " ")
				return p, nil
			}
			if consumed {
				return p, nil
			}
		}

		// Handle agent selector
		if p.selector != nil {
			wasOpen := p.selector.Open
			selectorMsg, selectorCmd := p.selector.Update(keyMsg)
			if selectorMsg != nil {
				return p, tea.Batch(selectorCmd, func() tea.Msg { return selectorMsg })
			}
			if p.selector.Open || wasOpen || keyMsg.Type == tea.KeyF2 {
				return p, selectorCmd
			}
		}
	}

	// Pass messages to composer
	var cmd tea.Cmd
	p.composer, cmd = p.composer.Update(msg)

	// Check if we should show/update the command picker
	p.updateCommandPicker()

	return p, cmd
}

// updateCommandPicker shows or hides the picker based on composer content.
func (p *ChatPanel) updateCommandPicker() {
	if p.commandPicker == nil {
		return
	}

	value := p.composer.Value()

	// Show picker when typing starts with /
	if strings.HasPrefix(value, "/") {
		query := strings.TrimPrefix(value, "/")
		// Only show if we're still typing the command (no space yet, or query is short)
		if !strings.Contains(query, " ") || len(query) < 20 {
			if !p.commandPicker.Visible() {
				p.commandPicker.Show(query)
			} else {
				p.commandPicker.UpdateQuery(query)
			}
		} else {
			p.commandPicker.Hide()
		}
	} else {
		p.commandPicker.Hide()
	}
}

// View renders the complete chat panel (history + composer).
func (p *ChatPanel) View() string {
	if p.height <= 0 || p.width <= 0 {
		return ""
	}

	selectorHeight := 0
	if p.selector != nil {
		selectorHeight = 1
	}

	// Calculate heights
	composerHeight := 8                                             // Fixed height for composer area
	historyHeight := p.height - composerHeight - 1 - selectorHeight // -1 for separator
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
	if p.selector != nil {
		p.composer.SetTitle("Model: " + p.selector.currentName())
	} else {
		p.composer.SetTitle("")
	}
	composerView := p.composer.View()

	// Join vertically - don't add Width constraints here
	// The SplitLayout.ensureSize() handles width normalization
	sections := []string{
		historyView,
		separator,
	}

	// Add command picker above composer if visible
	if p.commandPicker != nil && p.commandPicker.Visible() {
		p.commandPicker.SetSize(p.width-4, 12)
		sections = append(sections, p.commandPicker.View())
	}

	sections = append(sections, composerView)

	if p.selector != nil && p.selector.Open {
		sections = append(sections, p.selector.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
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
	lastRole := ""
	for _, msg := range p.messages {
		roleLower := strings.ToLower(msg.Role)
		showRole := roleLower != "system"
		if p.settings.GroupMessages && roleLower == lastRole {
			showRole = false
		}

		// Role header (omit system labels)
		if showRole {
			roleStyle := p.roleStyle(msg.Role)
			lines = append(lines, roleStyle.Render(formatRole(msg.Role)+":"))
		}
		lastRole = roleLower

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

// SetAgentSelector sets the selector rendered under the composer.
func (p *ChatPanel) SetAgentSelector(selector *AgentSelector) {
	p.selector = selector
}

// SetSettings updates chat panel settings.
func (p *ChatPanel) SetSettings(settings ChatSettings) {
	p.settings = settings
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

// ScrollOffsetForTest exposes the scroll offset for tests.
func (p *ChatPanel) ScrollOffsetForTest() int {
	return p.scroll
}

// SubmitInput returns the current composer value and checks if it's a slash command.
// If it's a slash command, it returns a SlashCommandMsg command and clears the composer.
// If it's regular text, it returns nil and leaves the value for the caller to handle.
// Use this when the user presses Enter to submit input.
func (p *ChatPanel) SubmitInput() tea.Cmd {
	value := p.Value()
	if cmd, args, isSlash := ParseSlashCommand(value); isSlash {
		p.ClearComposer()
		return func() tea.Msg {
			return SlashCommandMsg{Command: cmd, Args: args}
		}
	}
	return nil
}

// SetViewCommands sets view-specific slash commands for the picker.
// These are added to the global commands.
func (p *ChatPanel) SetViewCommands(commands []SlashCommandDef) {
	if p.commandPicker == nil {
		return
	}
	// Combine global commands with view-specific commands
	all := append(GlobalCommands(), commands...)
	p.commandPicker.SetCommands(all)
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
