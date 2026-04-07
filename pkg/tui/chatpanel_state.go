package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// AddMessage adds a message to the finalized chat history.
func (p *ChatPanel) AddMessage(role, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	msg := ChatMessage{Role: role, Content: content}
	p.history = append(p.history, msg)

	// Render and cache the new message's lines.
	contentWidth := p.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}
	lines := p.renderMessageLines(msg, contentWidth, p.lastRole())
	p.historyLines = append(p.historyLines, lines)

	// Auto-scroll to bottom when new message added
	if p.settings.AutoScroll {
		p.scroll = 0
		p.followTail = true
	}
}

// lastRole returns the role of the second-to-last history message (for grouping).
// Used when caching a newly-added message.
func (p *ChatPanel) lastRole() string {
	if len(p.history) < 2 {
		return ""
	}
	return strings.ToLower(p.history[len(p.history)-2].Role)
}

// SetStatus sets the streaming status label. Pass empty string to clear.
func (p *ChatPanel) SetStatus(status string) {
	p.status = status
	p.streaming = status != ""
}

// IsStreaming returns whether the agent is currently streaming.
func (p *ChatPanel) IsStreaming() bool {
	return p.streaming
}

// SpinnerTick returns the spinner tick command. Call this from the parent view's Init or when streaming starts.
func (p *ChatPanel) SpinnerTick() tea.Cmd {
	return p.spinner.Tick()
}

// ClearMessages removes all messages from the chat history.
func (p *ChatPanel) ClearMessages() {
	p.history = nil
	p.historyLines = nil
	p.buffer = nil
	p.scroll = 0
	p.followTail = true
}

// ResetSession clears chat history and resets handler continuation state.
func (p *ChatPanel) ResetSession() {
	if mth, ok := p.handler.(MultiTurnHandler); ok {
		mth.ResetSession()
	}
	p.history = nil
	p.historyLines = nil
	p.buffer = nil
	p.scroll = 0
	p.followTail = true
	p.chatState = ChatIdle
	p.SetStatus("")
	p.cleanupStream()
}

// Messages returns a copy of all messages, including any in-progress buffer content.
func (p *ChatPanel) Messages() []ChatMessage {
	result := make([]ChatMessage, len(p.history))
	copy(result, p.history)
	// Include partial buffer content so callers see the in-progress message.
	if p.buffer != nil && p.buffer.Len() > 0 {
		result = append(result, ChatMessage{
			Role:    "agent",
			Content: p.buffer.String(),
		})
	}
	return result
}

// SetSize sets the dimensions of the chat panel.
func (p *ChatPanel) SetSize(width, height int) {
	// Invalidate history cache if width changed.
	if width != p.width {
		p.historyLines = nil
		p.historyWidth = 0
	}
	p.width = width
	p.height = height

	// Composer has a dynamic content height but bounded max area.
	composerHeight := 12
	p.composer.SetSize(width, composerHeight)
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
	p.followTail = false
	p.scroll++
}

// ScrollDown scrolls the history down (shows newer messages).
func (p *ChatPanel) ScrollDown() {
	if p.scroll > 0 {
		p.scroll--
	}
	if p.scroll == 0 {
		p.followTail = true
	}
}

// ScrollToBottom scrolls to the most recent messages.
func (p *ChatPanel) ScrollToBottom() {
	p.scroll = 0
	p.followTail = true
}

// ScrollOffsetForTest exposes the scroll offset for tests.
func (p *ChatPanel) ScrollOffsetForTest() int {
	return p.scroll
}

// FollowTailForTest exposes the followTail flag for tests.
func (p *ChatPanel) FollowTailForTest() bool {
	return p.followTail
}

// BufferForTest exposes the streaming buffer for tests.
func (p *ChatPanel) BufferForTest() *StreamBuffer {
	return p.buffer
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
