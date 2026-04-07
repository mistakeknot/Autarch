package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/Masaq/spinner"
)

// ChatMessage represents a single message in the chat history.
type ChatMessage struct {
	Role    string // "user", "agent", "system"
	Content string
}

// ChatPanel combines a scrollable chat history with a composer at the bottom.
// This is the right-side panel in the Cursor-style split layout.
//
// Streaming output is separated from finalized history: a StreamBuffer owns
// in-flight content during a turn, and history holds only completed messages.
// Pre-rendered line caches avoid re-running glamour markdown on every frame.
type ChatPanel struct {
	history       []ChatMessage  // finalized messages only
	historyLines  [][]string     // pre-rendered lines per history message
	historyWidth  int            // width the cache was rendered at
	buffer        *StreamBuffer  // live streaming content (nil when idle)
	followTail    bool           // true = auto-scroll to bottom on new content
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
	return &ChatPanel{
		history:       []ChatMessage{},
		historyLines:  [][]string{},
		followTail:    true,
		composer:      composer,
		commandPicker: picker,
		settings:      DefaultChatSettings(),
		spinner:       spinner.New(),
		chatState:     ChatIdle,
	}
}

// SetHandler sets the ChatHandler for processing non-slash input.
func (p *ChatPanel) SetHandler(handler ChatHandler) {
	p.handler = handler
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

// View renders the complete chat panel (history + buffer + composer).
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
	totalHeight := p.height - composerHeight - 1 - selectorHeight // -1 for separator
	if totalHeight < 1 {
		totalHeight = 1
	}

	// Split available height between history and buffer.
	var bufferView string
	bufferHeight := 0
	if p.buffer != nil {
		bufferView = p.renderBuffer(p.width)
		bh := lipgloss.Height(bufferView)
		// Buffer gets at most 1/3 of available space.
		maxBuf := totalHeight / 3
		if maxBuf < 3 {
			maxBuf = 3
		}
		if bh > maxBuf {
			bufferHeight = maxBuf
		} else {
			bufferHeight = bh
		}
	}
	historyHeight := totalHeight - bufferHeight
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
	}

	// Insert buffer between history and separator when streaming.
	if p.buffer != nil && bufferView != "" {
		// Truncate buffer to allocated height (show tail).
		bvLines := strings.Split(bufferView, "\n")
		if len(bvLines) > bufferHeight {
			bvLines = bvLines[len(bvLines)-bufferHeight:]
		}
		sections = append(sections, strings.Join(bvLines, "\n"))
	}

	sections = append(sections, separator)

	sections = append(sections, composerView)

	// Add command picker below composer if visible
	if p.commandPicker != nil && p.commandPicker.Visible() {
		p.commandPicker.SetSize(p.width-4, 12)
		sections = append(sections, p.commandPicker.View())
	}

	if p.selector != nil && p.selector.Open {
		sections = append(sections, p.selector.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}
