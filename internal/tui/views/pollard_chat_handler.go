package views

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/mistakeknot/autarch/pkg/claude"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// PollardChatHandler implements ChatHandler for the Pollard dashboard.
type PollardChatHandler struct {
	mu              sync.RWMutex
	cwd             string
	continueSession bool
	sessionID       string
}

// NewPollardChatHandler creates a Pollard chat handler.
func NewPollardChatHandler() *PollardChatHandler {
	cwd, _ := os.Getwd()
	return &PollardChatHandler{cwd: cwd}
}

// HandleMessage sends the user's message to Claude with Pollard context.
func (h *PollardChatHandler) HandleMessage(ctx context.Context, userMsg string) (<-chan pkgtui.StreamMsg, error) {
	systemCtx := "You are Pollard, a general-purpose research intelligence assistant. Help the user research topics, analyze competitive landscapes, find academic papers, and synthesize findings across domains."
	h.mu.RLock()
	continueSession := h.continueSession
	sessionID := h.sessionID
	h.mu.RUnlock()

	args := []string{}
	if continueSession {
		if sessionID != "" {
			args = append(args, "--resume", sessionID)
		} else {
			args = append(args, "-c")
		}
	}
	args = append(args, "--system-prompt", systemCtx, "-p", userMsg)
	events, err := claude.RunStreaming(ctx, h.cwd, args)
	if err != nil {
		return nil, err
	}

	out := make(chan pkgtui.StreamMsg, 64)
	go func() {
		defer close(out)
		var streamSessionID string
		for event := range events {
			var msg pkgtui.StreamMsg
			switch event.Type {
			case claude.EventSessionID:
				streamSessionID = event.SessionID
				continue
			case claude.EventText:
				msg = pkgtui.TextDelta{Text: event.Text}
			case claude.EventThinkingStart:
				msg = pkgtui.ReasoningStart{}
			case claude.EventThinking:
				msg = pkgtui.ReasoningDelta{Text: event.Text}
			case claude.EventThinkingEnd:
				msg = pkgtui.ReasoningEnd{}
			case claude.EventToolUse:
				msg = pkgtui.ToolCallStart{Name: event.ToolName}
			case claude.EventResult:
				if event.IsError {
					msg = pkgtui.StreamError{Err: fmt.Errorf("%s", event.Text)}
				} else {
					msg = pkgtui.StreamDone{
						FinishReason: "stop",
						SessionID:    streamSessionID,
					}
				}
			case claude.EventError:
				msg = pkgtui.StreamError{Err: fmt.Errorf("%s", event.Text)}
			default:
				continue
			}

			select {
			case out <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

// SetContinue configures multi-turn continuation state for Claude CLI.
func (h *PollardChatHandler) SetContinue(cont bool, sessionID string) {
	h.mu.Lock()
	h.continueSession = cont
	h.sessionID = sessionID
	h.mu.Unlock()
}

// ResetSession clears any continuation state for a new conversation.
func (h *PollardChatHandler) ResetSession() {
	h.mu.Lock()
	h.continueSession = false
	h.sessionID = ""
	h.mu.Unlock()
}
