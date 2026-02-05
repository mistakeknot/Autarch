package exploration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Explore runs Claude Code and returns parsed output.
// Returns map[string]any - don't define types until we see real output.
// Tool usage is streamed to slog (appears in log pane when TUI is running).
func Explore(ctx context.Context, cwd string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	slog.Info("exploration starting", "path", cwd)

	cmd := exec.CommandContext(ctx, "claude",
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--print",
	)
	cmd.Dir = cwd

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return nil, fmt.Errorf("claude CLI not found: install with 'npm install -g @anthropic-ai/claude-code'")
		}
		return nil, fmt.Errorf("failed to start claude: %w", err)
	}

	// Parse streaming JSON output, log tool usage, capture final result
	var finalResult string
	var isError bool
	scanner := bufio.NewScanner(stdout)
	// Increase buffer size for large JSON lines
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg streamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip malformed lines
		}

		// Log tool usage
		if msg.Type == "assistant" && msg.Message != nil {
			for _, content := range msg.Message.Content {
				if content.Type == "tool_use" {
					logToolUse(content.Name, content.Input)
				}
			}
		}

		// Capture final result
		if msg.Type == "result" {
			finalResult = msg.Result
			isError = msg.IsError
		}
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("claude failed: %w", err)
	}

	if isError {
		return nil, fmt.Errorf("claude returned error: %s", finalResult)
	}

	slog.Info("exploration complete")

	// Parse the result JSON (stream-json gives us the result directly)
	var result map[string]any
	if err := json.Unmarshal([]byte(finalResult), &result); err == nil {
		return result, nil
	}

	// Try extracting JSON from markdown code fence
	extracted := extractJSONFromMarkdown(finalResult)
	if extracted != "" {
		if err := json.Unmarshal([]byte(extracted), &result); err == nil {
			return result, nil
		}
	}

	// Fallback: return raw text
	return map[string]any{"raw": finalResult}, nil
}

// extractJSONFromMarkdown extracts JSON content from markdown code fences.
// Handles ```json ... ``` and ``` ... ``` patterns.
var jsonFenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(\\{.*?\\})\\s*```")

func extractJSONFromMarkdown(text string) string {
	// Try regex extraction first
	matches := jsonFenceRe.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Fallback: look for first { to last }
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}

	return ""
}

// streamMessage represents a line from Claude's stream-json output.
type streamMessage struct {
	Type    string         `json:"type"`
	Result  string         `json:"result"`
	IsError bool           `json:"is_error"`
	Message *streamContent `json:"message"`
}

type streamContent struct {
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type  string         `json:"type"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// logToolUse logs a tool invocation in a human-readable format.
func logToolUse(toolName string, input map[string]any) {
	switch toolName {
	case "Read":
		if path, ok := input["file_path"].(string); ok {
			slog.Info("📖 Read", "file", path)
		}
	case "Grep":
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		if path == "" {
			path = "."
		}
		slog.Info("🔍 Grep", "pattern", pattern, "path", path)
	case "Glob":
		pattern, _ := input["pattern"].(string)
		slog.Info("📁 Glob", "pattern", pattern)
	case "Bash":
		if desc, ok := input["description"].(string); ok {
			slog.Info("💻 Bash", "desc", desc)
		} else if cmd, ok := input["command"].(string); ok {
			// Truncate long commands
			if len(cmd) > 50 {
				cmd = cmd[:47] + "..."
			}
			slog.Info("💻 Bash", "cmd", cmd)
		}
	case "LS":
		path, _ := input["path"].(string)
		if path == "" {
			path = "."
		}
		slog.Info("📂 LS", "path", path)
	default:
		slog.Info("🔧 "+toolName, "input", input)
	}
}

const prompt = `Explore this codebase for PRD generation.

Find:
- Vision: What does this project do? Why does it exist?
- Problem: What pain points does it solve?
- Users: Who uses this?

Extract VERBATIM QUOTES as evidence. Skip .env files.

Return JSON: {"vision": {...}, "problem": {...}, "users": {...}}`
