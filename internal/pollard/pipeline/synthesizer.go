// Package pipeline provides the research pipeline components.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Synthesizer spawns user's coding agents to analyze research items.
// This is agent-native: we orchestrate, we don't embed an LLM.
type Synthesizer struct {
	AgentCmd    string        // Agent command (e.g., "claude", "cursor --ask", "aider")
	Parallelism int           // Max concurrent agent instances
	Timeout     time.Duration // Per-item timeout
}

// NewSynthesizer creates a synthesizer with the given configuration.
func NewSynthesizer(agentCmd string, parallelism int, timeout time.Duration) *Synthesizer {
	if parallelism <= 0 {
		parallelism = 3
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &Synthesizer{
		AgentCmd:    agentCmd,
		Parallelism: parallelism,
		Timeout:     timeout,
	}
}

// SynthesizeBatch spawns parallel agent instances to analyze items.
func (s *Synthesizer) SynthesizeBatch(ctx context.Context, items []FetchedItem, query string) ([]SynthesizedItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	// If no agent configured, return items without synthesis
	if s.AgentCmd == "" {
		return s.skipSynthesis(items), nil
	}

	sem := make(chan struct{}, s.Parallelism)
	results := make([]SynthesizedItem, len(items))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for i, item := range items {
		wg.Add(1)
		go func(idx int, item FetchedItem) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				errs = append(errs, ctx.Err())
				mu.Unlock()
				results[idx] = SynthesizedItem{
					Fetched:   item,
					Synthesis: Synthesis{},
				}
				return
			}

			synthesis, err := s.synthesizeOne(ctx, item, query)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("item %d: %w", idx, err))
				mu.Unlock()
				synthesis = Synthesis{
					Summary:    "Synthesis failed",
					Confidence: 0,
				}
			}

			results[idx] = SynthesizedItem{
				Fetched:   item,
				Synthesis: synthesis,
			}
		}(i, item)
	}

	wg.Wait()

	// Return results even if some failed
	return results, nil
}

// synthesizeOne spawns a single agent instance via CLI.
func (s *Synthesizer) synthesizeOne(ctx context.Context, item FetchedItem, query string) (Synthesis, error) {
	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	// Build content for analysis
	content := s.buildItemContent(item)

	prompt := fmt.Sprintf(`Analyze this %s for relevance to: "%s"

%s

Respond with ONLY valid JSON (no markdown, no explanation):
{"summary": "...", "key_features": ["...", "..."], "relevance_rationale": "...", "recommendations": ["...", "..."], "confidence": 0.0-1.0}`,
		item.Raw.Type, query, content)

	// Parse agent command - support multi-word commands
	parts := strings.Fields(s.AgentCmd)
	if len(parts) == 0 {
		return Synthesis{}, fmt.Errorf("empty agent command")
	}

	// Add --print flag for non-interactive mode if using claude
	args := append(parts[1:], prompt)
	if parts[0] == "claude" {
		args = append([]string{"--print"}, args...)
	}

	cmd := exec.CommandContext(ctx, parts[0], args...)
	cmd.Env = os.Environ()

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return Synthesis{}, fmt.Errorf("agent failed: %s", string(exitErr.Stderr))
		}
		return Synthesis{}, fmt.Errorf("agent execution failed: %w", err)
	}

	// Parse JSON output
	var result Synthesis
	outputStr := strings.TrimSpace(string(output))

	// Try to extract JSON from the output (agent might include extra text)
	jsonStart := strings.Index(outputStr, "{")
	jsonEnd := strings.LastIndex(outputStr, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		outputStr = outputStr[jsonStart : jsonEnd+1]
	}

	if err := json.Unmarshal([]byte(outputStr), &result); err != nil {
		return Synthesis{
			Summary:    "Failed to parse agent response",
			Confidence: 0,
		}, fmt.Errorf("failed to parse agent JSON: %w", err)
	}

	result.AgentUsed = parts[0]
	result.SynthesizedAt = time.Now()

	return result, nil
}

// buildItemContent creates a summary of the item for the agent.
func (s *Synthesizer) buildItemContent(item FetchedItem) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Title: %s\n", item.Raw.Title))
	sb.WriteString(fmt.Sprintf("URL: %s\n", item.Raw.URL))
	sb.WriteString(fmt.Sprintf("Type: %s\n", item.Raw.Type))

	// Add type-specific metadata
	if item.Raw.Metadata != nil {
		if stars, ok := item.Raw.Metadata["stars"].(int); ok {
			sb.WriteString(fmt.Sprintf("Stars: %d\n", stars))
		}
		if points, ok := item.Raw.Metadata["points"].(int); ok {
			sb.WriteString(fmt.Sprintf("Points: %d\n", points))
		}
		if language, ok := item.Raw.Metadata["language"].(string); ok && language != "" {
			sb.WriteString(fmt.Sprintf("Language: %s\n", language))
		}
		if topics, ok := item.Raw.Metadata["topics"].([]string); ok && len(topics) > 0 {
			sb.WriteString(fmt.Sprintf("Topics: %s\n", strings.Join(topics, ", ")))
		}
		if citations, ok := item.Raw.Metadata["citations"].(int); ok {
			sb.WriteString(fmt.Sprintf("Citations: %d\n", citations))
		}
		if description, ok := item.Raw.Metadata["description"].(string); ok && description != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n", description))
		}
	}

	// Add fetched content (README, abstract, etc.)
	if item.Content != "" {
		// Truncate if too long
		content := item.Content
		if len(content) > 2000 {
			content = content[:2000] + "\n...[truncated]"
		}
		sb.WriteString(fmt.Sprintf("\n%s:\n%s\n", item.ContentType, content))
	}

	return sb.String()
}

// skipSynthesis returns items without synthesis when agent is not configured.
func (s *Synthesizer) skipSynthesis(items []FetchedItem) []SynthesizedItem {
	results := make([]SynthesizedItem, len(items))
	for i, item := range items {
		results[i] = SynthesizedItem{
			Fetched: item,
			Synthesis: Synthesis{
				Summary:            "Synthesis skipped - no agent configured",
				KeyFeatures:        nil,
				RelevanceRationale: "",
				Recommendations:    nil,
				Confidence:         0,
			},
		}
	}
	return results
}

// SynthesizerConfig holds configuration for creating a Synthesizer.
type SynthesizerConfig struct {
	Agent       string        `yaml:"agent"`       // Agent command
	Parallelism int           `yaml:"parallelism"` // Max concurrent agents
	Timeout     time.Duration `yaml:"timeout"`     // Per-item timeout
}

// DefaultSynthesizerConfig returns sensible defaults.
func DefaultSynthesizerConfig() SynthesizerConfig {
	return SynthesizerConfig{
		Agent:       "", // Empty means synthesis disabled
		Parallelism: 3,
		Timeout:     2 * time.Minute,
	}
}
