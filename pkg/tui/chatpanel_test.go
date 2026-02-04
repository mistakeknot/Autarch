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
