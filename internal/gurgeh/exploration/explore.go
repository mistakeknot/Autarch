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

// GeneratePhase asks Claude Code to generate content for a specific phase.
// It uses prior phase content as context to maintain consistency.
func GeneratePhase(ctx context.Context, cwd string, phase string, priorContext map[string]string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Build context from prior phases
	var contextParts []string
	phaseOrder := []string{"vision", "problem", "users", "features", "ux", "tech", "risks", "mvp"}
	for _, p := range phaseOrder {
		if content, ok := priorContext[p]; ok && content != "" {
			contextParts = append(contextParts, fmt.Sprintf("## %s\n%s", strings.Title(p), content))
		}
	}

	priorContextStr := ""
	if len(contextParts) > 0 {
		priorContextStr = fmt.Sprintf("\n\nPRIOR CONTEXT (approved phases):\n%s\n", strings.Join(contextParts, "\n\n"))
	}

	phasePrompt := fmt.Sprintf(`Generate content for the %s section of a PRD.

Explore this codebase to understand what it does, then write the %s section.
%s
Guidelines:
- Be concise and specific to THIS project
- Extract evidence from the actual codebase
- No generic placeholder content
- 2-4 paragraphs max

Return ONLY the section content, no headers or markdown fences.`, phase, phase, priorContextStr)

	slog.Info("generating phase", "phase", phase)

	cmd := exec.CommandContext(ctx, "claude",
		"-p", phasePrompt,
		"--output-format", "stream-json",
		"--verbose",
		"--print",
	)
	cmd.Dir = cwd

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start claude: %w", err)
	}

	// Parse streaming output, log tool usage, capture result
	var finalResult string
	var isError bool
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg streamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
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
		return "", fmt.Errorf("claude failed: %w", err)
	}

	if isError {
		return "", fmt.Errorf("claude returned error: %s", finalResult)
	}

	slog.Info("phase generation complete", "phase", phase)
	return strings.TrimSpace(finalResult), nil
}

// Revise takes a spec section and user feedback, returns a revised version.
// This runs Claude Code to intelligently revise the content based on feedback.
func Revise(ctx context.Context, cwd string, phase string, currentContent string, feedback string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	revisePrompt := fmt.Sprintf(`Revise this %s section based on user feedback.

CURRENT CONTENT:
%s

USER FEEDBACK:
%s

Revise the content to address the feedback. Keep it concise and focused.
Return ONLY the revised content, no explanation or markdown fences.`, phase, currentContent, feedback)

	slog.Info("revising spec", "phase", phase)

	cmd := exec.CommandContext(ctx, "claude",
		"-p", revisePrompt,
		"--output-format", "stream-json",
		"--verbose",
		"--print",
	)
	cmd.Dir = cwd

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start claude: %w", err)
	}

	// Parse streaming output, log tool usage, capture result
	var finalResult string
	var isError bool
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg streamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
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
		return "", fmt.Errorf("claude failed: %w", err)
	}

	if isError {
		return "", fmt.Errorf("claude returned error: %s", finalResult)
	}

	slog.Info("revision complete")
	return strings.TrimSpace(finalResult), nil
}

// PhaseUpdate represents a single phase's content update from propagation.
type PhaseUpdate struct {
	Phase   string
	Content string
	Changed bool // true if content differs from previous
}

// PropagateChanges regenerates all phases in a single Claude Code call.
// It takes the current phase content map and user feedback (if any), then returns
// updated content for all phases. The agent decides which phases need changes
// based on the feedback and maintains consistency across the spec.
//
// This is more efficient than calling GeneratePhase multiple times because:
// 1. Single Claude Code invocation (one context load)
// 2. Agent sees the full spec and can make intelligent decisions about what to update
// 3. Returns only changed content (phases that don't need changes return unchanged)
func PropagateChanges(ctx context.Context, cwd string, currentPhases map[string]string, changedPhase string, feedback string) (map[string]PhaseUpdate, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()

	// Build the current spec state
	phaseOrder := []string{"vision", "problem", "users", "features", "requirements", "scope", "cujs", "acceptance"}
	var specParts []string
	for _, p := range phaseOrder {
		if content, ok := currentPhases[p]; ok && content != "" {
			specParts = append(specParts, fmt.Sprintf("## %s\n%s", strings.Title(p), content))
		}
	}
	currentSpec := strings.Join(specParts, "\n\n")

	propagatePrompt := fmt.Sprintf(`You are updating a PRD (Product Requirements Document) after changes.

CURRENT SPEC:
%s

CHANGED PHASE: %s
USER FEEDBACK: %s

Your task:
1. First, explore this codebase to understand the project
2. Apply the user's feedback to the %s section
3. Review ALL other sections - if the change to %s affects them, update them too
4. Return ONLY sections that changed (to save tokens)

For example:
- If "features" changes, "requirements" and "cujs" likely need updates
- If "users" changes, "cujs" (user journeys) likely need updates
- If "vision" changes, everything might need review

Return JSON with ONLY the phases that need changes:
{
  "updates": {
    "phase_name": "new content for this phase",
    "another_phase": "new content if it changed"
  }
}

Guidelines:
- Be concise and specific to THIS project
- Maintain consistency across all sections
- Only include phases that actually changed
- If a phase doesn't need changes, don't include it`, currentSpec, changedPhase, feedback, changedPhase, changedPhase)

	slog.Info("propagating changes", "changed_phase", changedPhase)

	cmd := exec.CommandContext(ctx, "claude",
		"-p", propagatePrompt,
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
		return nil, fmt.Errorf("failed to start claude: %w", err)
	}

	// Parse streaming output, log tool usage, capture result
	var finalResult string
	var isError bool
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg streamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
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

	// Parse the JSON response
	var response struct {
		Updates map[string]string `json:"updates"`
	}

	// Try direct JSON parse
	if err := json.Unmarshal([]byte(finalResult), &response); err != nil {
		// Try extracting from markdown code fence
		extracted := extractJSONFromMarkdown(finalResult)
		if extracted != "" {
			if err := json.Unmarshal([]byte(extracted), &response); err != nil {
				return nil, fmt.Errorf("failed to parse response JSON: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
	}

	// Convert to PhaseUpdate map
	updates := make(map[string]PhaseUpdate)
	for phase, content := range response.Updates {
		oldContent := currentPhases[phase]
		updates[phase] = PhaseUpdate{
			Phase:   phase,
			Content: content,
			Changed: content != oldContent,
		}
	}

	slog.Info("propagation complete", "phases_updated", len(updates))
	return updates, nil
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
// Attributes are embedded in the message since the log pane only shows messages.
func logToolUse(toolName string, input map[string]any) {
	switch toolName {
	case "Read":
		if path, ok := input["file_path"].(string); ok {
			slog.Info("📖 Read " + truncatePath(path, 60))
		}
	case "Grep":
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		if path == "" {
			path = "."
		}
		slog.Info(fmt.Sprintf("🔍 Grep %q in %s", truncate(pattern, 30), truncatePath(path, 30)))
	case "Glob":
		pattern, _ := input["pattern"].(string)
		slog.Info("📁 Glob " + pattern)
	case "Bash":
		if desc, ok := input["description"].(string); ok {
			slog.Info("💻 " + truncate(desc, 60))
		} else if cmd, ok := input["command"].(string); ok {
			slog.Info("💻 " + truncate(cmd, 60))
		}
	case "LS":
		path, _ := input["path"].(string)
		if path == "" {
			path = "."
		}
		slog.Info("📂 LS " + truncatePath(path, 60))
	case "Task":
		// Subagent invocation
		desc, _ := input["description"].(string)
		agentType, _ := input["subagent_type"].(string)
		if desc != "" {
			slog.Info(fmt.Sprintf("🤖 Task(%s) %s", agentType, truncate(desc, 50)))
		} else {
			slog.Info("🤖 Task(" + agentType + ")")
		}
	default:
		slog.Info("🔧 " + toolName)
	}
}

// truncate shortens a string to max length with ellipsis.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// truncatePath shortens a path, keeping the filename visible.
func truncatePath(path string, max int) string {
	if len(path) <= max {
		return path
	}
	// Keep the last part (filename) and truncate the beginning
	if idx := strings.LastIndex(path, "/"); idx > 0 && len(path)-idx < max-3 {
		remaining := max - 3 - (len(path) - idx)
		if remaining > 0 {
			return path[:remaining] + "..." + path[idx:]
		}
	}
	return "..." + path[len(path)-max+3:]
}

const prompt = `Explore this codebase for PRD generation.

Find ALL of the following by reading code, docs, and config:
- Project name: What is this project called?
- Vision: What does this project do? Why does it exist?
- Problem: What pain points does it solve?
- Users: Who uses this? What are their workflows?
- Features: What are the main capabilities/features?
- Tech: What technologies, frameworks, and architecture patterns are used?
- Risks: What are potential technical risks or challenges?

Extract VERBATIM QUOTES as evidence. Skip .env files.

Return JSON:
{
  "project_name": "Name of the project",
  "vision": {"summary": "...", "evidence": [{"quote": "...", "source": "file:line"}]},
  "problem": {"summary": "...", "evidence": [{"quote": "...", "source": "file:line"}]},
  "users": {"summary": "...", "evidence": [{"quote": "...", "source": "file:line"}]},
  "features": {"summary": "...", "evidence": [{"quote": "...", "source": "file:line"}]},
  "tech": {"summary": "...", "evidence": [{"quote": "...", "source": "file:line"}]},
  "risks": {"summary": "...", "evidence": [{"quote": "...", "source": "file:line"}]}
}`
