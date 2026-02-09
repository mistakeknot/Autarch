package views

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/mistakeknot/autarch/pkg/autarch"
	"github.com/mistakeknot/autarch/pkg/claude"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

type gurgehSpecStore interface {
	GetSpec(id string) (autarch.Spec, error)
}

// GurgehChatHandler implements ChatHandler with spec-aware context.
type GurgehChatHandler struct {
	mu              sync.RWMutex
	specStore       gurgehSpecStore
	currentSpec     string
	cwd             string
	continueSession bool
	sessionID       string
}

// NewGurgehChatHandler creates a handler for the Gurgeh spec browser.
func NewGurgehChatHandler() *GurgehChatHandler {
	cwd, _ := os.Getwd()
	return &GurgehChatHandler{cwd: cwd}
}

// SetCurrentSpec updates the spec ID for context enrichment.
func (h *GurgehChatHandler) SetCurrentSpec(specID string) {
	h.mu.Lock()
	h.currentSpec = specID
	h.mu.Unlock()
}

// SetSpecStore sets the spec data source.
func (h *GurgehChatHandler) SetSpecStore(store gurgehSpecStore) {
	h.mu.Lock()
	h.specStore = store
	h.mu.Unlock()
}

// HandleMessage sends the user's message to Claude with spec context.
func (h *GurgehChatHandler) HandleMessage(ctx context.Context, userMsg string) (<-chan pkgtui.StreamMsg, error) {
	cwd := h.cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	systemCtx := h.buildSystemContext()
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
	if systemCtx != "" {
		args = append(args, "--system-prompt", systemCtx)
	}
	args = append(args, "-p", userMsg)

	events, err := claude.RunStreaming(ctx, cwd, args)
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
func (h *GurgehChatHandler) SetContinue(cont bool, sessionID string) {
	h.mu.Lock()
	h.continueSession = cont
	h.sessionID = sessionID
	h.mu.Unlock()
}

// ResetSession clears any continuation state for a new conversation.
func (h *GurgehChatHandler) ResetSession() {
	h.mu.Lock()
	h.continueSession = false
	h.sessionID = ""
	h.mu.Unlock()
}

func (h *GurgehChatHandler) buildSystemContext() string {
	const base = "You are a spec assistant for the Gurgeh PRD tool. Help the user create and refine product specifications."

	h.mu.RLock()
	specID := h.currentSpec
	store := h.specStore
	h.mu.RUnlock()

	if strings.TrimSpace(specID) == "" {
		return base
	}

	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\n")
	fmt.Fprintf(&b, "The user is viewing spec: %s.\n", specID)
	b.WriteString("Answer questions about this spec. Suggest improvements. Help refine acceptance criteria.\n")

	if store == nil {
		return strings.TrimSpace(b.String())
	}

	spec, err := store.GetSpec(specID)
	if err != nil {
		return strings.TrimSpace(b.String())
	}

	b.WriteString("\nCurrent spec context:\n")
	fmt.Fprintf(&b, "- ID: %s\n", spec.ID)
	if spec.Title != "" {
		fmt.Fprintf(&b, "- Title: %s\n", spec.Title)
	}
	if spec.Status != "" {
		fmt.Fprintf(&b, "- Status: %s\n", spec.Status)
	}
	if spec.Project != "" {
		fmt.Fprintf(&b, "- Project: %s\n", spec.Project)
	}

	if spec.Vision != "" {
		b.WriteString("\nVision:\n")
		b.WriteString(spec.Vision)
		b.WriteString("\n")
	}
	if spec.Problem != "" {
		b.WriteString("\nProblem:\n")
		b.WriteString(spec.Problem)
		b.WriteString("\n")
	}
	if spec.Users != "" {
		b.WriteString("\nUsers:\n")
		b.WriteString(spec.Users)
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}
