// Package agent provides detection and execution of coding agents (Claude Code, Codex CLI).
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/mistakeknot/autarch/pkg/agenttargets"
)

// Type represents the type of coding agent
type Type string

const (
	TypeClaude Type = "claude"
	TypeCodex  Type = "codex"
	TypeNone   Type = "none"
)

// Agent represents a detected coding agent
type Agent struct {
	Type    Type
	Path    string
	Version string
}

// DetectAgent finds available coding agents on the system.
// Preference order: claude > codex.
// Uses the cached multi-method detector from pkg/agenttargets.
func DetectAgent() (*Agent, error) {
	ctx := context.Background()
	tool, found := agenttargets.DetectPreferredTool(ctx)
	if !found {
		return nil, &NoAgentError{}
	}
	return &Agent{
		Type:    Type(tool.Name),
		Path:    tool.Path,
		Version: tool.Version,
	}, nil
}

// DetectAgentByName finds the requested agent by name.
// The lookPath parameter is kept for backward compatibility but is no longer
// used — detection now goes through the cached multi-method detector.
func DetectAgentByName(name string, lookPath func(string) (string, error)) (*Agent, error) {
	ctx := context.Background()
	normalized := strings.ToLower(name)
	tool, found := agenttargets.DetectTool(ctx, normalized)
	if !found {
		return nil, fmt.Errorf("agent %q not found", name)
	}
	return &Agent{
		Type:    Type(tool.Name),
		Path:    tool.Path,
		Version: tool.Version,
	}, nil
}

// NoAgentError indicates no coding agent was found
type NoAgentError struct{}

func (e *NoAgentError) Error() string {
	return "no coding agent found"
}

// Instructions returns installation instructions
func (e *NoAgentError) Instructions() string {
	return `No coding agent found. Please install one of:

1. Claude Code (recommended):
   npm install -g @anthropic-ai/claude-code

2. Codex CLI:
   npm install -g @openai/codex

Alternatively, set ANTHROPIC_API_KEY or OPENAI_API_KEY
environment variable to use direct API calls.`
}


// GenerateRequest represents a request to generate content via an agent
type GenerateRequest struct {
	Prompt      string
	MaxTokens   int
	Temperature float64
}

// GenerateResponse represents the agent's response
type GenerateResponse struct {
	Content string
	Error   error
}

// OutputCallback is called with each line of output from the agent.
type OutputCallback func(line string)

// Generate runs a prompt through the detected agent
func (a *Agent) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	return a.GenerateWithOutput(ctx, req, nil)
}

// GenerateWithOutput runs a prompt and streams output to a callback.
// This dispatches via agenttargets.Dispatch() regardless of agent type.
func (a *Agent) GenerateWithOutput(ctx context.Context, req GenerateRequest, onOutput OutputCallback) (*GenerateResponse, error) {
	cfg := agenttargets.DefaultDispatchConfig()
	cfg.PreferredAgent = string(a.Type)

	handle, err := agenttargets.Dispatch(ctx, cfg, "", req.Prompt)
	if err != nil {
		return nil, fmt.Errorf("%s execution failed: %w", a.Type, err)
	}

	var finalResult string
	var contentBuilder strings.Builder
	var sawResult bool

	for event := range handle.Events {
		switch event.Type {
		case agenttargets.StreamText:
			contentBuilder.WriteString(event.Text)
			if onOutput != nil {
				onOutput(event.Text)
			}
		case agenttargets.StreamSessionID:
			if onOutput != nil {
				onOutput("Session started...")
			}
		case agenttargets.StreamResult:
			sawResult = true
			if event.IsError {
				return nil, fmt.Errorf("%s error: %s", a.Type, event.Text)
			}
			finalResult = event.Text
		case agenttargets.StreamError:
			if !sawResult {
				return nil, fmt.Errorf("%s execution failed: %s", a.Type, event.Text)
			}
		}
	}

	// Prefer the explicit result if available, otherwise use accumulated content
	content := finalResult
	if content == "" {
		content = contentBuilder.String()
	}

	return &GenerateResponse{
		Content: content,
	}, nil
}

// String returns a display string for the agent
func (a *Agent) String() string {
	if a == nil {
		return "none"
	}
	return fmt.Sprintf("%s (%s)", a.Type, a.Version)
}
