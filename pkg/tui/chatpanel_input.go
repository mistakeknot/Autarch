package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

// SubmitInput processes slash commands and non-slash chat input.
func (p *ChatPanel) SubmitInput() tea.Cmd {
	// If the command picker is visible with a selection, execute that command
	// instead of the raw composer text. This lets "/g" + Enter execute "/gurgeh".
	if p.commandPicker != nil && p.commandPicker.Visible() && len(p.commandPicker.filtered) > 0 {
		selected := p.commandPicker.filtered[p.commandPicker.selected]
		p.commandPicker.Hide()
		p.ClearComposer()
		return func() tea.Msg {
			return SlashCommandMsg{Command: selected.Command}
		}
	}

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
	// Create a streaming buffer instead of appending an empty message.
	p.buffer = NewStreamBuffer()

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
	// Finalize any partial buffer content into history.
	if p.buffer != nil && p.buffer.Len() > 0 {
		msg := p.buffer.Finalize()
		p.AddMessage(msg.Role, msg.Content)
	}
	p.buffer = nil
	p.chatState = ChatIdle
	p.SetStatus("")
	p.cleanupStream()
}

func (p *ChatPanel) handleStreamChunk(event StreamMsg) (*ChatPanel, tea.Cmd) {
	switch e := event.(type) {
	case TextDelta:
		// Append to the streaming buffer instead of mutating history.
		if p.buffer == nil {
			p.buffer = NewStreamBuffer()
		}
		p.buffer.Append(e.Text)
		if p.chatState != ChatStreaming {
			p.chatState = ChatStreaming
			p.SetStatus(ChatStreaming.String())
			if p.buffer != nil {
				p.buffer.SetState(ChatStreaming)
			}
		}
		// Do NOT force scroll — respect followTail.
		return p, waitForStreamEvent(p.events)

	case ReasoningStart:
		p.chatState = ChatThinking
		p.SetStatus(ChatThinking.String())
		if p.buffer != nil {
			p.buffer.SetState(ChatThinking)
		}
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
		// Finalize buffer with error content.
		if p.buffer != nil {
			if p.buffer.Len() == 0 {
				p.buffer.Append("Error: " + e.Err.Error())
			}
			msg := p.buffer.Finalize()
			p.AddMessage(msg.Role, msg.Content)
			p.buffer = nil
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
		// Finalize buffer into history.
		if p.buffer != nil {
			if p.buffer.Len() > 0 {
				msg := p.buffer.Finalize()
				p.AddMessage(msg.Role, msg.Content)
			}
			p.buffer = nil
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
