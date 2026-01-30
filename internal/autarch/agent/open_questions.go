package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ResolvedQuestion struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type OpenQuestionsResolution struct {
	Resolved  []ResolvedQuestion `json:"resolved"`
	Remaining []string           `json:"remaining"`
}

type ResolveOpenQuestionsInput struct {
	Phase         string
	Summary       string
	Evidence      []EvidenceItem
	OpenQuestions []string
	UserAnswer    string
	Vision        string
	Problem       string
	Users         string
	Platform      string
	Language      string
	Requirements  []string
}

func ResolveOpenQuestionsWithOutput(ctx context.Context, agent *Agent, input ResolveOpenQuestionsInput, onOutput OutputCallback) (*OpenQuestionsResolution, error) {
	prompt := buildOpenQuestionsPrompt(input)
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	resp, err := agent.GenerateWithOutput(ctx, GenerateRequest{Prompt: prompt}, onOutput)
	if err != nil {
		return nil, fmt.Errorf("agent generation failed: %w", err)
	}

	return parseOpenQuestionsResponse(resp.Content)
}

func buildOpenQuestionsPrompt(input ResolveOpenQuestionsInput) string {
	var sb strings.Builder

	sb.WriteString("You are updating open questions for a project scan.\n\n")
	if input.Phase != "" {
		sb.WriteString("PHASE: ")
		sb.WriteString(input.Phase)
		sb.WriteString("\n\n")
	}
	if input.Summary != "" {
		sb.WriteString("CURRENT SUMMARY:\n")
		sb.WriteString(input.Summary)
		sb.WriteString("\n\n")
	}
	if len(input.Evidence) > 0 {
		sb.WriteString("EVIDENCE:\n")
		for _, ev := range input.Evidence {
			line := ev.Path
			if ev.Quote != "" {
				if line != "" {
					line += ": "
				}
				line += ev.Quote
			}
			if line != "" {
				sb.WriteString("- ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}
	if len(input.OpenQuestions) > 0 {
		sb.WriteString("OPEN QUESTIONS:\n")
		for i, q := range input.OpenQuestions {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, q))
		}
		sb.WriteString("\n")
	}
	if input.UserAnswer != "" {
		sb.WriteString("USER ANSWER:\n")
		sb.WriteString(input.UserAnswer)
		sb.WriteString("\n\n")
	}

	sb.WriteString("KNOWN CONTEXT:\n")
	if input.Vision != "" {
		sb.WriteString("- vision: ")
		sb.WriteString(input.Vision)
		sb.WriteString("\n")
	}
	if input.Problem != "" {
		sb.WriteString("- problem: ")
		sb.WriteString(input.Problem)
		sb.WriteString("\n")
	}
	if input.Users != "" {
		sb.WriteString("- users: ")
		sb.WriteString(input.Users)
		sb.WriteString("\n")
	}
	if input.Platform != "" {
		sb.WriteString("- platform: ")
		sb.WriteString(input.Platform)
		sb.WriteString("\n")
	}
	if input.Language != "" {
		sb.WriteString("- language: ")
		sb.WriteString(input.Language)
		sb.WriteString("\n")
	}
	if len(input.Requirements) > 0 {
		sb.WriteString("- requirements: ")
		sb.WriteString(strings.Join(input.Requirements, "; "))
		sb.WriteString("\n")
	}

	sb.WriteString(`
\nDetermine which open questions are answered by the user response.
- Move answered questions to "resolved" with a concise answer.
- Keep unanswered ones in "remaining".
- Only resolve questions actually addressed by the user answer.

Output ONLY valid JSON in this exact format (no markdown, no explanation):
{
  "resolved": [
    {"question": "...", "answer": "..."}
  ],
  "remaining": ["..."]
}

Generate the JSON now:`)

	return sb.String()
}

func parseOpenQuestionsResponse(content string) (*OpenQuestionsResolution, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}

	var response OpenQuestionsResolution
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if len(response.Resolved) == 0 && len(response.Remaining) == 0 {
		return nil, fmt.Errorf("no resolution generated")
	}

	return &response, nil
}
