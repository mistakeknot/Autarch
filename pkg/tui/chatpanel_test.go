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
