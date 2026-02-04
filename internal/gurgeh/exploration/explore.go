package exploration

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Explore runs Claude Code and returns parsed output.
// Returns map[string]any - don't define types until we see real output.
func Explore(ctx context.Context, cwd string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude",
		"-p", prompt,
		"--output-format", "json",
		"--print",
	)
	cmd.Dir = cwd

	out, err := cmd.Output()
	if err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return nil, fmt.Errorf("claude not found: install Claude Code CLI")
		}
		return nil, fmt.Errorf("claude failed: %w", err)
	}

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
	var result map[string]any
	if err := json.Unmarshal([]byte(envelope.Result), &result); err != nil {
		// Result might be plain text, not JSON - return as-is
		return map[string]any{"raw": envelope.Result}, nil
	}
	return result, nil
}

const prompt = `Explore this codebase for PRD generation.

Find:
- Vision: What does this project do? Why does it exist?
- Problem: What pain points does it solve?
- Users: Who uses this?

Extract VERBATIM QUOTES as evidence. Skip .env files.

Return JSON: {"vision": {...}, "problem": {...}, "users": {...}}`
