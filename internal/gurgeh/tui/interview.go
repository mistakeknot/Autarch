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
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/gurgeh/agents"
	"github.com/mistakeknot/autarch/internal/gurgeh/config"
	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/research"
	"github.com/mistakeknot/autarch/internal/gurgeh/scan"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/internal/gurgeh/suggestions"
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
	stepGoals
	stepNonGoals
	stepAssumptions
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
	case stepGoals:
		m.handleTextStep(msg, stepGoals)
	case stepNonGoals:
		m.handleTextStep(msg, stepNonGoals)
	case stepAssumptions:
		m.handleTextStep(msg, stepAssumptions)
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

	switch key {
	case "enter":
		m.storeInterviewAnswer(step)
		m.appendInterviewMessage("user", m.interview.answerForStep(step))
		m.iterateInterviewStep(step)
		return
	default:
		// Pass all other keys to the chat panel (handles input, cursor movement, etc.)
		if m.chatPanel != nil {
			m.chatPanel, _ = m.chatPanel.Update(msg)
		}
	}
	m.storeInterviewAnswer(step)
}

func (m *Model) appendInterviewMessage(role, text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	// Add to chat panel (primary)
	if m.chatPanel != nil {
		m.chatPanel.AddMessage(role, trimmed)
	}
	// Also keep in interview state for backward compatibility with existing logic
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
	if m.chatPanel != nil {
		m.interview.answers[step] = m.chatPanel.Value()
	}
}

func (m *Model) loadInterviewInput() {
	prompt, _, _ := interviewStepInfo(m.interview.step)
	if m.chatPanel == nil {
		return
	}
	if !prompt.expectsText {
		m.chatPanel.SetValue("")
		return
	}
	m.chatPanel.SetValue(m.interview.answerForStep(m.interview.step))
	m.updateInterviewDocPanel()
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
	case stepGoals:
		m.interview.step = stepProblem
	case stepNonGoals:
		m.interview.step = stepGoals
	case stepAssumptions:
		m.interview.step = stepNonGoals
	case stepRequirements:
		m.interview.step = stepAssumptions
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
		m.advanceTextStep(stepGoals)
	case stepGoals:
		m.advanceTextStep(stepNonGoals)
	case stepNonGoals:
		m.advanceTextStep(stepAssumptions)
	case stepAssumptions:
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
	if m.chatPanel != nil {
		m.chatPanel.ClearComposer()
		m.chatPanel.ClearMessages()
	}
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

	// Header with breadcrumbs and navigation
	breadcrumbs := m.renderInterviewBreadcrumbs(width)
	nav := m.renderInterviewHeaderNav(width)
	header := strings.Join([]string{breadcrumbs, nav}, "\n")
	headerLines := strings.Split(header, "\n")
	remaining := height - len(headerLines)
	if remaining <= 0 {
		return header
	}

	// Use shared split layout for Cursor-style view
	if m.splitLayout == nil || m.docPanel == nil || m.chatPanel == nil {
		// Fallback if components not initialized
		return header + "\n" + m.renderInterviewLayoutLegacy(width, remaining)
	}

	// Update layout dimensions
	m.splitLayout.SetSize(width, remaining)
	m.docPanel.SetSize(m.splitLayout.LeftWidth(), m.splitLayout.LeftHeight())
	m.chatPanel.SetSize(m.splitLayout.RightWidth(), m.splitLayout.RightHeight())

	// Render split layout: document on left, chat on right
	leftContent := m.docPanel.View()
	rightContent := m.chatPanel.View()

	splitView := m.splitLayout.Render(leftContent, rightContent)
	return lipgloss.JoinVertical(lipgloss.Left, header, splitView)
}

// renderInterviewLayoutLegacy is the old rendering logic for fallback.
func (m Model) renderInterviewLayoutLegacy(width, height int) string {
	topHeight := height / 2
	bottomHeight := height - topHeight
	if height >= 6 {
		if topHeight < 3 {
			topHeight = 3
			bottomHeight = height - topHeight
		}
		if bottomHeight < 3 {
			bottomHeight = 3
			topHeight = height - bottomHeight
		}
	}
	topContentHeight := max(1, topHeight-2)
	bottomContentHeight := max(1, bottomHeight-2)

	listContent := m.renderGroupListContent(topContentHeight)
	sectionTitle := m.interviewSectionTitle()
	sectionContent := m.interviewSectionContent()
	chatBottom := m.renderInterviewChatContentLegacy(bottomContentHeight)

	if width < 100 {
		top := renderStackedLayout("PRDs", listContent, sectionTitle, sectionContent, width, topHeight)
		bottom := renderSingleColumnLayout("CHAT", chatBottom, width, bottomHeight)
		return strings.Join([]string{top, bottom}, "\n")
	}

	top := renderDualColumnLayout("PRDs", listContent, sectionTitle, sectionContent, width, topHeight)
	bottom := renderSingleColumnLayout("CHAT", chatBottom, width, bottomHeight)
	return strings.Join([]string{top, bottom}, "\n")
}

func (m Model) renderInterviewBreadcrumbs(width int) string {
	label := "PRDs > Interview"
	if strings.TrimSpace(m.interview.targetID) != "" {
		label = "PRDs > " + m.interview.targetID + " > Interview"
	}
	return ensureExactWidth(label, width)
}

func (m Model) renderInterviewHeaderNav(width int) string {
	steps := []interviewStep{
		stepScanPrompt,
		stepDraftConfirm,
		stepBootstrapPrompt,
		stepVision,
		stepUsers,
		stepProblem,
		stepGoals,
		stepNonGoals,
		stepAssumptions,
		stepRequirements,
		stepResearchPrompt,
	}
	labels := make([]string, 0, len(steps))
	activeIndex := 0
	for idx, step := range steps {
		prompt, _, _ := interviewStepInfo(step)
		label := "[" + prompt.title + "]"
		if step == m.interview.step {
			label = "[[" + prompt.title + "]]"
			activeIndex = idx
		}
		labels = append(labels, label)
	}
	if width < 80 {
		start := activeIndex - 1
		if start < 0 {
			start = 0
		}
		end := activeIndex + 1
		if end >= len(labels) {
			end = len(labels) - 1
		}
		collapsed := make([]string, 0, 5)
		if start > 0 {
			collapsed = append(collapsed, "...")
		}
		collapsed = append(collapsed, labels[start:end+1]...)
		if end < len(labels)-1 {
			collapsed = append(collapsed, "...")
		}
		nav := strings.Join(collapsed, "  ")
		return ensureExactWidth(nav, width)
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

// renderInterviewChatContentLegacy is the old chat rendering for fallback mode.
func (m Model) renderInterviewChatContentLegacy(height int) string {
	if height <= 0 {
		return ""
	}
	composer := m.renderInterviewComposerLinesLegacy()
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
			lines = append(lines, "["+role+"]")
			lines = append(lines, "  "+msg.Text)
			lines = append(lines, "")
		}
	}
	if height <= 0 || len(lines) <= height {
		return lines
	}
	return lines[len(lines)-height:]
}

// renderInterviewComposerLinesLegacy is the old composer rendering for fallback mode.
func (m Model) renderInterviewComposerLinesLegacy() []string {
	prompt, _, _ := interviewStepInfo(m.interview.step)
	title := "Compose · " + prompt.title
	lines := []string{renderComposerTitle(title)}
	// Create a simple text display since TextBuffer is no longer used
	text := ""
	if m.chatPanel != nil {
		text = m.chatPanel.Value()
	}
	textLines := strings.Split(text, "\n")
	if len(textLines) == 0 {
		textLines = []string{""}
	}
	// Limit to 6 lines for display
	if len(textLines) > 6 {
		textLines = textLines[len(textLines)-6:]
	}
	lines = append(lines, renderInputBoxLines(textLines)...)
	lines = append(lines, "enter: send  ctrl+j: newline  [/]: nav  ctrl+o: open")
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
		text := ""
		if m.chatPanel != nil {
			text = m.chatPanel.Value()
		}
		textLines := strings.Split(text, "\n")
		if len(textLines) == 0 {
			textLines = []string{""}
		}
		if len(textLines) > 6 {
			textLines = textLines[len(textLines)-6:]
		}
		inputLines := renderInputBoxLines(textLines)
		b.WriteString("```\n")
		for _, line := range inputLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("```\n")
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
		stepGoals,
		stepNonGoals,
		stepAssumptions,
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
	goals := strings.TrimSpace(interviewValue(stepGoals, answers, drafts))
	nonGoals := strings.TrimSpace(interviewValue(stepNonGoals, answers, drafts))
	assumptions := strings.TrimSpace(interviewValue(stepAssumptions, answers, drafts))
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
	if goals != "" {
		updated.Goals = parseGoals(goals)
	}
	if nonGoals != "" {
		updated.NonGoals = parseNonGoals(nonGoals)
	}
	if assumptions != "" {
		updated.Assumptions = parseAssumptions(assumptions)
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

// parseGoals converts comma/newline separated goals into structured Goal objects.
func parseGoals(input string) []specs.Goal {
	parts := splitInput(input)
	var out []specs.Goal
	for i, part := range parts {
		out = append(out, specs.Goal{
			ID:          fmt.Sprintf("GOAL-%s", pad3(i+1)),
			Description: part,
			Metric:      "", // User can refine later
			Target:      "", // User can refine later
		})
	}
	return out
}

// parseNonGoals converts comma/newline separated non-goals into structured NonGoal objects.
func parseNonGoals(input string) []specs.NonGoal {
	parts := splitInput(input)
	var out []specs.NonGoal
	for i, part := range parts {
		out = append(out, specs.NonGoal{
			ID:          fmt.Sprintf("NG-%s", pad3(i+1)),
			Description: part,
			Rationale:   "", // User can refine later
		})
	}
	return out
}

// parseAssumptions converts comma/newline separated assumptions into structured Assumption objects.
func parseAssumptions(input string) []specs.Assumption {
	parts := splitInput(input)
	var out []specs.Assumption
	for i, part := range parts {
		out = append(out, specs.Assumption{
			ID:            fmt.Sprintf("ASSM-%s", pad3(i+1)),
			Description:   part,
			ImpactIfFalse: "", // User can refine later
			Confidence:    "medium",
		})
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
	// Use lipgloss-style rounded corners for consistency with other views
	top := "╭" + strings.Repeat("─", width+2) + "╮"
	bottom := "╰" + strings.Repeat("─", width+2) + "╯"
	box := []string{top}
	for _, line := range lines {
		padding := width - runeCount(line)
		box = append(box, "│ "+line+strings.Repeat(" ", padding)+" │")
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
	total := 11 // Updated for new steps
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
	case stepGoals:
		return interviewPrompt{
			title:       "Goals",
			question:    "What are the measurable goals? (comma or newline separated)",
			expectsText: true,
		}, 7, total
	case stepNonGoals:
		return interviewPrompt{
			title:       "Non-Goals",
			question:    "What is explicitly out of scope? (comma or newline separated)",
			expectsText: true,
		}, 8, total
	case stepAssumptions:
		return interviewPrompt{
			title:       "Assumptions",
			question:    "What assumptions does this PRD rely on? (comma or newline separated)",
			expectsText: true,
		}, 9, total
	case stepRequirements:
		return interviewPrompt{
			title:       "Requirements",
			question:    "List requirements (comma or newline separated).",
			expectsText: true,
		}, 10, total
	case stepResearchPrompt:
		return interviewPrompt{
			title:    "Research",
			question: "Run research now?",
			options:  []string{"1) Yes - create research brief", "2) No - skip for now"},
		}, 11, total
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
