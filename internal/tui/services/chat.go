// Package services provides shared services for TUI views.
package services

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// DefaultTimeout is the default timeout for AI generation requests.
const DefaultTimeout = 30 * time.Second

// AgentInterface abstracts the AI agent for testability.
type AgentInterface interface {
	// Generate sends a prompt to the agent and returns the response.
	Generate(ctx context.Context, prompt string) (string, error)
	// Available returns whether the agent is available for use.
	Available() bool
}

// ChatService manages chat interactions with an AI agent.
type ChatService struct {
	agent   AgentInterface
	timeout time.Duration
}

// NewChatService creates a new chat service with the given agent.
func NewChatService(agent AgentInterface) *ChatService {
	return &ChatService{
		agent:   agent,
		timeout: DefaultTimeout,
	}
}

// SetTimeout sets the timeout for AI generation requests.
func (s *ChatService) SetTimeout(d time.Duration) {
	s.timeout = d
}

// Send sends a message to the agent and returns a command that will
// produce a ChatResponseMsg when the response is ready.
func (s *ChatService) Send(viewID, msg string) tea.Cmd {
	// Handle agent unavailable
	if s.agent == nil || !s.agent.Available() {
		return func() tea.Msg {
			return ChatResponseMsg{
				ViewID: viewID,
				Error:  ErrAgentUnavailable,
			}
		}
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
		defer cancel()

		resp, err := s.agent.Generate(ctx, msg)
		return ChatResponseMsg{
			ViewID:   viewID,
			Response: resp,
			Error:    err,
		}
	}
}

// SendThinking returns a command that produces a ChatThinkingMsg.
// This can be used to show a loading indicator while waiting for a response.
func (s *ChatService) SendThinking(viewID string) tea.Cmd {
	return func() tea.Msg {
		return ChatThinkingMsg{ViewID: viewID}
	}
}

// ErrAgentUnavailable is returned when the agent is nil or not available.
var ErrAgentUnavailable = errors.New("agent unavailable")

// ChatResponseMsg is sent when a chat response is received (or an error occurs).
type ChatResponseMsg struct {
	ViewID   string
	Response string
	Error    error
}

// ChatThinkingMsg is sent to indicate the agent is processing.
type ChatThinkingMsg struct {
	ViewID string
}

// ChatCancelMsg is sent when the user cancels a pending request.
type ChatCancelMsg struct {
	ViewID string
}
