package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestChatPanelHidesSystemRoleLabel(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(60, 20)
	panel.AddMessage("system", "Welcome")

	view := panel.View()
	if strings.Contains(view, "System:") {
		t.Fatalf("expected System label to be hidden, got %q", view)
	}
	if !strings.Contains(view, "Welcome") {
		t.Fatalf("expected system content to be rendered")
	}
}

func TestChatPanelIgnoresMouseEscapeSequences(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(60, 20)
	panel.Focus()
	panel.SetValue("hello")

	_, _ = panel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[<64;150;16M")})

	if panel.Value() != "hello" {
		t.Fatalf("expected mouse escape sequence to be ignored")
	}
}

func TestParseSlashCommand(t *testing.T) {
	tests := []struct {
		input   string
		wantCmd string
		wantArg []string
		wantOK  bool
	}{
		{"/help", "help", nil, true},
		{"/HELP", "help", nil, true},
		{"/quit", "quit", nil, true},
		{"  /help  ", "help", nil, true},
		{"/search foo bar", "search", []string{"foo", "bar"}, true},
		{"hello world", "", nil, false},
		{"/ ", "", nil, false},
		{"/", "", nil, false},
		{"", "", nil, false},
	}

	for _, tt := range tests {
		cmd, args, ok := ParseSlashCommand(tt.input)
		if ok != tt.wantOK {
			t.Errorf("ParseSlashCommand(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
		}
		if cmd != tt.wantCmd {
			t.Errorf("ParseSlashCommand(%q) cmd = %q, want %q", tt.input, cmd, tt.wantCmd)
		}
		if len(args) != len(tt.wantArg) {
			t.Errorf("ParseSlashCommand(%q) args = %v, want %v", tt.input, args, tt.wantArg)
		}
	}
}

func TestChatPanelSubmitInput(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(60, 20)
	panel.Focus()

	// Regular text should return nil
	panel.SetValue("hello world")
	cmd := panel.SubmitInput()
	if cmd != nil {
		t.Fatal("expected nil for regular text")
	}
	if panel.Value() != "hello world" {
		t.Fatal("expected value to remain unchanged for regular text")
	}

	// Slash command should return SlashCommandMsg and clear composer
	panel.SetValue("/help")
	cmd = panel.SubmitInput()
	if cmd == nil {
		t.Fatal("expected non-nil cmd for slash command")
	}
	if panel.Value() != "" {
		t.Fatal("expected composer to be cleared after slash command")
	}

	// Execute the command and check the message type
	msg := cmd()
	slashMsg, ok := msg.(SlashCommandMsg)
	if !ok {
		t.Fatalf("expected SlashCommandMsg, got %T", msg)
	}
	if slashMsg.Command != "help" {
		t.Fatalf("expected command 'help', got %q", slashMsg.Command)
	}
}

func TestChatPanelKernelSlashCommands(t *testing.T) {
	tests := []struct {
		input   string
		wantCmd string
		wantArg []string
	}{
		// Note: "/new" and "/clear" are special-cased to reset session and return nil
		{"/quit extra-arg", "quit", []string{"extra-arg"}},
		{"/model opus", "model", []string{"opus"}},
		{"/accept", "accept", nil},
		{"/vision jump-here", "vision", []string{"jump-here"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			panel := NewChatPanel()
			panel.SetSize(60, 20)
			panel.Focus()
			panel.SetValue(tt.input)

			cmd := panel.SubmitInput()
			if cmd == nil {
				t.Fatalf("expected non-nil cmd for %q", tt.input)
			}
			msg := cmd()
			slashMsg, ok := msg.(SlashCommandMsg)
			if !ok {
				t.Fatalf("expected SlashCommandMsg, got %T", msg)
			}
			if slashMsg.Command != tt.wantCmd {
				t.Errorf("command = %q, want %q", slashMsg.Command, tt.wantCmd)
			}
			if len(slashMsg.Args) != len(tt.wantArg) {
				t.Errorf("args = %v, want %v", slashMsg.Args, tt.wantArg)
			}
		})
	}
}

func TestChatPanelRenderMessages(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(80, 30)

	// Add messages with plain text (no markdown — glamour would alter it)
	panel.AddMessage("user", "What is the current sprint status")
	panel.AddMessage("agent", "The sprint is in brainstorm phase")
	panel.AddMessage("system", "Connected to kernel")

	view := panel.View()

	// User and system messages render as plain text
	if !strings.Contains(view, "sprint status") {
		t.Error("expected user message content in view")
	}
	if !strings.Contains(view, "Connected to kernel") {
		t.Error("expected system message content in view")
	}
	// Agent messages go through glamour markdown rendering, so check for a word
	// that survives rendering (glamour doesn't consume plain words)
	if !strings.Contains(view, "brainstorm") {
		t.Error("expected agent message content word in view")
	}
}

// --- New tests for streaming buffer / history split ---

func TestChatPanelStreamBuffer(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(80, 30)

	// Simulate streaming: create buffer and send chunks.
	panel.buffer = NewStreamBuffer()
	panel.chatState = ChatStreaming
	panel.SetStatus(ChatStreaming.String())

	// TextDelta writes to buffer, not history.
	panel.handleStreamChunk(TextDelta{Text: "Hello "})
	panel.handleStreamChunk(TextDelta{Text: "world"})

	if panel.buffer == nil {
		t.Fatal("buffer should not be nil during streaming")
	}
	if got := panel.buffer.String(); got != "Hello world" {
		t.Fatalf("buffer content = %q, want %q", got, "Hello world")
	}
	// History should not contain the streaming content.
	if len(panel.history) != 0 {
		t.Fatalf("history should be empty during streaming, got %d messages", len(panel.history))
	}

	// StreamDone finalizes buffer into history.
	panel.handleStreamChunk(StreamDone{FinishReason: "stop"})

	if panel.buffer != nil {
		t.Fatal("buffer should be nil after StreamDone")
	}
	if len(panel.history) != 1 {
		t.Fatalf("history should have 1 message after finalization, got %d", len(panel.history))
	}
	if panel.history[0].Content != "Hello world" {
		t.Fatalf("finalized content = %q, want %q", panel.history[0].Content, "Hello world")
	}
}

func TestChatPanelFollowTail(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(80, 30)

	// Default: followTail is true.
	if !panel.followTail {
		t.Fatal("followTail should default to true")
	}

	// Scroll up disables followTail.
	panel.ScrollUp()
	if panel.followTail {
		t.Fatal("followTail should be false after ScrollUp")
	}
	if panel.scroll != 1 {
		t.Fatalf("scroll = %d, want 1", panel.scroll)
	}

	// Scroll down to bottom re-enables followTail.
	panel.ScrollDown()
	if !panel.followTail {
		t.Fatal("followTail should be true after scrolling back to bottom")
	}

	// ScrollUp again, then ScrollToBottom.
	panel.ScrollUp()
	panel.ScrollUp()
	panel.ScrollToBottom()
	if !panel.followTail {
		t.Fatal("followTail should be true after ScrollToBottom")
	}
	if panel.scroll != 0 {
		t.Fatalf("scroll = %d, want 0 after ScrollToBottom", panel.scroll)
	}
}

func TestChatPanelStreamingDoesNotForceScroll(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(80, 30)

	// Add some history so we have something to scroll in.
	for i := 0; i < 20; i++ {
		panel.AddMessage("user", "message line for scrolling test")
	}

	// Scroll up.
	panel.ScrollUp()
	panel.ScrollUp()
	savedScroll := panel.scroll
	savedFollow := panel.followTail

	// Simulate streaming — TextDelta should NOT change scroll or followTail.
	panel.buffer = NewStreamBuffer()
	panel.chatState = ChatStreaming
	panel.SetStatus(ChatStreaming.String())

	// Need to set events to non-nil so handleStreamChunk doesn't panic.
	ch := make(chan StreamMsg, 1)
	ch <- StreamDone{FinishReason: "stop"}
	panel.events = ch

	panel.handleStreamChunk(TextDelta{Text: "new content"})

	if panel.scroll != savedScroll {
		t.Fatalf("scroll changed during TextDelta: got %d, want %d", panel.scroll, savedScroll)
	}
	if panel.followTail != savedFollow {
		t.Fatalf("followTail changed during TextDelta: got %v, want %v", panel.followTail, savedFollow)
	}
}

func TestChatPanelMessagesIncludesBuffer(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(80, 30)
	panel.AddMessage("user", "hello")

	// Create a buffer with partial content.
	panel.buffer = NewStreamBuffer()
	panel.buffer.Append("partial response")

	msgs := panel.Messages()
	if len(msgs) != 2 {
		t.Fatalf("Messages() should include buffer, got %d messages", len(msgs))
	}
	if msgs[1].Role != "agent" || msgs[1].Content != "partial response" {
		t.Fatalf("buffer message = {%q, %q}, want {agent, partial response}", msgs[1].Role, msgs[1].Content)
	}
}

func TestChatPanelCacheInvalidation(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(80, 30)
	panel.AddMessage("user", "test message")

	// Cache should be populated.
	if len(panel.historyLines) != 1 {
		t.Fatalf("historyLines length = %d, want 1", len(panel.historyLines))
	}

	// Resize should invalidate cache.
	panel.SetSize(60, 30)
	if len(panel.historyLines) != 0 {
		t.Fatalf("historyLines should be cleared after resize, got %d", len(panel.historyLines))
	}

	// Rendering should rebuild cache.
	panel.renderHistory(20)
	if len(panel.historyLines) != 1 {
		t.Fatalf("historyLines should be rebuilt after render, got %d", len(panel.historyLines))
	}
}

func TestChatPanelBufferRendering(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(80, 30)

	// No buffer — renderBuffer returns empty.
	if got := panel.renderBuffer(80); got != "" {
		t.Fatalf("renderBuffer with nil buffer should return empty, got %q", got)
	}

	// With buffer content.
	panel.buffer = NewStreamBuffer()
	panel.buffer.Append("streaming text")
	panel.streaming = true
	panel.status = "Responding..."

	rendered := panel.renderBuffer(80)
	// Glamour renders markdown and may add ANSI codes, but the word should survive.
	if !strings.Contains(rendered, "streaming") {
		t.Errorf("renderBuffer should contain buffer text, got %q", rendered)
	}
	if !strings.Contains(rendered, "Responding...") {
		t.Error("renderBuffer should contain status indicator")
	}
}

func TestChatPanelBufferDisappearsOnDone(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(80, 30)

	// Set up streaming.
	panel.buffer = NewStreamBuffer()
	panel.buffer.Append("response")
	panel.chatState = ChatStreaming
	panel.SetStatus(ChatStreaming.String())

	// Finalize.
	panel.handleStreamChunk(StreamDone{FinishReason: "stop"})

	if panel.buffer != nil {
		t.Fatal("buffer should be nil after StreamDone")
	}
	if panel.streaming {
		t.Fatal("streaming should be false after StreamDone")
	}
}

func TestChatPanelStreamError(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(80, 30)

	// Set up streaming with empty buffer.
	panel.buffer = NewStreamBuffer()
	panel.chatState = ChatStreaming
	panel.SetStatus(ChatStreaming.String())

	// Error should finalize buffer with error content.
	panel.handleStreamChunk(StreamError{Err: errForTest("connection lost")})

	if panel.buffer != nil {
		t.Fatal("buffer should be nil after StreamError")
	}
	if len(panel.history) != 1 {
		t.Fatalf("history should have 1 message after error, got %d", len(panel.history))
	}
	if !strings.Contains(panel.history[0].Content, "connection lost") {
		t.Fatalf("error message should contain error text, got %q", panel.history[0].Content)
	}
}

// errForTest is a simple error implementation for tests.
type errForTest string

func (e errForTest) Error() string { return string(e) }

func TestChatPanelCancelStreamFinalizesBuffer(t *testing.T) {
	panel := NewChatPanel()
	panel.SetSize(80, 30)

	panel.buffer = NewStreamBuffer()
	panel.buffer.Append("partial content")
	panel.chatState = ChatStreaming
	panel.SetStatus(ChatStreaming.String())

	panel.CancelStream()

	if panel.buffer != nil {
		t.Fatal("buffer should be nil after CancelStream")
	}
	// Partial content should be finalized into history.
	if len(panel.history) != 1 {
		t.Fatalf("history should have 1 message after cancel, got %d", len(panel.history))
	}
	if panel.history[0].Content != "partial content" {
		t.Fatalf("cancelled content = %q, want %q", panel.history[0].Content, "partial content")
	}
}
