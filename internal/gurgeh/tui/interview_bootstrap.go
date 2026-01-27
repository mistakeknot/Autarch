package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/agents"
	"github.com/mistakeknot/autarch/internal/gurgeh/config"
	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/scan"
	"gopkg.in/yaml.v3"
)

type interviewBootstrapAnswers struct {
	Vision       string `yaml:"vision"`
	Users        string `yaml:"users"`
	Problem      string `yaml:"problem"`
	Goals        string `yaml:"goals"`
	NonGoals     string `yaml:"non_goals"`
	Assumptions  string `yaml:"assumptions"`
	Requirements string `yaml:"requirements"`
}

func (m *Model) runInterviewBootstrap() {
	if strings.TrimSpace(m.interview.scanSummary) == "" {
		res, _ := scan.ScanRepo(m.interview.root, scan.Options{})
		m.interview.scanSummary = renderScanSummary(res)
	}
	briefPath, err := writeInterviewBootstrapBrief(m.interview.root, m.interview.targetID, m.interview.scanSummary)
	if err != nil {
		m.status = "bootstrap brief failed: " + err.Error()
		return
	}
	cfg, err := config.LoadFromRoot(m.interview.root)
	if err != nil {
		m.status = "agent config missing"
		return
	}
	agentName := defaultAgentName(cfg)
	profile, err := agents.Resolve(agentProfiles(cfg), agentName)
	if err != nil {
		m.status = "agent not found"
		return
	}
	runner := runAgent
	if isClaudeProfile(agentName, profile) {
		runner = runSubagent
	}
	output, err := runner(profile, briefPath)
	if err != nil {
		m.status = "agent not found; brief at " + briefPath
		return
	}
	answers := parseInterviewBootstrapAnswers(output)
	if answers.isEmpty() {
		m.status = "bootstrap returned empty answers"
		return
	}
	m.applyInterviewBootstrapAnswers(answers)
	m.interview.step = stepVision
	m.loadInterviewInput()
	m.status = "bootstrap answers loaded"
}

func (m *Model) applyInterviewBootstrapAnswers(ans interviewBootstrapAnswers) {
	if m.interview.answers == nil {
		m.interview.answers = map[interviewStep]string{}
	}
	if m.interview.drafts == nil {
		m.interview.drafts = map[interviewStep]string{}
	}
	if strings.TrimSpace(ans.Vision) != "" {
		m.interview.answers[stepVision] = strings.TrimSpace(ans.Vision)
		m.interview.drafts[stepVision] = strings.TrimSpace(ans.Vision)
	}
	if strings.TrimSpace(ans.Users) != "" {
		m.interview.answers[stepUsers] = strings.TrimSpace(ans.Users)
		m.interview.drafts[stepUsers] = strings.TrimSpace(ans.Users)
	}
	if strings.TrimSpace(ans.Problem) != "" {
		m.interview.answers[stepProblem] = strings.TrimSpace(ans.Problem)
		m.interview.drafts[stepProblem] = strings.TrimSpace(ans.Problem)
	}
	if strings.TrimSpace(ans.Goals) != "" {
		m.interview.answers[stepGoals] = strings.TrimSpace(ans.Goals)
		m.interview.drafts[stepGoals] = strings.TrimSpace(ans.Goals)
	}
	if strings.TrimSpace(ans.NonGoals) != "" {
		m.interview.answers[stepNonGoals] = strings.TrimSpace(ans.NonGoals)
		m.interview.drafts[stepNonGoals] = strings.TrimSpace(ans.NonGoals)
	}
	if strings.TrimSpace(ans.Assumptions) != "" {
		m.interview.answers[stepAssumptions] = strings.TrimSpace(ans.Assumptions)
		m.interview.drafts[stepAssumptions] = strings.TrimSpace(ans.Assumptions)
	}
	if strings.TrimSpace(ans.Requirements) != "" {
		m.interview.answers[stepRequirements] = strings.TrimSpace(ans.Requirements)
		m.interview.drafts[stepRequirements] = strings.TrimSpace(ans.Requirements)
	}
}

func writeInterviewBootstrapBrief(root, id, scanSummary string) (string, error) {
	briefsDir := project.BriefsDir(root)
	if err := os.MkdirAll(briefsDir, 0o755); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("%s-bootstrap-%s.md", id, stamp)
	path := filepath.Join(briefsDir, name)
	content := buildInterviewBootstrapBrief(scanSummary)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func buildInterviewBootstrapBrief(scanSummary string) string {
	return fmt.Sprintf(`# PRD Interview Bootstrap

Repo scan summary:
%s

Instructions:
- Use the codebase context to draft initial answers for a PRD.
- Return YAML only, in this format:
  vision: |
    <vision>
  users: |
    <primary users>
  problem: |
    <problem statement>
  goals: |
    <measurable goals, newline-separated>
  non_goals: |
    <what is out of scope, newline-separated>
  assumptions: |
    <assumptions the PRD relies on, newline-separated>
  requirements: |
    <requirements, newline-separated>
- No extra commentary or markdown.
`, strings.TrimSpace(scanSummary))
}

func parseInterviewBootstrapAnswers(output []byte) interviewBootstrapAnswers {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return interviewBootstrapAnswers{}
	}
	var payload interviewBootstrapAnswers
	if err := yaml.Unmarshal(output, &payload); err != nil {
		return interviewBootstrapAnswers{}
	}
	return payload
}

func (a interviewBootstrapAnswers) isEmpty() bool {
	return strings.TrimSpace(a.Vision) == "" &&
		strings.TrimSpace(a.Users) == "" &&
		strings.TrimSpace(a.Problem) == "" &&
		strings.TrimSpace(a.Goals) == "" &&
		strings.TrimSpace(a.NonGoals) == "" &&
		strings.TrimSpace(a.Assumptions) == "" &&
		strings.TrimSpace(a.Requirements) == ""
}
