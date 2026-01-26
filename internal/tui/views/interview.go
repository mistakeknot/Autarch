package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/autarch/drafts"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/internal/tui/components"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// InterviewQuestion is an alias to the type in the tui package.
type InterviewQuestion = tui.InterviewQuestion

// InterviewView provides an enhanced interview flow with research integration.
// Uses a Cursor-style split layout: left pane (2/3) for document, right pane (1/3) for chat.
type InterviewView struct {
	questions []InterviewQuestion
	current   int
	answers   map[string]string
	states    map[string]*drafts.QuestionState

	// Research integration
	coordinator     *research.Coordinator
	researchOverlay *ResearchOverlay
	teasers         map[string]*components.Teaser
	tradeoffs       map[string]*components.Tradeoff
	showResearch    bool

	// Chat history for the right pane
	chatHistory []pkgtui.ChatMessage

	// UI components - using shared components
	chatPanel   *pkgtui.ChatPanel
	docPanel    *pkgtui.DocPanel
	splitLayout *pkgtui.SplitLayout
	optionIndex int
	width       int
	height      int
	focused     bool

	// Draft persistence
	draft      *drafts.Draft
	draftStore *drafts.Store

	// Callbacks
	onComplete func(answers map[string]string) tea.Cmd
}

// NewInterviewView creates a new interview view with research integration.
func NewInterviewView(questions []InterviewQuestion, coordinator *research.Coordinator) *InterviewView {
	// Create shared components
	chatPanel := pkgtui.NewChatPanel()
	chatPanel.SetComposerHint("enter: send  ctrl+j: newline  [/]: nav")

	docPanel := pkgtui.NewDocPanel()

	splitLayout := pkgtui.NewSplitLayout(0.66) // 2/3 left, 1/3 right
	splitLayout.SetMinWidth(100)               // Fall back to stacked below 100 chars

	v := &InterviewView{
		questions:       questions,
		current:         0,
		answers:         make(map[string]string),
		states:          make(map[string]*drafts.QuestionState),
		coordinator:     coordinator,
		teasers:         make(map[string]*components.Teaser),
		tradeoffs:       make(map[string]*components.Tradeoff),
		chatHistory:     []pkgtui.ChatMessage{},
		chatPanel:       chatPanel,
		docPanel:        docPanel,
		splitLayout:     splitLayout,
		researchOverlay: NewResearchOverlay(coordinator),
	}

	// Initialize teasers for each question
	for _, q := range questions {
		v.teasers[q.ID] = components.NewTeaser(q.TopicKey, components.TeaserHeight)
		v.tradeoffs[q.ID] = components.NewTradeoff()
	}

	return v
}

// SetDraft sets the draft for persistence and restores state.
func (v *InterviewView) SetDraft(draft *drafts.Draft, store *drafts.Store) {
	v.draft = draft
	v.draftStore = store

	// Restore state from draft
	if draft.Interview != nil {
		v.current = draft.Interview.CurrentStep
		for k, val := range draft.Interview.Answers {
			if str, ok := val.(string); ok {
				v.answers[k] = str
			}
		}
		for _, qs := range draft.Interview.Questions {
			v.states[qs.QuestionID] = &qs
		}
	}

	v.loadCurrentInput()
}

// SetCompleteCallback sets the callback for when interview is complete.
func (v *InterviewView) SetCompleteCallback(cb func(answers map[string]string) tea.Cmd) {
	v.onComplete = cb
}

// SetSuggestions sets AI-generated suggestions for interview answers.
// These will be pre-filled in the input fields for the user to review/edit.
func (v *InterviewView) SetSuggestions(suggestions map[string]string) {
	for id, suggestion := range suggestions {
		// Only set if user hasn't already answered this question
		if _, exists := v.answers[id]; !exists {
			v.answers[id] = suggestion
		}
	}
	// Reload current input to show suggestion
	v.loadCurrentInput()
}

// Init implements View
func (v *InterviewView) Init() tea.Cmd {
	v.loadCurrentInput()
	return textarea.Blink
}

// currentQuestion returns the current interview question.
func (v *InterviewView) currentQuestion() *InterviewQuestion {
	if v.current >= 0 && v.current < len(v.questions) {
		return &v.questions[v.current]
	}
	return nil
}

// Update implements View
func (v *InterviewView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle research overlay if visible
	if v.showResearch {
		var cmd tea.Cmd
		v.researchOverlay, cmd = v.researchOverlay.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

		// Check for close
		if !v.researchOverlay.Visible() {
			v.showResearch = false
		}
		return v, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Account for unified_app's content padding (Padding(1, 3) = 6 horizontal, 2 vertical)
		v.width = msg.Width - 6
		v.height = msg.Height - 4 - 2

		// Update layout dimensions
		v.splitLayout.SetSize(v.width, v.height)
		v.docPanel.SetSize(v.splitLayout.LeftWidth(), v.splitLayout.LeftHeight())
		v.chatPanel.SetSize(v.splitLayout.RightWidth(), v.splitLayout.RightHeight())

		v.researchOverlay.SetSize(v.width, v.height)
		for _, teaser := range v.teasers {
			teaser.SetSize(v.splitLayout.LeftWidth() - 4)
		}
		for _, tradeoff := range v.tradeoffs {
			tradeoff.SetSize(v.splitLayout.LeftWidth() - 4)
		}
		return v, nil

	// Research messages
	case research.HunterUpdateMsg, research.HunterCompletedMsg:
		v.updateResearchForCurrentQuestion()
		return v, nil

	case research.RunCompletedMsg:
		v.updateResearchForCurrentQuestion()
		return v, nil

	// Tradeoff selection
	case components.TradeoffSelectedMsg:
		q := v.currentQuestion()
		if q != nil {
			v.answers[q.ID] = msg.Option.Label
			v.chatPanel.SetValue(msg.Option.Label)
			v.markQuestionTouched(q.ID)
		}
		return v, nil

	case tea.KeyMsg:
		return v.handleKeyMsg(msg)
	}

	return v, tea.Batch(cmds...)
}

func (v *InterviewView) handleKeyMsg(msg tea.KeyMsg) (tui.View, tea.Cmd) {
	var cmds []tea.Cmd

	// For text input questions, pass most keys to chat panel
	// except for special navigation keys
	if !v.isChoiceQuestion() {
		switch msg.String() {
		case "ctrl+r":
			v.showResearch = true
			return v, v.researchOverlay.Show()

		case "enter":
			return v, v.handleEnter()

		case "ctrl+j":
			// Insert newline - let chat panel handle it
			var cmd tea.Cmd
			v.chatPanel, cmd = v.chatPanel.Update(msg)
			if q := v.currentQuestion(); q != nil {
				v.markQuestionTouched(q.ID)
			}
			return v, cmd

		case "[":
			// Previous question
			if v.current > 0 {
				v.saveCurrent()
				v.current--
				v.loadCurrentInput()
			}
			return v, nil

		case "]":
			// Next question
			if v.current < len(v.questions)-1 {
				v.saveCurrent()
				v.current++
				v.loadCurrentInput()
			}
			return v, nil

		default:
			// Pass all other keys to chat panel
			var cmd tea.Cmd
			v.chatPanel, cmd = v.chatPanel.Update(msg)
			if q := v.currentQuestion(); q != nil {
				v.markQuestionTouched(q.ID)
			}
			return v, cmd
		}
	}

	// Choice question key handling
	switch msg.String() {
	case "ctrl+r":
		// Toggle research overlay
		v.showResearch = true
		return v, v.researchOverlay.Show()

	case "enter":
		return v, v.handleEnter()

	case "up", "k":
		if v.optionIndex > 0 {
			v.optionIndex--
		}
		return v, nil

	case "down", "j":
		q := v.currentQuestion()
		if q != nil && v.optionIndex < len(q.Options)-1 {
			v.optionIndex++
		}
		return v, nil

	case "[":
		// Previous question
		if v.current > 0 {
			v.saveCurrent()
			v.current--
			v.loadCurrentInput()
		}
		return v, nil

	case "]":
		// Next question (without completing)
		if v.current < len(v.questions)-1 {
			v.saveCurrent()
			v.current++
			v.loadCurrentInput()
		}
		return v, nil

	case "1", "2", "3":
		// Handle tradeoff selection
		q := v.currentQuestion()
		if q != nil {
			tradeoff := v.tradeoffs[q.ID]
			if tradeoff != nil && tradeoff.HasOptions() {
				var cmd tea.Cmd
				tradeoff, cmd = tradeoff.Update(msg)
				v.tradeoffs[q.ID] = tradeoff
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				return v, tea.Batch(cmds...)
			}
		}

		// Also handle as option selection for choice questions
		idx := int(msg.String()[0] - '1')
		if q != nil && idx >= 0 && idx < len(q.Options) {
			v.optionIndex = idx
			return v, v.handleEnter()
		}
	}

	return v, tea.Batch(cmds...)
}

func (v *InterviewView) handleEnter() tea.Cmd {
	v.saveCurrent()

	// Add user message to chat history
	q := v.currentQuestion()
	if q != nil {
		answer := v.answers[q.ID]
		if answer != "" {
			v.chatPanel.AddMessage("user", answer)
		}
	}

	// Advance to next question
	if v.current < len(v.questions)-1 {
		v.current++
		v.loadCurrentInput()
		v.maybeApplyResearchDefault()
		return nil
	}

	// Complete interview
	if v.onComplete != nil {
		return v.onComplete(v.answers)
	}
	return nil
}

func (v *InterviewView) saveCurrent() {
	q := v.currentQuestion()
	if q == nil {
		return
	}

	if v.isChoiceQuestion() {
		if v.optionIndex >= 0 && v.optionIndex < len(q.Options) {
			v.answers[q.ID] = q.Options[v.optionIndex]
		}
	} else {
		v.answers[q.ID] = v.chatPanel.Value()
	}

	// Persist to draft
	v.saveDraft()
}

func (v *InterviewView) loadCurrentInput() {
	q := v.currentQuestion()
	if q == nil {
		return
	}

	// Update composer placeholder and value
	v.chatPanel.SetComposerPlaceholder(q.Placeholder)
	if answer, ok := v.answers[q.ID]; ok {
		v.chatPanel.SetValue(answer)
	} else {
		v.chatPanel.SetValue("")
	}

	// Update composer title to show current step
	v.chatPanel.SetComposerTitle(q.Title)

	// Reset option index for choice questions
	if len(q.Options) > 0 {
		v.optionIndex = 0
		for i, opt := range q.Options {
			if opt == v.answers[q.ID] {
				v.optionIndex = i
				break
			}
		}
	}

	// Update document panel content
	v.updateDocPanel()

	// Update research for this question
	v.updateResearchForCurrentQuestion()
}

func (v *InterviewView) updateDocPanel() {
	q := v.currentQuestion()
	if q == nil {
		return
	}

	v.docPanel.ClearSections()
	v.docPanel.SetTitle(q.Title)
	v.docPanel.SetSubtitle(q.Prompt)

	// Add research teaser if available
	teaser := v.teasers[q.ID]
	if teaser != nil {
		teaserContent := teaser.View()
		if strings.TrimSpace(teaserContent) != "" {
			v.docPanel.AddSection(pkgtui.ResearchSection(teaserContent))
		}
	}

	// Add tradeoff suggestions if available
	tradeoff := v.tradeoffs[q.ID]
	if tradeoff != nil && tradeoff.HasOptions() {
		var tradeoffContent string
		if v.isQuestionTouched(q.ID) {
			tradeoffContent = tradeoff.ViewFYI()
		} else {
			tradeoffContent = tradeoff.View()
		}
		if strings.TrimSpace(tradeoffContent) != "" {
			v.docPanel.AddSection(pkgtui.TradeoffSection(tradeoffContent))
		}
	}

	// Add options for choice questions
	if len(q.Options) > 0 {
		v.docPanel.AddSection(pkgtui.InfoSection("Options", v.renderOptionsContent(q)))
	}
}

func (v *InterviewView) isChoiceQuestion() bool {
	q := v.currentQuestion()
	return q != nil && len(q.Options) > 0
}

func (v *InterviewView) markQuestionTouched(questionID string) {
	if v.states[questionID] == nil {
		v.states[questionID] = &drafts.QuestionState{
			QuestionID: questionID,
		}
	}
	v.states[questionID].Touched = true
}

func (v *InterviewView) isQuestionTouched(questionID string) bool {
	if state, ok := v.states[questionID]; ok {
		return state.Touched
	}
	return false
}

func (v *InterviewView) updateResearchForCurrentQuestion() {
	q := v.currentQuestion()
	if q == nil || v.coordinator == nil {
		return
	}

	run := v.coordinator.GetActiveRun()
	if run == nil {
		return
	}

	// Update teaser
	teaser := v.teasers[q.ID]
	if teaser != nil {
		updates := run.GetUpdatesForTopic(q.TopicKey)
		teaser.UpdateFromRun(run.RunID, updates)
	}

	// Update tradeoffs from findings
	tradeoff := v.tradeoffs[q.ID]
	if tradeoff != nil {
		var allFindings []research.Finding
		for _, update := range run.GetUpdatesForTopic(q.TopicKey) {
			allFindings = append(allFindings, update.Findings...)
		}
		options := components.CreateFromFindings(allFindings, q.TopicKey)
		tradeoff.SetOptions(options)
	}

	// Update doc panel to reflect research changes
	v.updateDocPanel()
}

func (v *InterviewView) maybeApplyResearchDefault() {
	q := v.currentQuestion()
	if q == nil {
		return
	}

	// Don't apply if already touched or default already applied
	if v.isQuestionTouched(q.ID) {
		return
	}
	if state, ok := v.states[q.ID]; ok && state.DefaultApplied {
		return
	}

	// Check for research suggestions
	tradeoff := v.tradeoffs[q.ID]
	if tradeoff == nil || !tradeoff.HasOptions() {
		return
	}

	// Apply first suggestion as default
	if opt := tradeoff.GetOption(0); opt != nil {
		v.chatPanel.SetValue(opt.Label)
		v.answers[q.ID] = opt.Label

		// Mark default as applied
		if v.states[q.ID] == nil {
			v.states[q.ID] = &drafts.QuestionState{
				QuestionID: q.ID,
			}
		}
		v.states[q.ID].DefaultApplied = true
	}
}

func (v *InterviewView) saveDraft() {
	if v.draft == nil || v.draftStore == nil {
		return
	}

	// Update draft interview state
	if v.draft.Interview == nil {
		v.draft.Interview = &drafts.InterviewState{
			Answers: make(map[string]any),
		}
	}

	v.draft.Interview.CurrentStep = v.current
	v.draft.Interview.TotalSteps = len(v.questions)
	for k, val := range v.answers {
		v.draft.Interview.Answers[k] = val
	}

	// Update question states
	v.draft.Interview.Questions = nil
	for id, state := range v.states {
		v.draft.Interview.Questions = append(v.draft.Interview.Questions, drafts.QuestionState{
			QuestionID:     id,
			Touched:        state.Touched,
			DefaultApplied: state.DefaultApplied,
		})
	}

	// Save asynchronously
	go v.draftStore.Save(v.draft)
}

// View implements View
func (v *InterviewView) View() string {
	// Check for research overlay
	if v.showResearch {
		return v.renderWithResearchOverlay()
	}

	return v.renderSplitLayout()
}

func (v *InterviewView) renderSplitLayout() string {
	q := v.currentQuestion()
	if q == nil {
		return pkgtui.LabelStyle.Render("No questions configured")
	}

	var sections []string

	// Progress bar at top (outside split layout)
	progress := v.renderProgress()
	sections = append(sections, progress)
	sections = append(sections, "")

	// Render the split layout
	leftContent := v.docPanel.View()
	rightContent := v.chatPanel.View()

	splitView := v.splitLayout.Render(leftContent, rightContent)
	sections = append(sections, splitView)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (v *InterviewView) renderWithResearchOverlay() string {
	// Wide terminals: side-by-side
	if v.width >= 120 {
		mainWidth := v.width * 2 / 3
		overlayWidth := v.width - mainWidth

		mainView := v.renderSplitLayout()
		overlayView := v.researchOverlay.View()

		mainStyle := lipgloss.NewStyle().Width(mainWidth)
		overlayStyle := lipgloss.NewStyle().Width(overlayWidth)

		return lipgloss.JoinHorizontal(lipgloss.Top,
			mainStyle.Render(mainView),
			overlayStyle.Render(overlayView),
		)
	}

	// Narrow terminals: overlay
	return v.researchOverlay.View()
}

func (v *InterviewView) renderProgress() string {
	total := len(v.questions)
	if total == 0 {
		return "No questions configured"
	}
	current := v.current + 1

	// Progress bar - ensure positive values
	barWidth := min(40, v.width-20)
	if barWidth <= 0 {
		barWidth = 20
	}
	filled := (barWidth * current) / total
	empty := barWidth - filled
	if filled < 0 {
		filled = 0
	}
	if empty < 0 {
		empty = 0
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	barStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorPrimary)

	return fmt.Sprintf("Step %d/%d  %s", current, total, barStyle.Render(bar))
}

func (v *InterviewView) renderOptionsContent(q *InterviewQuestion) string {
	var lines []string
	for i, opt := range q.Options {
		var line string
		if i == v.optionIndex {
			line = fmt.Sprintf("> %d) %s", i+1, opt)
		} else {
			line = fmt.Sprintf("  %d) %s", i+1, opt)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// Focus implements View
func (v *InterviewView) Focus() tea.Cmd {
	v.focused = true
	return v.chatPanel.Focus()
}

// Blur implements View
func (v *InterviewView) Blur() {
	v.focused = false
	v.chatPanel.Blur()
	v.saveDraft()
}

// Name implements View
func (v *InterviewView) Name() string {
	return "Interview"
}

// ShortHelp implements View
func (v *InterviewView) ShortHelp() string {
	if v.isChoiceQuestion() {
		return "enter: next  [/]: nav  j/k: select  ctrl+r: research"
	}
	return "enter: send  ctrl+j: newline  [/]: nav  ctrl+r: research"
}

// GetAnswers returns all collected answers.
func (v *InterviewView) GetAnswers() map[string]string {
	return v.answers
}

// DefaultInterviewQuestions returns the standard onboarding questions.
// This is a convenience wrapper around tui.DefaultInterviewQuestions.
func DefaultInterviewQuestions() []InterviewQuestion {
	return tui.DefaultInterviewQuestions()
}
