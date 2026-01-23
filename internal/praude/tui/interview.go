package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	stepBootstrapPrompt
	stepVision
	stepUsers
	stepProblem
	stepRequirements
	stepResearchPrompt
)

type interviewMessage struct {
	Role string
	Text string
}

type interviewState struct {
	step              interviewStep
	root              string
	scanSummary       string
	warnings          []string
	targetID          string
	targetPath        string
	baseSpec          specs.Spec
	answers           map[interviewStep]string
	drafts            map[interviewStep]string
	optionIndex       int
	finalized         bool
	chat              []interviewMessage
	bootstrapEligible bool
}

func startInterview(root string, base specs.Spec, targetPath string) interviewState {
	state := interviewState{
		step:        stepScanPrompt,
		root:        root,
		targetID:    base.ID,
		targetPath:  targetPath,
		baseSpec:    base,
		answers:     map[interviewStep]string{},
		drafts:      map[interviewStep]string{},
		optionIndex: 0,
	}
	if strings.TrimSpace(base.Title) != "" {
		state.answers[stepVision] = base.Title
	}
	if strings.TrimSpace(base.UserStory.Text) != "" {
		state.answers[stepUsers] = base.UserStory.Text
	}
	if strings.TrimSpace(base.Summary) != "" {
		state.answers[stepProblem] = base.Summary
	}
	if len(base.Requirements) > 0 {
		state.answers[stepRequirements] = strings.Join(base.Requirements, "\n")
	}
	return state
}

func (s interviewState) answerForStep(step interviewStep) string {
	if s.answers == nil {
		return ""
	}
	return s.answers[step]
}

func (m *Model) handleInterviewInput(msg tea.KeyMsg) {
	key := msg.String()
	switch key {
	case "ctrl+o":
		m.openInterviewSpec()
		return
	case "ctrl+`", "\\":
		m.interviewLayoutSwap = !m.interviewLayoutSwap
		return
	}
	if key == "tab" {
		m.toggleInterviewFocus()
		return
	}
	if key == "[" {
		m.prevInterviewStep()
		return
	}
	if key == "]" {
		m.nextInterviewStep()
		return
	}
	switch m.interview.step {
	case stepScanPrompt:
		m.handleOptionStep(key, func() {
			res, _ := scan.ScanRepo(m.interview.root, scan.Options{})
			m.interview.scanSummary = renderScanSummary(res)
			m.interview.step = stepDraftConfirm
			m.interview.optionIndex = 0
		}, func() {
			m.interview.step = stepDraftConfirm
			m.interview.optionIndex = 0
		})
	case stepDraftConfirm:
		m.handleOptionStep(key, func() {
			if m.interview.bootstrapEligible {
				m.interview.step = stepBootstrapPrompt
				m.interview.optionIndex = 0
				return
			}
			m.interview.step = stepVision
			m.loadInterviewInput()
		}, func() {
			m.exitInterview()
		})
	case stepBootstrapPrompt:
		m.handleOptionStep(key, func() {
			m.runInterviewBootstrap()
		}, func() {
			m.interview.step = stepVision
			m.loadInterviewInput()
		})
	case stepVision:
		m.handleTextStep(msg, stepVision)
	case stepUsers:
		m.handleTextStep(msg, stepUsers)
	case stepProblem:
		m.handleTextStep(msg, stepProblem)
	case stepRequirements:
		m.handleTextStep(msg, stepRequirements)
	case stepResearchPrompt:
		m.handleOptionStep(key, func() {
			m.finishInterview(true)
		}, func() {
			m.finishInterview(false)
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

func (m *Model) handleTextStep(msg tea.KeyMsg, step interviewStep) {
	key := msg.String()
	if msg.Alt && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		switch msg.Runes[0] {
		case 'b':
			m.input.MoveWordLeft()
			m.storeInterviewAnswer(step)
			return
		case 'f':
			m.input.MoveWordRight()
			m.storeInterviewAnswer(step)
			return
		}
	}
	switch key {
	case "enter":
		m.storeInterviewAnswer(step)
		m.appendInterviewMessage("user", m.interview.answerForStep(step))
		m.iterateInterviewStep(step)
		return
	case " ":
		m.input.InsertRune(' ')
	case "space":
		m.input.InsertRune(' ')
	case "backspace":
		if msg.Alt {
			m.input.DeleteWordLeft()
		} else {
			m.input.Backspace()
		}
	case "left":
		if msg.Alt {
			m.input.MoveWordLeft()
		} else {
			m.input.MoveLeft()
		}
	case "right":
		if msg.Alt {
			m.input.MoveWordRight()
		} else {
			m.input.MoveRight()
		}
	case "up":
		m.input.MoveUp()
	case "down":
		m.input.MoveDown()
	case "alt+left":
		m.input.MoveWordLeft()
	case "alt+right":
		m.input.MoveWordRight()
	case "alt+backspace":
		m.input.DeleteWordLeft()
	case "ctrl+j":
		m.input.InsertRune('\n')
	default:
		if msg.Type == tea.KeyRunes {
			for _, r := range msg.Runes {
				m.input.InsertRune(r)
			}
		}
	}
	m.storeInterviewAnswer(step)
}

func (m *Model) appendInterviewMessage(role, text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	m.interview.chat = append(m.interview.chat, interviewMessage{Role: role, Text: trimmed})
}

func (m *Model) openInterviewSpec() {
	path := strings.TrimSpace(m.interview.targetPath)
	if path == "" {
		m.status = "No PRD file to open"
		return
	}
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor)
	cmdName := editor
	args := []string{path}
	if len(parts) > 0 {
		cmdName = parts[0]
		if len(parts) > 1 {
			args = append(parts[1:], path)
		}
	}
	cmd := exec.Command(cmdName, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		m.status = "Open failed: " + err.Error()
		return
	}
	m.status = "Opened " + filepath.Base(path)
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
		m.applyOptionSelection(onYes, onNo)
	}
}

func (m *Model) applyOptionSelection(onYes func(), onNo func()) {
	if m.interview.optionIndex == 0 {
		onYes()
		return
	}
	onNo()
}

func (m *Model) storeInterviewAnswer(step interviewStep) {
	prompt, _, _ := interviewStepInfo(step)
	if !prompt.expectsText {
		return
	}
	if m.interview.answers == nil {
		m.interview.answers = map[interviewStep]string{}
	}
	m.interview.answers[step] = m.input.Text()
}

func (m *Model) loadInterviewInput() {
	prompt, _, _ := interviewStepInfo(m.interview.step)
	if !prompt.expectsText {
		m.input.SetText("")
		return
	}
	m.input.SetText(m.interview.answerForStep(m.interview.step))
}

func (m *Model) prevInterviewStep() {
	m.storeInterviewAnswer(m.interview.step)
	switch m.interview.step {
	case stepScanPrompt:
		return
	case stepDraftConfirm:
		m.interview.step = stepScanPrompt
	case stepVision:
		m.interview.step = stepDraftConfirm
	case stepUsers:
		m.interview.step = stepVision
	case stepProblem:
		m.interview.step = stepUsers
	case stepRequirements:
		m.interview.step = stepProblem
	case stepResearchPrompt:
		m.interview.step = stepRequirements
	}
	m.loadInterviewInput()
}

func (m *Model) nextInterviewStep() {
	switch m.interview.step {
	case stepScanPrompt:
		m.applyOptionSelection(func() {
			res, _ := scan.ScanRepo(m.interview.root, scan.Options{})
			m.interview.scanSummary = renderScanSummary(res)
			m.interview.step = stepDraftConfirm
			m.interview.optionIndex = 0
		}, func() {
			m.interview.step = stepDraftConfirm
			m.interview.optionIndex = 0
		})
	case stepDraftConfirm:
		m.applyOptionSelection(func() {
			m.interview.step = stepVision
			m.loadInterviewInput()
		}, func() {
			m.exitInterview()
		})
	case stepVision:
		m.advanceTextStep(stepUsers)
	case stepUsers:
		m.advanceTextStep(stepProblem)
	case stepProblem:
		m.advanceTextStep(stepRequirements)
	case stepRequirements:
		m.advanceTextStep(stepResearchPrompt)
		m.interview.optionIndex = 0
	case stepResearchPrompt:
		m.applyOptionSelection(func() {
			m.finishInterview(true)
		}, func() {
			m.finishInterview(false)
		})
	}
}

func (m *Model) advanceTextStep(next interviewStep) {
	m.storeInterviewAnswer(m.interview.step)
	m.interview.step = next
	m.loadInterviewInput()
}

func (m *Model) iterateInterviewStep(step interviewStep) {
	prompt, _, _ := interviewStepInfo(step)
	if !prompt.expectsText {
		return
	}
	answer := strings.TrimSpace(m.interview.answerForStep(step))
	draft := strings.TrimSpace(m.interview.drafts[step])
	briefPath, err := writeInterviewBrief(m.interview.root, m.interview.targetID, step, answer, draft, m.interview.baseSpec)
	if err != nil {
		m.status = "interview brief failed: " + err.Error()
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
	newDraft := parseAgentDraft(output)
	if strings.TrimSpace(newDraft) == "" {
		m.status = "agent returned empty draft"
		return
	}
	if m.interview.drafts == nil {
		m.interview.drafts = map[interviewStep]string{}
	}
	m.interview.drafts[step] = newDraft
	m.appendInterviewMessage("agent", newDraft)
	m.status = "draft updated"
}

func (m *Model) finishInterview(runResearch bool) {
	if err := m.finalizeInterview(); err != nil {
		m.status = "Interview save failed: " + err.Error()
		m.exitInterview()
		return
	}
	if runResearch {
		m.runResearch()
	}
	m.exitInterview()
}

func (m *Model) finalizeInterview() error {
	if m.interview.finalized {
		return nil
	}
	if strings.TrimSpace(m.interview.targetPath) == "" {
		return fmt.Errorf("missing target path")
	}
	spec := mergeInterviewSpec(m.interview.baseSpec, m.interview.answers, m.interview.drafts)
	raw, err := yaml.Marshal(spec)
	if err != nil {
		return err
	}
	if err := osWriteFile(m.interview.targetPath, raw, 0o644); err != nil {
		return err
	}
	res, err := specs.Validate(raw, specs.ValidationOptions{Mode: specs.ValidationSoft, Root: m.interview.root})
	if err != nil {
		return err
	}
	if len(res.Warnings) > 0 {
		_ = specs.StoreValidationWarnings(m.interview.targetPath, res.Warnings)
		m.interview.warnings = res.Warnings
	}
	m.interview.finalized = true
	m.reloadSummaries()
	m.autoApplySuggestions()
	return nil
}

func (m *Model) runResearch() {
	if m.interview.targetID == "" {
		return
	}
	researchDir := project.ResearchDir(m.interview.root)
	_, _ = research.Create(researchDir, m.interview.targetID, time.Now())
}

func (m *Model) autoApplySuggestions() {
	if m.interview.targetID == "" {
		return
	}
	now := time.Now()
	suggDir := project.SuggestionsDir(m.interview.root)
	if err := os.MkdirAll(suggDir, 0o755); err != nil {
		m.status = "Suggestions failed: " + err.Error()
		return
	}
	suggPath, err := suggestions.Create(suggDir, m.interview.targetID, now)
	if err != nil {
		m.status = "Suggestions failed: " + err.Error()
		return
	}
	briefPath, err := writeSuggestionBrief(m.interview.root, m.interview.targetID, suggPath, now)
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
	applied, err := applyReadySuggestions(m.interview.root, m.interview.targetID, suggPath)
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
	m.input.SetText("")
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

func (m Model) renderInterviewLayout(width, height int) string {
	if height <= 0 {
		return ""
	}
	nav := m.renderInterviewHeaderNav(width)
	navLines := strings.Split(nav, "\n")
	remaining := height - len(navLines)
	if remaining <= 0 {
		return nav
	}
	topHeight := remaining / 2
	bottomHeight := remaining - topHeight
	if remaining >= 6 {
		if topHeight < 3 {
			topHeight = 3
			bottomHeight = remaining - topHeight
		}
		if bottomHeight < 3 {
			bottomHeight = 3
			topHeight = remaining - bottomHeight
		}
	}
	topContentHeight := max(1, topHeight-2)
	bottomContentHeight := max(1, bottomHeight-2)

	listContent := m.renderGroupListContent(topContentHeight)
	sectionTitle := m.interviewSectionTitle()
	sectionContent := m.interviewSectionContent()
	chatTop := m.renderInterviewChatContent(topContentHeight)
	chatBottom := m.renderInterviewChatContent(bottomContentHeight)

	if width < 100 {
		topTitle := sectionTitle
		topRightContent := sectionContent
		bottomTitle := "CHAT"
		bottomContent := chatBottom
		if m.interviewLayoutSwap {
			topTitle = "CHAT"
			topRightContent = chatTop
			bottomTitle = sectionTitle
			bottomContent = sectionContent
		}
		top := renderStackedLayout("PRDs", listContent, topTitle, topRightContent, width, topHeight)
		bottom := renderSingleColumnLayout(bottomTitle, bottomContent, width, bottomHeight)
		return strings.Join([]string{nav, top, bottom}, "\n")
	}

	topTitle := sectionTitle
	topRightContent := sectionContent
	bottomTitle := "CHAT"
	bottomContent := chatBottom
	if m.interviewLayoutSwap {
		topTitle = "CHAT"
		topRightContent = chatTop
		bottomTitle = sectionTitle
		bottomContent = sectionContent
	}
	top := renderDualColumnLayout("PRDs", listContent, topTitle, topRightContent, width, topHeight)
	bottom := renderSingleColumnLayout(bottomTitle, bottomContent, width, bottomHeight)
	return strings.Join([]string{nav, top, bottom}, "\n")
}

func (m Model) renderInterviewHeaderNav(width int) string {
	steps := []interviewStep{
		stepScanPrompt,
		stepDraftConfirm,
		stepBootstrapPrompt,
		stepVision,
		stepUsers,
		stepProblem,
		stepRequirements,
		stepResearchPrompt,
	}
	labels := make([]string, 0, len(steps))
	for _, step := range steps {
		prompt, _, _ := interviewStepInfo(step)
		label := prompt.title
		if step == m.interview.step {
			label = "[" + label + "]"
		}
		labels = append(labels, label)
	}
	nav := strings.Join(labels, "  ")
	return ensureExactWidth(nav, width)
}

func (m Model) interviewSectionTitle() string {
	prompt, _, _ := interviewStepInfo(m.interview.step)
	return "SECTION · " + prompt.title
}

func (m Model) interviewSectionContent() string {
	prompt, _, _ := interviewStepInfo(m.interview.step)
	content := "No content yet."
	if prompt.expectsText {
		if draft := strings.TrimSpace(m.interview.drafts[m.interview.step]); draft != "" {
			content = draft
		} else if answer := strings.TrimSpace(m.interview.answers[m.interview.step]); answer != "" {
			content = answer
		}
	} else {
		switch m.interview.step {
		case stepScanPrompt:
			if strings.TrimSpace(m.interview.scanSummary) != "" {
				content = m.interview.scanSummary
			} else {
				content = "Scan the repo to capture context."
			}
		case stepDraftConfirm:
			content = "Blank PRD ready."
		case stepResearchPrompt:
			content = "Research step pending."
		}
	}
	return "Open file: Ctrl+O\n\n" + content
}

func (m Model) renderInterviewChatContent(height int) string {
	if height <= 0 {
		return ""
	}
	composer := m.renderInterviewComposerLines()
	if height <= len(composer) {
		start := len(composer) - height
		if start < 0 {
			start = 0
		}
		return strings.Join(composer[start:], "\n")
	}
	transcriptHeight := height - len(composer) - 1
	transcript := m.renderInterviewTranscriptLines(transcriptHeight)
	lines := make([]string, 0, len(transcript)+1+len(composer))
	lines = append(lines, transcript...)
	lines = append(lines, "")
	lines = append(lines, composer...)
	return strings.Join(lines, "\n")
}

func (m Model) renderInterviewTranscriptLines(height int) []string {
	lines := []string{"PM-focused agent: Codex CLI / Claude Code"}
	if len(m.interview.chat) == 0 {
		lines = append(lines, "No messages yet.")
	} else {
		for _, msg := range m.interview.chat {
			role := formatInterviewRole(msg.Role)
			lines = append(lines, role+": "+msg.Text)
		}
	}
	if height <= 0 || len(lines) <= height {
		return lines
	}
	return lines[len(lines)-height:]
}

func (m Model) renderInterviewComposerLines() []string {
	lines := []string{"Compose:"}
	lines = append(lines, renderInputBoxLines(m.input.Render(6))...)
	lineNum, colNum := m.input.CursorPosition()
	lines = append(lines, fmt.Sprintf("Input (line %d, col %d)", lineNum, colNum))
	lines = append(lines, "Enter: iterate  [ / ]: prev/next")
	return lines
}

func (m Model) interviewMarkdown() string {
	prompt, stepNum, total := interviewStepInfo(m.interview.step)
	var b strings.Builder
	b.WriteString("# Interview\n")
	b.WriteString("**PM-focused agent:** Codex CLI / Claude Code\n\n")
	b.WriteString(fmt.Sprintf("**Step %d/%d: %s**\n\n", stepNum, total, prompt.title))
	b.WriteString("Hint: Enter iterate · [ / ] move steps\n\n")
	b.WriteString("Question:\n")
	b.WriteString(prompt.question)
	b.WriteString("\n\n")
	if m.interview.step == stepDraftConfirm {
		b.WriteString("Draft:\n")
		b.WriteString("Blank PRD ready.\n")
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
		if draft := strings.TrimSpace(m.interview.drafts[m.interview.step]); draft != "" {
			b.WriteString("Draft:\n")
			b.WriteString("```\n")
			b.WriteString(draft)
			b.WriteString("\n```\n\n")
		}
		b.WriteString("Conversation:\n")
		if len(m.interview.chat) == 0 {
			b.WriteString("No messages yet.\n\n")
		} else {
			for _, msg := range m.interview.chat {
				role := formatInterviewRole(msg.Role)
				b.WriteString(role)
				b.WriteString(": ")
				b.WriteString(msg.Text)
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
		b.WriteString("Compose:\n")
		b.WriteString("Input:\n")
		inputLines := renderInputBoxLines(m.input.Render(6))
		b.WriteString("```\n")
		for _, line := range inputLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("```\n")
		lineNum, colNum := m.input.CursorPosition()
		b.WriteString(fmt.Sprintf("Input (line %d, col %d)\n", lineNum, colNum))
		b.WriteString("Enter: iterate  [ / ]: prev/next\n")
	} else {
		b.WriteString("```\n")
		b.WriteString("> [1/2] (arrows + Enter)  [ / ]: prev/next\n")
		b.WriteString("```\n")
	}
	return b.String()
}

func formatInterviewRole(role string) string {
	trimmed := strings.TrimSpace(role)
	if trimmed == "" {
		return "Agent"
	}
	if len(trimmed) == 1 {
		return strings.ToUpper(trimmed)
	}
	return strings.ToUpper(trimmed[:1]) + trimmed[1:]
}

func (m Model) interviewStepsMarkdown() string {
	steps := []interviewStep{
		stepScanPrompt,
		stepDraftConfirm,
		stepBootstrapPrompt,
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

func buildSpecFromInterview(vision, users, problem, requirements string) specs.Spec {
	reqList := parseRequirements(requirements)
	if len(reqList) == 0 {
		reqList = []string{"REQ-001: TBD"}
	}
	firstReq := extractReqID(reqList[0])
	title := firstNonEmpty(vision, problem, "New PRD")
	summary := firstNonEmpty(problem, vision, "Summary pending")
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
			Text: "As a user, " + firstNonEmpty(users, "I need", "I need") + ", " + summary,
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

func mergeInterviewSpec(base specs.Spec, answers, drafts map[interviewStep]string) specs.Spec {
	vision := strings.TrimSpace(interviewValue(stepVision, answers, drafts))
	users := strings.TrimSpace(interviewValue(stepUsers, answers, drafts))
	problem := strings.TrimSpace(interviewValue(stepProblem, answers, drafts))
	requirements := strings.TrimSpace(interviewValue(stepRequirements, answers, drafts))

	updated := base
	if updated.Status == "" {
		updated.Status = "draft"
	}
	if updated.CreatedAt == "" {
		updated.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if vision != "" {
		updated.Title = vision
	}
	if problem != "" {
		updated.Summary = problem
	}
	if requirements != "" {
		reqList := parseRequirements(requirements)
		if len(reqList) == 0 {
			reqList = []string{"REQ-001: TBD"}
		}
		updated.Requirements = reqList
	}
	if users != "" || problem != "" {
		summary := updated.Summary
		if strings.TrimSpace(summary) == "" {
			summary = firstNonEmpty(problem, vision, "Summary pending")
		}
		updated.UserStory = specs.UserStory{
			Text: "As a user, " + firstNonEmpty(users, "I need", "I need") + ", " + summary,
			Hash: "pending",
		}
	}
	if updated.StrategicContext.CUJID == "" && updated.StrategicContext.CUJName == "" && updated.StrategicContext.FeatureID == "" {
		updated.StrategicContext = specs.StrategicContext{
			CUJID:       "CUJ-001",
			CUJName:     "Primary Journey",
			FeatureID:   "",
			MVPIncluded: true,
		}
	}
	if len(updated.CriticalUserJourneys) == 0 && len(updated.Requirements) > 0 {
		firstReq := extractReqID(updated.Requirements[0])
		updated.CriticalUserJourneys = []specs.CriticalUserJourney{
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
		}
	}
	return updated
}

func interviewValue(step interviewStep, answers, drafts map[interviewStep]string) string {
	if drafts != nil {
		if val := strings.TrimSpace(drafts[step]); val != "" {
			return val
		}
	}
	if answers == nil {
		return ""
	}
	return answers[step]
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

func renderInputBoxLines(lines []string) []string {
	width := 0
	for _, line := range lines {
		if l := runeCount(line); l > width {
			width = l
		}
	}
	if width < 20 {
		width = 20
	}
	top := "+" + strings.Repeat("-", width+2) + "+"
	bottom := top
	box := []string{top}
	for _, line := range lines {
		padding := width - runeCount(line)
		box = append(box, "| "+line+strings.Repeat(" ", padding)+" |")
	}
	box = append(box, bottom)
	return box
}

var osWriteFile = os.WriteFile

type interviewPrompt struct {
	title       string
	question    string
	options     []string
	expectsText bool
}

func interviewStepInfo(step interviewStep) (interviewPrompt, int, int) {
	total := 8
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
	case stepBootstrapPrompt:
		return interviewPrompt{
			title:    "Bootstrap",
			question: "Generate initial answers from the codebase?",
			options:  []string{"1) Yes - run coding agent", "2) No - skip bootstrap"},
		}, 3, total
	case stepVision:
		return interviewPrompt{
			title:       "Vision",
			question:    "What is the vision?",
			expectsText: true,
		}, 4, total
	case stepUsers:
		return interviewPrompt{
			title:       "Users",
			question:    "Who are the primary users?",
			expectsText: true,
		}, 5, total
	case stepProblem:
		return interviewPrompt{
			title:       "Problem",
			question:    "What problem are we solving?",
			expectsText: true,
		}, 6, total
	case stepRequirements:
		return interviewPrompt{
			title:       "Requirements",
			question:    "List requirements (comma or newline separated).",
			expectsText: true,
		}, 7, total
	case stepResearchPrompt:
		return interviewPrompt{
			title:    "Research",
			question: "Run research now?",
			options:  []string{"1) Yes - create research brief", "2) No - skip for now"},
		}, 8, total
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
