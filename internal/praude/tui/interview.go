package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/agents"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/research"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/scan"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/suggestions"
	"gopkg.in/yaml.v3"
)

type interviewStep int

const (
	stepScanPrompt interviewStep = iota
	stepDraftConfirm
	stepVision
	stepUsers
	stepProblem
	stepRequirements
	stepResearchPrompt
)

type interviewState struct {
	step         interviewStep
	root         string
	draft        specs.Spec
	scanSummary  string
	vision       string
	users        string
	problem      string
	requirements string
	warnings     []string
	specID       string
	specPath     string
	optionIndex  int
}

func startInterview(root string) interviewState {
	return interviewState{step: stepScanPrompt, root: root, optionIndex: 0}
}

func (m *Model) handleInterviewInput(key string) {
	if key == "tab" {
		m.toggleInterviewFocus()
		return
	}
	switch m.interview.step {
	case stepScanPrompt:
		m.handleOptionStep(key, func() {
			res, _ := scan.ScanRepo(m.interview.root, scan.Options{})
			m.interview.scanSummary = renderScanSummary(res)
			m.interview.draft = buildDraftSpec(m.interview.scanSummary)
			m.interview.step = stepDraftConfirm
			m.interview.optionIndex = 0
		}, func() {
			m.interview.draft = buildDraftSpec(m.interview.scanSummary)
			m.interview.step = stepDraftConfirm
			m.interview.optionIndex = 0
		})
	case stepDraftConfirm:
		m.handleOptionStep(key, func() {
			m.interview.step = stepVision
		}, func() {
			m.exitInterview()
		})
	case stepVision:
		m.handleTextStep(key, func(input string) {
			m.interview.vision = input
			m.interview.step = stepUsers
		})
	case stepUsers:
		m.handleTextStep(key, func(input string) {
			m.interview.users = input
			m.interview.step = stepProblem
		})
	case stepProblem:
		m.handleTextStep(key, func(input string) {
			m.interview.problem = input
			m.interview.step = stepRequirements
		})
	case stepRequirements:
		m.handleTextStep(key, func(input string) {
			m.interview.requirements = input
			m.finalizeInterview()
			m.interview.step = stepResearchPrompt
			m.interview.optionIndex = 0
		})
	case stepResearchPrompt:
		m.handleOptionStep(key, func() {
			m.runResearch()
			m.exitInterview()
		}, func() {
			m.exitInterview()
		})
	}
}

func (m *Model) toggleInterviewFocus() {
	if strings.EqualFold(m.interviewFocus, "steps") {
		m.interviewFocus = "question"
		return
	}
	m.interviewFocus = "steps"
}

func (m *Model) handleTextStep(key string, onDone func(string)) {
	switch key {
	case "enter":
		onDone(strings.TrimSpace(m.input))
		m.input = ""
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		m.input += key
	}
}

func (m *Model) handleOptionStep(key string, onYes func(), onNo func()) {
	if !strings.EqualFold(m.interviewFocus, "question") {
		return
	}
	switch key {
	case "up", "down", "left", "right":
		if m.interview.optionIndex == 0 {
			m.interview.optionIndex = 1
		} else {
			m.interview.optionIndex = 0
		}
		return
	case "1":
		onYes()
		return
	case "2":
		onNo()
		return
	case "enter":
		if m.interview.optionIndex == 0 {
			onYes()
			return
		}
		onNo()
		return
	}
}

func (m *Model) finalizeInterview() {
	spec := buildSpecFromInterview(m.interview)
	path, id, warnings := writeSpec(m.interview.root, spec)
	m.interview.specPath = path
	m.interview.specID = id
	m.interview.warnings = warnings
	m.reloadSummaries()
	m.autoApplySuggestions()
}

func (m *Model) runResearch() {
	if m.interview.specID == "" {
		return
	}
	researchDir := project.ResearchDir(m.interview.root)
	_, _ = research.Create(researchDir, m.interview.specID, time.Now())
}

func (m *Model) autoApplySuggestions() {
	if m.interview.specID == "" {
		return
	}
	now := time.Now()
	suggDir := project.SuggestionsDir(m.interview.root)
	if err := os.MkdirAll(suggDir, 0o755); err != nil {
		m.status = "Suggestions failed: " + err.Error()
		return
	}
	suggPath, err := suggestions.Create(suggDir, m.interview.specID, now)
	if err != nil {
		m.status = "Suggestions failed: " + err.Error()
		return
	}
	briefPath, err := writeSuggestionBrief(m.interview.root, m.interview.specID, suggPath, now)
	if err != nil {
		m.status = "Suggestions failed: " + err.Error()
		return
	}
	cfg, err := config.LoadFromRoot(m.interview.root)
	if err != nil {
		m.status = "Suggestions failed: " + err.Error()
		return
	}
	agentName := defaultAgentName(cfg)
	profile, err := agents.Resolve(agentProfiles(cfg), agentName)
	if err != nil {
		m.status = "Suggestions failed: " + err.Error()
		return
	}
	launcher := launchAgent
	if isClaudeProfile(agentName, profile) {
		launcher = launchSubagent
	}
	if err := launcher(profile, briefPath); err != nil {
		m.status = "agent not found; brief at " + briefPath
		return
	}
	applied, err := applyReadySuggestions(m.interview.root, m.interview.specID, suggPath)
	if err != nil {
		m.status = "Suggestions failed: " + err.Error()
		return
	}
	if applied {
		m.status = "applied suggestions from " + agentName
		m.reloadSummaries()
		return
	}
	m.status = "launched suggestions agent " + agentName
}

func (m *Model) exitInterview() {
	m.mode = "list"
	m.input = ""
	m.interview = interviewState{}
}

func renderScanSummary(res scan.Result) string {
	return "Scan summary: " + itoa(len(res.Entries)) + " files, " + itoa(int(res.TotalBytes)) + " bytes"
}

func (m Model) renderInterviewPanel(width int) []string {
	return renderMarkdownLines(m.interviewMarkdown(), width)
}

func (m Model) renderInterviewStepsPanel(width int) []string {
	return renderMarkdownLines(m.interviewStepsMarkdown(), width)
}

func (m Model) interviewMarkdown() string {
	prompt, stepNum, total := interviewStepInfo(m.interview.step)
	var b strings.Builder
	b.WriteString("# Interview\n")
	b.WriteString("**PM-focused agent:** Codex CLI / Claude Code\n\n")
	b.WriteString(fmt.Sprintf("**Step %d/%d: %s**\n\n", stepNum, total, prompt.title))
	b.WriteString("Question:\n")
	b.WriteString(prompt.question)
	b.WriteString("\n\n")
	if m.interview.step == stepDraftConfirm {
		b.WriteString("Draft:\n")
		b.WriteString("Draft PRD ready.\n")
		if strings.TrimSpace(m.interview.scanSummary) != "" {
			b.WriteString("Context: ")
			b.WriteString(m.interview.scanSummary)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(prompt.options) > 0 {
		b.WriteString("Options:\n")
		b.WriteString("```\n")
		for idx, opt := range prompt.options {
			marker := "[ ]"
			if idx == m.interview.optionIndex {
				marker = "[*]"
			}
			b.WriteString(marker)
			b.WriteString(" ")
			b.WriteString(opt)
			b.WriteString("\n")
		}
		b.WriteString("```\n")
		b.WriteString("\n")
	}
	if prompt.expectsText {
		b.WriteString("```\n")
		b.WriteString("> ")
		b.WriteString(m.input)
		b.WriteString("\n```\n")
		b.WriteString("Press Enter to submit.\n")
	} else {
		b.WriteString("```\n")
		b.WriteString("> [1/2] (arrows + Enter)\n")
		b.WriteString("```\n")
	}
	return b.String()
}

func (m Model) interviewStepsMarkdown() string {
	steps := []interviewStep{
		stepScanPrompt,
		stepDraftConfirm,
		stepVision,
		stepUsers,
		stepProblem,
		stepRequirements,
		stepResearchPrompt,
	}
	var b strings.Builder
	b.WriteString("## STEPS\n\n")
	b.WriteString("```\n")
	for i, step := range steps {
		prompt, _, _ := interviewStepInfo(step)
		label := fmt.Sprintf("%d) %s", i+1, prompt.title)
		if step == m.interview.step {
			b.WriteString("> ")
			b.WriteString(label)
			b.WriteString("\n")
			continue
		}
		b.WriteString(label)
		b.WriteString("\n")
	}
	b.WriteString("```\n")
	return b.String()
}

func renderMarkdownLines(content string, width int) []string {
	if width <= 0 {
		width = 80
	}
	rendered := renderMarkdown(content, width)
	trimmed := strings.TrimRight(rendered, "\n")
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "\n")
}

func buildDraftSpec(summary string) specs.Spec {
	text := summary
	if text == "" {
		text = "Draft from scan"
	}
	return specs.Spec{Title: "Draft PRD", Summary: text}
}

func buildSpecFromInterview(state interviewState) specs.Spec {
	reqList := parseRequirements(state.requirements)
	if len(reqList) == 0 {
		reqList = []string{"REQ-001: TBD"}
	}
	firstReq := extractReqID(reqList[0])
	title := firstNonEmpty(state.vision, state.problem, "New PRD")
	summary := firstNonEmpty(state.problem, state.vision, "Summary pending")
	return specs.Spec{
		Title:        title,
		Summary:      summary,
		Requirements: reqList,
		StrategicContext: specs.StrategicContext{
			CUJID:       "CUJ-001",
			CUJName:     "Primary Journey",
			FeatureID:   "",
			MVPIncluded: true,
		},
		UserStory: specs.UserStory{
			Text: "As a user, " + firstNonEmpty(state.users, "I need", "I need") + ", " + summary,
			Hash: "pending",
		},
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				ID:                 "CUJ-001",
				Title:              "Primary Journey",
				Priority:           "high",
				Steps:              []string{"Start", "Finish"},
				SuccessCriteria:    []string{"Goal achieved"},
				LinkedRequirements: []string{firstReq},
			},
			{
				ID:                 "CUJ-002",
				Title:              "Maintenance",
				Priority:           "low",
				Steps:              []string{"Routine upkeep"},
				SuccessCriteria:    []string{"System remains stable"},
				LinkedRequirements: []string{firstReq},
			},
		},
	}
}

func writeSpec(root string, spec specs.Spec) (string, string, []string) {
	specDir := project.SpecsDir(root)
	id, err := specs.NextID(specDir)
	if err != nil {
		return "", "", nil
	}
	spec.ID = id
	if spec.CreatedAt == "" {
		spec.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	raw, err := yaml.Marshal(spec)
	if err != nil {
		return "", id, nil
	}
	path := filepath.Join(specDir, id+".yaml")
	if err := osWriteFile(path, raw, 0o644); err != nil {
		return path, id, nil
	}
	res, err := specs.Validate(raw, specs.ValidationOptions{Mode: specs.ValidationSoft, Root: root})
	if err != nil {
		return path, id, nil
	}
	if len(res.Warnings) > 0 {
		_ = specs.StoreValidationWarnings(path, res.Warnings)
	}
	return path, id, res.Warnings
}

func applyReadySuggestions(root, id, suggPath string) (bool, error) {
	raw, err := os.ReadFile(suggPath)
	if err != nil {
		return false, err
	}
	ready := suggestions.ParseReady(raw)
	if suggestions.IsEmpty(ready) {
		return false, nil
	}
	specPath := filepath.Join(project.SpecsDir(root), id+".yaml")
	if err := suggestions.Apply(specPath, ready); err != nil {
		return false, err
	}
	updated, err := os.ReadFile(specPath)
	if err != nil {
		return true, err
	}
	res, err := specs.Validate(updated, specs.ValidationOptions{Mode: specs.ValidationSoft, Root: root})
	if err != nil {
		return true, err
	}
	if len(res.Warnings) > 0 {
		_ = specs.StoreValidationWarnings(specPath, res.Warnings)
	}
	return true, nil
}

func parseRequirements(input string) []string {
	parts := splitInput(input)
	var out []string
	for i, part := range parts {
		id := formatReqID(i + 1)
		out = append(out, id+": "+part)
	}
	return out
}

func splitInput(input string) []string {
	input = strings.ReplaceAll(input, "\n", ",")
	parts := strings.Split(input, ",")
	var out []string
	for _, part := range parts {
		trim := strings.TrimSpace(part)
		if trim != "" {
			out = append(out, trim)
		}
	}
	return out
}

func formatReqID(n int) string {
	return "REQ-" + pad3(n)
}

func pad3(n int) string {
	if n < 10 {
		return "00" + itoa(n)
	}
	if n < 100 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func extractReqID(req string) string {
	fields := strings.Fields(req)
	if len(fields) == 0 {
		return "REQ-001"
	}
	id := strings.TrimSuffix(fields[0], ":")
	if strings.HasPrefix(id, "REQ-") {
		return id
	}
	return "REQ-001"
}

func firstNonEmpty(values ...string) string {
	for _, val := range values {
		if strings.TrimSpace(val) != "" {
			return val
		}
	}
	return ""
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

var osWriteFile = os.WriteFile

type interviewPrompt struct {
	title       string
	question    string
	options     []string
	expectsText bool
}

func interviewStepInfo(step interviewStep) (interviewPrompt, int, int) {
	total := 7
	switch step {
	case stepScanPrompt:
		return interviewPrompt{
			title:    "Scan repo",
			question: "Scan repo now?",
			options:  []string{"1) Yes - scan repo for context", "2) No - skip scan"},
		}, 1, total
	case stepDraftConfirm:
		return interviewPrompt{
			title:    "Confirm draft",
			question: "Confirm draft?",
			options:  []string{"1) Yes - continue interview", "2) No - cancel interview"},
		}, 2, total
	case stepVision:
		return interviewPrompt{
			title:       "Vision",
			question:    "What is the vision?",
			expectsText: true,
		}, 3, total
	case stepUsers:
		return interviewPrompt{
			title:       "Users",
			question:    "Who are the primary users?",
			expectsText: true,
		}, 4, total
	case stepProblem:
		return interviewPrompt{
			title:       "Problem",
			question:    "What problem are we solving?",
			expectsText: true,
		}, 5, total
	case stepRequirements:
		return interviewPrompt{
			title:       "Requirements",
			question:    "List requirements (comma or newline separated).",
			expectsText: true,
		}, 6, total
	case stepResearchPrompt:
		return interviewPrompt{
			title:    "Research",
			question: "Run research now?",
			options:  []string{"1) Yes - create research brief", "2) No - skip for now"},
		}, 7, total
	default:
		return interviewPrompt{
			title:    "Interview",
			question: "Continue the interview.",
		}, 1, total
	}
}

func parseYesNoKey(key string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "y", "1":
		return true, true
	case "n", "2":
		return false, true
	default:
		return false, false
	}
}
