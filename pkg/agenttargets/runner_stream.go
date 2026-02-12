package agenttargets

import (
	"context"
	"fmt"
)

// StreamingRunner launches agent processes with streaming event output.
type StreamingRunner interface {
	RunStreaming(ctx context.Context, cfg DispatchConfig, workDir, prompt string) (*StreamHandle, error)
}

// Dispatch is the primary entry point for launching an agent.
// It auto-detects the available backend (preferring Claude, then Codex)
// using the cached multi-method detector, applies the config, and returns
// a StreamHandle for consuming events.
func Dispatch(ctx context.Context, cfg DispatchConfig, workDir, prompt string) (*StreamHandle, error) {
	agent := cfg.PreferredAgent
	if agent == "" {
		if tool, found := DetectPreferredTool(ctx); found {
			agent = tool.Name
		}
	}

	switch agent {
	case "claude":
		backend := &ClaudeBackend{}
		return backend.RunStreaming(ctx, cfg, workDir, prompt)
	case "codex":
		backend := &CodexBackend{}
		return backend.RunStreaming(ctx, cfg, workDir, prompt)
	default:
		return nil, fmt.Errorf("no supported agent available (tried claude, codex)")
	}
}

// CollectResult consumes a StreamHandle and returns the final result text.
// This is a convenience function for callers that don't need streaming.
func CollectResult(handle *StreamHandle) (string, string, error) {
	var resultText string
	var sessionID string
	var resultIsError bool
	var sawResult bool
	var firstErr string

	for event := range handle.Events {
		switch event.Type {
		case StreamResult:
			resultText = event.Text
			resultIsError = event.IsError
			sawResult = true
		case StreamSessionID:
			if sessionID == "" {
				sessionID = event.SessionID
			}
		case StreamError:
			if firstErr == "" {
				firstErr = event.Text
			}
		}
	}

	if resultIsError {
		return "", sessionID, fmt.Errorf("agent returned error: %s", resultText)
	}
	if sawResult {
		return resultText, sessionID, nil
	}
	if firstErr != "" {
		return "", sessionID, fmt.Errorf("%s", firstErr)
	}
	return "", sessionID, nil
}
