package tui

import (
	"context"
	"fmt"
	"os"

	"github.com/mistakeknot/autarch/pkg/claude"
)

// ClaudeChatHandler implements ChatHandler by running Claude CLI.
type ClaudeChatHandler struct {
	// CWD is the working directory for Claude. Defaults to os.Getwd() if empty.
	CWD string
	// ExtraArgs are additional CLI arguments passed to Claude.
	ExtraArgs []string
	// Continue toggles Claude CLI multi-turn continuation.
	Continue bool
	// SessionID is the Claude CLI session ID used for --resume.
	SessionID string
}

// HandleMessage sends the user's message to Claude CLI and converts stream events.
func (h *ClaudeChatHandler) HandleMessage(ctx context.Context, userMsg string) (<-chan StreamMsg, error) {
	cwd := h.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	args := make([]string, 0, len(h.ExtraArgs)+4)
	if h.Continue {
		if h.SessionID != "" {
			args = append(args, "--resume", h.SessionID)
		} else {
			args = append(args, "-c")
		}
	}
	args = append(args, h.ExtraArgs...)
	args = append(args, "-p", userMsg)

	events, err := claude.RunStreaming(ctx, cwd, args)
	if err != nil {
		return nil, err
	}

	out := make(chan StreamMsg, 64)
	go func() {
		defer close(out)
		var sessionID string
		for event := range events {
			var msg StreamMsg
			switch event.Type {
			case claude.EventSessionID:
				sessionID = event.SessionID
				continue
			case claude.EventText:
				msg = TextDelta{Text: event.Text}
			case claude.EventThinkingStart:
				msg = ReasoningStart{}
			case claude.EventThinking:
				msg = ReasoningDelta{Text: event.Text}
			case claude.EventThinkingEnd:
				msg = ReasoningEnd{}
			case claude.EventToolUse:
				msg = ToolCallStart{Name: event.ToolName}
			case claude.EventResult:
				if event.IsError {
					msg = StreamError{Err: fmt.Errorf("%s", event.Text)}
				} else {
					msg = StreamDone{
						FinishReason: "stop",
						SessionID:    sessionID,
					}
				}
			case claude.EventError:
				msg = StreamError{Err: fmt.Errorf("%s", event.Text)}
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
func (h *ClaudeChatHandler) SetContinue(cont bool, sessionID string) {
	h.Continue = cont
	h.SessionID = sessionID
}

// ResetSession clears any continuation state for a new conversation.
func (h *ClaudeChatHandler) ResetSession() {
	h.Continue = false
	h.SessionID = ""
}
