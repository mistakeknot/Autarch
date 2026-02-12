package exploration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/agenttargets"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Explore runs an agent and returns parsed output plus session ID.
// Session ID can be used with GeneratePhase() to avoid re-exploration.
// Tool usage is streamed to slog (appears in log pane when TUI is running).
func Explore(ctx context.Context, cwd string) (map[string]any, string, error) {
	slog.Info("exploration starting", "path", cwd)

	cfg := agenttargets.DefaultDispatchConfig()
	cfg.Timeout = 10 * time.Minute

	handle, err := agenttargets.Dispatch(ctx, cfg, cwd, prompt)
	if err != nil {
		return nil, "", err
	}

	// Process events: capture session ID, log tool usage, capture result.
	var sessionID string
	var finalResult string
	var isError bool
	for event := range handle.Events {
		switch event.Type {
		case agenttargets.StreamSessionID:
			if sessionID == "" {
				sessionID = event.SessionID
				slog.Info("exploration session", "id", sessionID)
			}
		case agenttargets.StreamToolUse:
			logToolUse(event.ToolName, event.ToolInput)
		case agenttargets.StreamResult:
			finalResult = event.Text
			isError = event.IsError
		case agenttargets.StreamError:
			slog.Warn("exploration stream error", "err", event.Text)
		}
	}

	if isError {
		return nil, "", fmt.Errorf("agent returned error: %s", finalResult)
	}

	slog.Info("exploration complete")

	// Parse the result JSON (stream-json gives us the result directly)
	var result map[string]any
	if err := json.Unmarshal([]byte(finalResult), &result); err == nil {
		return result, sessionID, nil
	}

	// Try extracting JSON from markdown code fence
	extracted := extractJSONFromMarkdown(finalResult)
	if extracted != "" {
		if err := json.Unmarshal([]byte(extracted), &result); err == nil {
			return result, sessionID, nil
		}
	}

	// Fallback: return raw text
	return map[string]any{"raw": finalResult}, sessionID, nil
}

// GeneratePhase asks an agent to generate content for a specific phase.
// If sessionID is non-empty, resumes that session to avoid re-exploring.
// Falls back to fresh exploration if resumed session fails.
func GeneratePhase(ctx context.Context, cwd string, phase string,
	priorContext map[string]string, sessionID string, researchContext string, model ...string) (string, error) {

	// Build context from prior phases
	var contextParts []string
	titler := cases.Title(language.English)
	phaseOrder := []string{"vision", "problem", "users", "features", "cujs", "requirements", "scope", "acceptance"}
	for _, p := range phaseOrder {
		if content, ok := priorContext[p]; ok && content != "" {
			contextParts = append(contextParts, fmt.Sprintf("## %s\n%s", titler.String(p), content))
		}
	}
	priorContextStr := strings.Join(contextParts, "\n\n")
	researchContext = strings.TrimSpace(researchContext)
	researchContextStr := ""
	if researchContext != "" {
		researchContextStr = fmt.Sprintf("\nRESEARCH FINDINGS:\n%s\n", researchContext)
	}

	// Choose prompt based on whether we have a session to resume
	var phasePrompt string
	if sessionID != "" {
		phasePrompt = fmt.Sprintf(`Generate the %s section for this PRD.

You already explored this codebase. Use that knowledge.

PRIOR SECTIONS:
%s
%s

Be specific to THIS project. 2-4 paragraphs max. No placeholders.
Return ONLY the section content.`, phase, priorContextStr, researchContextStr)
	} else {
		phasePrompt = fmt.Sprintf(`Generate content for the %s section of a PRD.

Explore this codebase to understand what it does, then write the %s section.

PRIOR CONTEXT (approved phases):
%s
%s

Be concise and specific to THIS project. Extract evidence from the codebase.
2-4 paragraphs max. Return ONLY the section content.`, phase, phase, priorContextStr, researchContextStr)
	}

	slog.Info("generating phase", "phase", phase, "resumed", sessionID != "")

	cfg := agenttargets.DefaultDispatchConfig()
	cfg.Timeout = 5 * time.Minute
	if sessionID != "" {
		cfg.SessionID = sessionID
	}
	if len(model) > 0 && model[0] != "" {
		cfg.Model = model[0]
	}

	result, err := dispatchAndCollect(ctx, cfg, cwd, phasePrompt)

	// If resumed session failed, retry without session
	if err != nil && sessionID != "" {
		slog.Warn("session resume failed, retrying fresh", "phase", phase, "err", err)
		cfg.SessionID = ""
		result, err = dispatchAndCollect(ctx, cfg, cwd, phasePrompt)
	}

	return result, err
}

// GeneratePhaseFromContext generates phase content using cached exploration context.
// This avoids re-scanning the codebase when we already have exploration data but
// the specific phase wasn't included or needs regeneration.
func GeneratePhaseFromContext(ctx context.Context, cwd string, phase string,
	priorContext map[string]string, explorationCtx map[string]any, researchContext string, model ...string) (string, error) {

	// Build exploration summary from cached data
	var explorationParts []string
	for key, data := range explorationCtx {
		if m, ok := data.(map[string]any); ok {
			if summary, ok := m["summary"].(string); ok && summary != "" {
				explorationParts = append(explorationParts,
					fmt.Sprintf("### %s\n%s", cases.Title(language.English).String(key), summary))
			}
		}
	}

	// Build prior context from accepted phases
	var priorParts []string
	phaseOrder := []string{"vision", "problem", "users", "features", "cujs", "requirements", "scope", "acceptance"}
	for _, p := range phaseOrder {
		if content, ok := priorContext[p]; ok && content != "" {
			priorParts = append(priorParts, fmt.Sprintf("## %s\n%s", cases.Title(language.English).String(p), content))
		}
	}

	priorContextStr := ""
	if len(priorParts) > 0 {
		priorContextStr = fmt.Sprintf("\nPRIOR PHASES (approved):\n%s\n", strings.Join(priorParts, "\n\n"))
	}
	researchContext = strings.TrimSpace(researchContext)
	researchContextStr := ""
	if researchContext != "" {
		researchContextStr = fmt.Sprintf("\nRESEARCH FINDINGS:\n%s\n", researchContext)
	}

	explorationContextStr := ""
	if len(explorationParts) > 0 {
		explorationContextStr = fmt.Sprintf("\nCODEBASE CONTEXT (from exploration):\n%s\n", strings.Join(explorationParts, "\n\n"))
	}

	phasePrompt := fmt.Sprintf(`Generate the %s section of a PRD.
%s%s%s
Guidelines:
- Be concise and specific to THIS project
- Use the codebase context and prior phases as your source of truth
- Incorporate relevant research findings when they materially affect the section
- No generic placeholder content
- 2-4 paragraphs max

Return ONLY the section content, no headers or markdown fences.`, phase, explorationContextStr, priorContextStr, researchContextStr)

	slog.Info("generating phase from context", "phase", phase)

	cfg := agenttargets.DefaultDispatchConfig()
	cfg.Timeout = 3 * time.Minute
	if len(model) > 0 && model[0] != "" {
		cfg.Model = model[0]
	}

	result, err := dispatchAndCollect(ctx, cfg, cwd, phasePrompt)
	if err != nil {
		return "", err
	}

	slog.Info("phase generation from context complete", "phase", phase)
	return result, nil
}

// Revise takes a spec section and user feedback, returns a revised version.
// This runs an agent to intelligently revise the content based on feedback.
func Revise(ctx context.Context, cwd string, phase string, currentContent string, feedback string, model ...string) (string, error) {
	revisePrompt := fmt.Sprintf(`Revise this %s section based on user feedback.

CURRENT CONTENT:
%s

USER FEEDBACK:
%s

Revise the content to address the feedback. Keep it concise and focused.
Return ONLY the revised content, no explanation or markdown fences.`, phase, currentContent, feedback)

	slog.Info("revising spec", "phase", phase)

	cfg := agenttargets.DefaultDispatchConfig()
	cfg.Timeout = 5 * time.Minute
	if len(model) > 0 && model[0] != "" {
		cfg.Model = model[0]
	}

	result, err := dispatchAndCollect(ctx, cfg, cwd, revisePrompt)
	if err != nil {
		return "", err
	}

	slog.Info("revision complete")
	return result, nil
}

// PhaseUpdate represents a single phase's content update from propagation.
type PhaseUpdate struct {
	Phase   string
	Content string
	Changed bool // true if content differs from previous
}

// PropagateChanges regenerates all phases in a single agent call.
// It takes the current phase content map and user feedback (if any), then returns
// updated content for all phases. The agent decides which phases need changes
// based on the feedback and maintains consistency across the spec.
//
// This is more efficient than calling GeneratePhase multiple times because:
// 1. Single agent invocation (one context load)
// 2. Agent sees the full spec and can make intelligent decisions about what to update
// 3. Returns only changed content (phases that don't need changes return unchanged)
func PropagateChanges(ctx context.Context, cwd string, currentPhases map[string]string, changedPhase string, feedback string, researchContext string) (map[string]PhaseUpdate, error) {
	// Build the current spec state
	phaseOrder := []string{"vision", "problem", "users", "features", "cujs", "requirements", "scope", "acceptance"}
	var specParts []string
	for _, p := range phaseOrder {
		if content, ok := currentPhases[p]; ok && content != "" {
			specParts = append(specParts, fmt.Sprintf("## %s\n%s", cases.Title(language.English).String(p), content))
		}
	}
	currentSpec := strings.Join(specParts, "\n\n")
	researchContext = strings.TrimSpace(researchContext)
	researchContextStr := ""
	if researchContext != "" {
		researchContextStr = fmt.Sprintf("\nRESEARCH FINDINGS:\n%s\n", researchContext)
	}

	propagatePrompt := fmt.Sprintf(`You are updating a PRD (Product Requirements Document) after changes.

CURRENT SPEC:
%s
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
- If research findings introduce constraints or risks, reflect them across affected sections

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
- Use research findings where relevant and do not invent unsupported claims
- Only include phases that actually changed
- If a phase doesn't need changes, don't include it`, currentSpec, researchContextStr, changedPhase, feedback, changedPhase, changedPhase)

	slog.Info("propagating changes", "changed_phase", changedPhase)

	cfg := agenttargets.DefaultDispatchConfig()
	cfg.Timeout = 8 * time.Minute

	finalResult, err := dispatchAndCollect(ctx, cfg, cwd, propagatePrompt)
	if err != nil {
		return nil, err
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

// dispatchAndCollect launches an agent and collects the result, logging tool usage.
// This is the shared implementation that replaces the copy-pasted scanner loops.
func dispatchAndCollect(ctx context.Context, cfg agenttargets.DispatchConfig, cwd, prompt string) (string, error) {
	handle, err := agenttargets.Dispatch(ctx, cfg, cwd, prompt)
	if err != nil {
		return "", err
	}

	var finalResult string
	var isError bool
	for event := range handle.Events {
		switch event.Type {
		case agenttargets.StreamToolUse:
			logToolUse(event.ToolName, event.ToolInput)
		case agenttargets.StreamResult:
			finalResult = event.Text
			isError = event.IsError
		case agenttargets.StreamError:
			slog.Warn("agent stream error", "err", event.Text)
		}
	}

	if isError {
		return "", fmt.Errorf("agent returned error: %s", finalResult)
	}

	return strings.TrimSpace(finalResult), nil
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
- CUJs: 2-3 critical user journeys (e.g., "User creates account and verifies email")
- Requirements: Key requirements implied by the code (functional and non-functional)
- Scope: What's in scope vs out of scope (based on TODOs, comments, roadmap)
- Acceptance: Acceptance criteria implied by tests, docs, or code comments
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
  "cujs": {"summary": "...", "journeys": ["Journey 1", "Journey 2"], "evidence": [{"quote": "...", "source": "file:line"}]},
  "requirements": {"summary": "...", "items": ["Requirement 1", "Requirement 2"], "evidence": [{"quote": "...", "source": "file:line"}]},
  "scope": {"summary": "...", "in_scope": ["Item 1"], "out_of_scope": ["Item 2"], "evidence": [{"quote": "...", "source": "file:line"}]},
  "acceptance": {"summary": "...", "criteria": ["Criterion 1", "Criterion 2"], "evidence": [{"quote": "...", "source": "file:line"}]},
  "tech": {"summary": "...", "evidence": [{"quote": "...", "source": "file:line"}]},
  "risks": {"summary": "...", "evidence": [{"quote": "...", "source": "file:line"}]}
}`
