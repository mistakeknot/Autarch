package exploration

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ProgressFunc reports exploration progress.
type ProgressFunc func(step, details string)

// Explore runs Claude Code and returns parsed output.
// Returns map[string]any - don't define types until we see real output.
func Explore(ctx context.Context, cwd string) (map[string]any, error) {
	return ExploreWithProgress(ctx, cwd, nil)
}

// ExploreWithProgress runs Claude Code exploration with progress reporting.
func ExploreWithProgress(ctx context.Context, cwd string, progress ProgressFunc) (map[string]any, error) {
	report := func(step, details string) {
		if progress != nil {
			progress(step, details)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	report("Starting", "Launching Claude Code exploration...")

	cmd := exec.CommandContext(ctx, "claude",
		"-p", prompt,
		"--output-format", "json",
		"--print",
	)
	cmd.Dir = cwd

	report("Exploring", "Claude is analyzing the codebase...")

	out, err := cmd.Output()
	if err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return nil, fmt.Errorf("claude CLI not found: install with 'npm install -g @anthropic-ai/claude-code'")
		}
		return nil, fmt.Errorf("claude failed: %w", err)
	}

	report("Parsing", "Processing exploration results...")

	// Parse the outer result envelope
	var envelope struct {
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		return nil, fmt.Errorf("parse envelope failed: %w", err)
	}
	if envelope.IsError {
		return nil, fmt.Errorf("claude returned error: %s", envelope.Result)
	}

	// Parse the inner result JSON
	// First try direct JSON parse
	var result map[string]any
	if err := json.Unmarshal([]byte(envelope.Result), &result); err == nil {
		return result, nil
	}

	// Try extracting JSON from markdown code fence
	extracted := extractJSONFromMarkdown(envelope.Result)
	if extracted != "" {
		if err := json.Unmarshal([]byte(extracted), &result); err == nil {
			return result, nil
		}
	}

	// Fallback: return raw text
	return map[string]any{"raw": envelope.Result}, nil
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

const prompt = `Explore this codebase for PRD generation.

Find:
- Vision: What does this project do? Why does it exist?
- Problem: What pain points does it solve?
- Users: Who uses this?

Extract VERBATIM QUOTES as evidence. Skip .env files.

Return JSON: {"vision": {...}, "problem": {...}, "users": {...}}`
