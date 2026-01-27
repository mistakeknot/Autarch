package services

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockAgent implements AgentInterface for testing.
type mockAgent struct {
	available   bool
	response    string
	err         error
	generateErr error
	delay       time.Duration
}

func (m *mockAgent) Available() bool {
	return m.available
}

func (m *mockAgent) Generate(ctx context.Context, prompt string) (string, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if m.generateErr != nil {
		return "", m.generateErr
	}
	return m.response, nil
}

func TestChatServiceNilAgentReturnsError(t *testing.T) {
	s := NewChatService(nil)
	cmd := s.Send("view-1", "hello")

	msg := cmd()
	resp, ok := msg.(ChatResponseMsg)
	if !ok {
		t.Fatal("expected ChatResponseMsg")
	}

	if resp.Error != ErrAgentUnavailable {
		t.Fatalf("expected ErrAgentUnavailable, got %v", resp.Error)
	}
	if resp.ViewID != "view-1" {
		t.Fatalf("expected viewID 'view-1', got %s", resp.ViewID)
	}
}

func TestChatServiceUnavailableAgentReturnsError(t *testing.T) {
	agent := &mockAgent{available: false}
	s := NewChatService(agent)
	cmd := s.Send("view-2", "hello")

	msg := cmd()
	resp := msg.(ChatResponseMsg)

	if resp.Error != ErrAgentUnavailable {
		t.Fatalf("expected ErrAgentUnavailable, got %v", resp.Error)
	}
}

func TestChatServiceSuccessfulGeneration(t *testing.T) {
	agent := &mockAgent{
		available: true,
		response:  "Hello! I can help you.",
	}
	s := NewChatService(agent)
	cmd := s.Send("view-3", "help me")

	msg := cmd()
	resp := msg.(ChatResponseMsg)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Response != "Hello! I can help you." {
		t.Fatalf("unexpected response: %s", resp.Response)
	}
	if resp.ViewID != "view-3" {
		t.Fatalf("unexpected viewID: %s", resp.ViewID)
	}
}

func TestChatServiceGenerationError(t *testing.T) {
	agent := &mockAgent{
		available:   true,
		generateErr: errors.New("network error"),
	}
	s := NewChatService(agent)
	cmd := s.Send("view-4", "help")

	msg := cmd()
	resp := msg.(ChatResponseMsg)

	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Error() != "network error" {
		t.Fatalf("expected 'network error', got %v", resp.Error)
	}
}

func TestChatServiceTimeout(t *testing.T) {
	agent := &mockAgent{
		available: true,
		delay:     2 * time.Second,
		response:  "slow response",
	}
	s := NewChatService(agent)
	s.SetTimeout(100 * time.Millisecond) // Very short timeout

	cmd := s.Send("view-5", "help")

	start := time.Now()
	msg := cmd()
	elapsed := time.Since(start)

	resp := msg.(ChatResponseMsg)

	// Should timeout within reasonable time
	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected timeout around 100ms, took %v", elapsed)
	}

	// Should have context deadline error
	if resp.Error == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(resp.Error, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", resp.Error)
	}
}

func TestChatServiceViewIDRouting(t *testing.T) {
	agent := &mockAgent{available: true, response: "ok"}
	s := NewChatService(agent)

	viewIDs := []string{"gurgeh", "pollard", "coldwine"}
	for _, viewID := range viewIDs {
		cmd := s.Send(viewID, "test")
		msg := cmd()
		resp := msg.(ChatResponseMsg)

		if resp.ViewID != viewID {
			t.Fatalf("expected viewID %s, got %s", viewID, resp.ViewID)
		}
	}
}

func TestChatServiceSendThinking(t *testing.T) {
	s := NewChatService(nil)
	cmd := s.SendThinking("view-6")

	msg := cmd()
	thinking, ok := msg.(ChatThinkingMsg)
	if !ok {
		t.Fatal("expected ChatThinkingMsg")
	}
	if thinking.ViewID != "view-6" {
		t.Fatalf("expected viewID 'view-6', got %s", thinking.ViewID)
	}
}
