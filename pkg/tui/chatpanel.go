package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
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
	mdRenderer    *glamour.TermRenderer
	mdWidth       int
	spinner       spinner.Model
	status        string // Current status text ("Thinking...", "Responding...", "")
	streaming     bool   // Whether the agent is currently streaming
	handler       ChatHandler
	chatState     ChatState
	streamCtx     context.Context
	cancelStream  context.CancelFunc
	events        <-chan StreamMsg
}

// StreamChunkMsg wraps a StreamMsg for delivery via Bubble Tea's message system.
type StreamChunkMsg struct {
	Event StreamMsg
}

type streamStartedMsg struct {
	events <-chan StreamMsg
}

// MultiTurnHandler is optionally implemented by ChatHandlers that support multi-turn.
type MultiTurnHandler interface {
	SetContinue(cont bool, sessionID string)
	ResetSession()
}

// NewChatPanel creates a new chat panel with default settings.
func NewChatPanel() *ChatPanel {
	composer := NewComposer(6)
	picker := NewCommandPicker(GlobalCommands())
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorPrimary)
	return &ChatPanel{
		messages:      []ChatMessage{},
		composer:      composer,
		commandPicker: picker,
		settings:      DefaultChatSettings(),
		spinner:       s,
		chatState:     ChatIdle,
	}
}

// SetHandler sets the ChatHandler for processing non-slash input.
func (p *ChatPanel) SetHandler(handler ChatHandler) {
	p.handler = handler
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
	return p.spinner.Tick
}

// ClearMessages removes all messages from the chat history.
func (p *ChatPanel) ClearMessages() {
	p.messages = nil
	p.scroll = 0
}

// ResetSession clears chat history and resets handler continuation state.
func (p *ChatPanel) ResetSession() {
	if mth, ok := p.handler.(MultiTurnHandler); ok {
		mth.ResetSession()
	}
	p.messages = nil
	p.scroll = 0
	p.chatState = ChatIdle
	p.SetStatus("")
	p.cleanupStream()
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

	// Composer has a dynamic content height but bounded max area.
	composerHeight := 12
	p.composer.SetSize(width, composerHeight)
}

func (p *ChatPanel) markdownRenderer(width int) *glamour.TermRenderer {
	if p.mdRenderer != nil && p.mdWidth == width {
		return p.mdRenderer
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithStandardStyle("dark"),
	)
	if err != nil {
		return nil
	}
	p.mdRenderer = r
	p.mdWidth = width
	return r
}

// Update handles tea.Msg for the chat panel.
func (p *ChatPanel) Update(msg tea.Msg) (*ChatPanel, tea.Cmd) {
	// Handle spinner animation
	if msg, ok := msg.(spinner.TickMsg); ok && p.streaming {
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		return p, cmd
	}

	switch typedMsg := msg.(type) {
	case streamStartedMsg:
		p.events = typedMsg.events
		return p, waitForStreamEvent(p.events)
	case StreamChunkMsg:
		return p.handleStreamChunk(typedMsg.Event)
	}

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

	// Render composer first so history can use its actual height.
	if p.selector != nil {
		p.composer.SetTitle("Model: " + p.selector.currentName())
	} else {
		p.composer.SetTitle("")
	}
	composerView := p.composer.View()

	// Calculate heights
	composerHeight := lipgloss.Height(composerView)
	if composerHeight < 1 {
		composerHeight = 1
	}
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

		// Content rendering — agent messages get markdown, others get plain text
		contentWidth := p.width - 4
		if contentWidth < 10 {
			contentWidth = 10
		}

		if strings.ToLower(msg.Role) == "agent" {
			// Render agent messages as markdown via glamour
			if r := p.markdownRenderer(contentWidth); r != nil {
				rendered, err := r.Render(msg.Content)
				if err == nil {
					rendered = strings.TrimSpace(rendered)
					// Indent rendered markdown
					contentStyle := lipgloss.NewStyle().PaddingLeft(2)
					lines = append(lines, contentStyle.Render(rendered))
				} else {
					// Fallback to plain text on render error
					contentStyle := lipgloss.NewStyle().
						Foreground(ColorFg).
						PaddingLeft(2)
					wrapped := wrapText(msg.Content, contentWidth)
					for _, line := range strings.Split(wrapped, "\n") {
						lines = append(lines, contentStyle.Render(line))
					}
				}
			} else {
				// Fallback if renderer creation fails
				contentStyle := lipgloss.NewStyle().
					Foreground(ColorFg).
					PaddingLeft(2)
				wrapped := wrapText(msg.Content, contentWidth)
				for _, line := range strings.Split(wrapped, "\n") {
					lines = append(lines, contentStyle.Render(line))
				}
			}
		} else {
			// User and system messages: plain text with word wrap
			contentStyle := lipgloss.NewStyle().
				Foreground(ColorFg).
				PaddingLeft(2)
			wrapped := wrapText(msg.Content, contentWidth)
			for _, line := range strings.Split(wrapped, "\n") {
				lines = append(lines, contentStyle.Render(line))
			}
		}
		lines = append(lines, "") // Blank line between messages
	}

	// Add streaming status indicator
	if p.streaming && p.status != "" {
		statusStyle := lipgloss.NewStyle().
			Foreground(ColorPrimary).
			PaddingLeft(2)
		lines = append(lines, statusStyle.Render(p.spinner.View()+" "+p.status))
		lines = append(lines, "") // Blank line after status
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

// SubmitInput processes slash commands and non-slash chat input.
func (p *ChatPanel) SubmitInput() tea.Cmd {
	value := strings.TrimSpace(p.Value())
	if value == "" {
		return nil
	}

	if cmd, args, isSlash := ParseSlashCommand(value); isSlash {
		p.ClearComposer()
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "/new" || trimmed == "/clear" {
			p.ResetSession()
			return nil
		}
		return func() tea.Msg {
			return SlashCommandMsg{Command: cmd, Args: args}
		}
	}

	if p.handler == nil {
		return nil
	}

	p.ClearComposer()
	p.AddMessage("user", value)
	p.cleanupStream()

	ctx, cancel := context.WithCancel(context.Background())
	p.streamCtx = ctx
	p.cancelStream = cancel
	p.chatState = ChatThinking
	p.SetStatus(ChatThinking.String())
	p.messages = append(p.messages, ChatMessage{Role: "agent", Content: ""})

	handler := p.handler
	userMsg := value

	return tea.Batch(
		p.SpinnerTick(),
		func() tea.Msg {
			events, err := handler.HandleMessage(ctx, userMsg)
			if err != nil {
				return StreamChunkMsg{Event: StreamError{Err: err}}
			}
			return streamStartedMsg{events: events}
		},
	)
}

// CancelStream cancels any active streaming and returns to idle state.
func (p *ChatPanel) CancelStream() {
	if p.chatState == ChatIdle {
		return
	}
	p.chatState = ChatIdle
	p.SetStatus("")
	p.cleanupStream()
}

func (p *ChatPanel) handleStreamChunk(event StreamMsg) (*ChatPanel, tea.Cmd) {
	switch e := event.(type) {
	case TextDelta:
		if len(p.messages) == 0 || strings.ToLower(p.messages[len(p.messages)-1].Role) != "agent" {
			p.messages = append(p.messages, ChatMessage{Role: "agent", Content: e.Text})
		} else {
			last := &p.messages[len(p.messages)-1]
			last.Content += e.Text
		}
		if p.chatState != ChatStreaming {
			p.chatState = ChatStreaming
			p.SetStatus(ChatStreaming.String())
		}
		p.scroll = 0
		return p, waitForStreamEvent(p.events)

	case ReasoningStart:
		p.chatState = ChatThinking
		p.SetStatus(ChatThinking.String())
		return p, tea.Batch(p.SpinnerTick(), waitForStreamEvent(p.events))

	case ReasoningDelta:
		return p, waitForStreamEvent(p.events)

	case ReasoningEnd:
		return p, waitForStreamEvent(p.events)

	case ToolCallStart:
		return p, waitForStreamEvent(p.events)

	case ToolCallInput:
		return p, waitForStreamEvent(p.events)

	case ToolCallResult:
		return p, waitForStreamEvent(p.events)

	case StreamError:
		p.chatState = ChatError
		p.SetStatus("")
		if len(p.messages) > 0 {
			last := &p.messages[len(p.messages)-1]
			if strings.TrimSpace(last.Content) == "" {
				last.Content = "Error: " + e.Err.Error()
			}
		}
		p.cleanupStream()
		return p, nil

	case StreamDone:
		p.chatState = ChatIdle
		p.SetStatus("")
		// Enable multi-turn continuation if the handler supports it.
		if e.SessionID != "" {
			if mth, ok := p.handler.(MultiTurnHandler); ok {
				mth.SetContinue(true, e.SessionID)
			}
		}
		if len(p.messages) > 0 {
			last := p.messages[len(p.messages)-1]
			if strings.ToLower(last.Role) == "agent" && strings.TrimSpace(last.Content) == "" {
				p.messages = p.messages[:len(p.messages)-1]
			}
		}
		p.cleanupStream()
		return p, nil
	}

	return p, waitForStreamEvent(p.events)
}

func (p *ChatPanel) cleanupStream() {
	if p.cancelStream != nil {
		p.cancelStream()
	}
	p.streamCtx = nil
	p.cancelStream = nil
	p.events = nil
}

func waitForStreamEvent(events <-chan StreamMsg) tea.Cmd {
	return func() tea.Msg {
		if events == nil {
			return StreamChunkMsg{Event: StreamDone{FinishReason: "stop"}}
		}
		event, ok := <-events
		if !ok {
			return StreamChunkMsg{Event: StreamDone{FinishReason: "stop"}}
		}
		return StreamChunkMsg{Event: event}
	}
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
