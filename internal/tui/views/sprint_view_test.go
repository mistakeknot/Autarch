package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/testutil"
	"github.com/mistakeknot/autarch/internal/tui"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

func TestSprintView_ChatSubmitProducesResponse(t *testing.T) {
	testutil.WithoutAgents(t)
	// Create a sprint view and start a sprint
	v := NewSprintView("/tmp/test-project", SprintViewOpts{})
	defer v.Blur()

	// Initialize and set size
	v.Init()
	v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Start a sprint (synchronous for test)
	startCmd := v.StartSprint("Build a todo app")
	if startCmd == nil {
		t.Fatal("StartSprint returned nil cmd")
	}

	// Execute the start command to get the message
	startMsg := startCmd()
	t.Logf("Start message type: %T", startMsg)

	// Feed the start message to the view
	v.Update(startMsg)

	// Now simulate typing "make the vision more specific" into the composer
	// First, we need to set the value directly since we can't easily simulate keystrokes
	v.chatPanel.SetValue("Make the vision more specific")
	got := v.chatPanel.Value()
	if got != "Make the vision more specific" {
		t.Fatalf("Composer value mismatch: got %q", got)
	}

	// Now simulate pressing Enter
	_, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter key returned nil cmd — chat submit didn't produce a waitForResponse cmd")
	}

	// Execute the cmd — it should read from the response channel
	msg := cmd()
	t.Logf("Response message type: %T", msg)

	switch m := msg.(type) {
	case tui.SprintStreamLineMsg:
		if m.Content == "" {
			t.Error("SprintStreamLineMsg has empty content")
		} else {
			t.Logf("Got agent response (%d chars): %s...", len(m.Content), m.Content[:min(100, len(m.Content))])
		}
	case tui.SprintStreamDoneMsg:
		t.Error("Got SprintStreamDoneMsg immediately — no content was streamed")
	case tui.GenerationErrorMsg:
		t.Errorf("Got generation error: %v", m.Error)
	default:
		t.Errorf("Unexpected message type: %T = %v", msg, msg)
	}
}

func TestSprintView_FocusedAndValueWorks(t *testing.T) {
	v := NewSprintView("/tmp/test-project", SprintViewOpts{})
	v.Init()
	v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Check focus
	if !v.chatPanel.Focused() {
		t.Error("Chat panel should be focused after Init")
	}

	// Type some characters via Update
	for _, r := range "hello" {
		v.chatPanel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	got := v.chatPanel.Value()
	if got != "hello" {
		t.Errorf("After typing 'hello', Value() = %q", got)
	}
}

func TestSprintView_TypingViaUpdateReachesComposer(t *testing.T) {
	testutil.WithoutAgents(t)
	v := NewSprintView("/tmp/test-project", SprintViewOpts{})
	defer v.Blur()
	v.Init()
	v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Start sprint so we're in a valid state
	startCmd := v.StartSprint("Build a todo app")
	startMsg := startCmd()
	v.Update(startMsg)

	// Type via v.Update (full key routing)
	for _, r := range "hello" {
		v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	got := v.chatPanel.Value()
	if got != "hello" {
		t.Errorf("After typing 'hello' via v.Update(), Value() = %q", got)
	}

	// Now press Enter
	_, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter via v.Update() returned nil cmd")
	}

	// Verify the user message was added to chat history
	messages := v.chatPanel.Messages()
	found := false
	for _, m := range messages {
		if m.Role == "user" && m.Content == "hello" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("User message 'hello' not found in chat history. Messages: %+v", messages)
	}
}

func TestSprintView_BlurCancelsContext(t *testing.T) {
	v := NewSprintView("/tmp/test-project", SprintViewOpts{})
	if v.ctx == nil {
		t.Fatal("expected sprint context to be initialized")
	}
	done := v.ctx.Done()

	v.Blur()

	select {
	case <-done:
	default:
		t.Fatal("expected context to be canceled on blur")
	}

	// Re-focus should recreate context and remain cancel-safe.
	v.Focus()
	v.Blur()
}

// Ensure the View interface is satisfied
var _ pkgtui.View = (*SprintView)(nil)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
