// Package agent provides detection and execution of coding agents (Claude Code, Codex CLI).
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
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
// Preference order: claude > codex
func DetectAgent() (*Agent, error) {
	// Try Claude Code first
	if path, err := exec.LookPath("claude"); err == nil {
		version := getVersion(path, "--version")
		return &Agent{
			Type:    TypeClaude,
			Path:    path,
			Version: version,
		}, nil
	}

	// Try Codex CLI
	if path, err := exec.LookPath("codex"); err == nil {
		version := getVersion(path, "--version")
		return &Agent{
			Type:    TypeCodex,
			Path:    path,
			Version: version,
		}, nil
	}

	return nil, &NoAgentError{}
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

func getVersion(path, flag string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, flag)
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
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
func (a *Agent) GenerateWithOutput(ctx context.Context, req GenerateRequest, onOutput OutputCallback) (*GenerateResponse, error) {
	switch a.Type {
	case TypeClaude:
		return a.generateClaudeStreaming(ctx, req, onOutput)
	case TypeCodex:
		return a.generateCodexStreaming(ctx, req, onOutput)
	default:
		return nil, fmt.Errorf("unsupported agent type: %s", a.Type)
	}
}

func (a *Agent) generateClaudeStreaming(ctx context.Context, req GenerateRequest, onOutput OutputCallback) (*GenerateResponse, error) {
	// Claude Code CLI: claude -p "prompt" --output-format json
	args := []string{
		"-p", req.Prompt,
		"--output-format", "json",
	}

	cmd := exec.CommandContext(ctx, a.Path, args...)

	// If we have an output callback, stream stderr (where claude shows progress)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout

	if onOutput != nil {
		// Create a pipe to read stderr line by line
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
		}

		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("failed to start claude: %w", err)
		}

		// Read stderr line by line and send to callback
		go func() {
			buf := make([]byte, 1024)
			var line strings.Builder
			for {
				n, err := stderrPipe.Read(buf)
				if n > 0 {
					for _, b := range buf[:n] {
						if b == '\n' || b == '\r' {
							if line.Len() > 0 {
								onOutput(line.String())
								line.Reset()
							}
						} else {
							line.WriteByte(b)
						}
					}
					// Also send partial lines for real-time feedback
					if line.Len() > 0 {
						onOutput(line.String())
					}
				}
				if err != nil {
					break
				}
			}
		}()

		if err := cmd.Wait(); err != nil {
			return nil, fmt.Errorf("claude execution failed: %w", err)
		}
	} else {
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("claude execution failed: %w\nstderr: %s", err, stderr.String())
		}
	}

	// Parse JSON output
	var result struct {
		Result string `json:"result"`
		Error  string `json:"error,omitempty"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		// If not JSON, treat stdout as plain text
		return &GenerateResponse{
			Content: stdout.String(),
		}, nil
	}

	if result.Error != "" {
		return nil, fmt.Errorf("claude error: %s", result.Error)
	}

	return &GenerateResponse{
		Content: result.Result,
	}, nil
}

func (a *Agent) generateCodexStreaming(ctx context.Context, req GenerateRequest, onOutput OutputCallback) (*GenerateResponse, error) {
	// Codex CLI: codex -q "prompt"
	args := []string{
		"-q", req.Prompt,
	}

	cmd := exec.CommandContext(ctx, a.Path, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout

	if onOutput != nil {
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
		}

		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("failed to start codex: %w", err)
		}

		// Read stderr and send to callback
		go func() {
			buf := make([]byte, 1024)
			var line strings.Builder
			for {
				n, err := stderrPipe.Read(buf)
				if n > 0 {
					for _, b := range buf[:n] {
						if b == '\n' || b == '\r' {
							if line.Len() > 0 {
								onOutput(line.String())
								line.Reset()
							}
						} else {
							line.WriteByte(b)
						}
					}
					if line.Len() > 0 {
						onOutput(line.String())
					}
				}
				if err != nil {
					break
				}
			}
		}()

		if err := cmd.Wait(); err != nil {
			return nil, fmt.Errorf("codex execution failed: %w", err)
		}
	} else {
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("codex execution failed: %w\nstderr: %s", err, stderr.String())
		}
	}

	return &GenerateResponse{
		Content: stdout.String(),
	}, nil
}

// String returns a display string for the agent
func (a *Agent) String() string {
	if a == nil {
		return "none"
	}
	return fmt.Sprintf("%s (%s)", a.Type, a.Version)
}
