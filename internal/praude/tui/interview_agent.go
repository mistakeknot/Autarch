package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
	"gopkg.in/yaml.v3"
)

func writeInterviewBrief(root, id string, step interviewStep, answer, draft string, spec specs.Spec) (string, error) {
	briefsDir := project.BriefsDir(root)
	if err := os.MkdirAll(briefsDir, 0o755); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	stepName := strings.ToLower(strings.ReplaceAll(interviewStepName(step), " ", "-"))
	name := fmt.Sprintf("%s-interview-%s-%s.md", id, stamp, stepName)
	path := filepath.Join(briefsDir, name)
	content := buildInterviewBrief(step, answer, draft, spec)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func buildInterviewBrief(step interviewStep, answer, draft string, spec specs.Spec) string {
	prompt, _, _ := interviewStepInfo(step)
	return fmt.Sprintf(`# PRD Interview Step: %s

Question: %s

Current Answer:
%s

Current Draft:
%s

Existing PRD Context:
Title: %s
Summary: %s
Requirements: %v

Instructions:
- Return YAML only, in this format:
  draft: |
    <updated draft text>
- No extra commentary or markdown.
`, prompt.title, prompt.question, strings.TrimSpace(answer), strings.TrimSpace(draft), spec.Title, spec.Summary, spec.Requirements)
}

func parseAgentDraft(output []byte) string {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return ""
	}
	var payload struct {
		Draft string `yaml:"draft"`
	}
	if err := yaml.Unmarshal(output, &payload); err == nil {
		if strings.TrimSpace(payload.Draft) != "" {
			return strings.TrimSpace(payload.Draft)
		}
	}
	return trimmed
}

func interviewStepName(step interviewStep) string {
	prompt, _, _ := interviewStepInfo(step)
	return prompt.title
}
