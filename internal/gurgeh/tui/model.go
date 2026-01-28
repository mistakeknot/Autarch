package tui

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/gurgeh/agents"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/gurgeh/archive"
	"github.com/mistakeknot/autarch/internal/gurgeh/config"
	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/research"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/internal/gurgeh/suggestions"
	pollardquick "github.com/mistakeknot/autarch/internal/pollard/quick"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

type Model struct {
	summaries         []specs.Summary
	selected          int
	viewOffset        int
	groupExpanded     map[string]bool
	groupTree         *GroupTree
	flatItems         []Item
	err               string
	root              string
	mode              string
	status            string
	router            Router
	width             int
	height            int
	mdCache           *MarkdownCache
	overlay           string
	focus             string
	search            SearchState
	searchOverlay     *SearchOverlay
	showArchived      bool
	confirmAction     string
	confirmMessage    string
	confirmID         string
	pendingPrevStatus string
	lastAction        *LastAction
	keys              pkgtui.CommonKeys
	helpOverlay       pkgtui.HelpOverlay
	sprint            *SprintView
	suggestions       suggestionsState
}

func NewModel() Model {
	cwd, err := os.Getwd()
	if err != nil {
		return Model{err: err.Error(), mode: "list", keys: pkgtui.NewCommonKeys(), helpOverlay: pkgtui.NewHelpOverlay()}
	}
	if err := project.EnsureInitialized(cwd); err != nil {
		model := Model{
			err:         err.Error(),
			root:        cwd,
			mode:        "list",
			router:      Router{active: "list"},
			width:       120,
			height:      40,
			mdCache:     NewMarkdownCache(),
			focus:       "LIST",
			keys:        pkgtui.NewCommonKeys(),
			helpOverlay: pkgtui.NewHelpOverlay(),
		}
		model.searchOverlay = NewSearchOverlay()
		model.groupExpanded = defaultExpanded()

		if state, err := LoadUIState(project.StatePath(cwd)); err == nil {
			if state.Expanded != nil {
				model.groupExpanded = state.Expanded
			}
			model.showArchived = state.ShowArchived
			model.lastAction = state.LastAction
			model.rebuildGroups()
			if state.SelectedID != "" {
				model.selected = selectedIndexFromID(model.flatItems, state.SelectedID)
				model.viewOffset = clampViewOffset(model.selected, model.viewOffset, model.listContentHeight(), len(model.flatItems))
			}
		} else {
			if !os.IsNotExist(err) {
				model.status = "state load failed"
			}
			model.rebuildGroups()
		}
		return model
	}
	state, stateErr := LoadUIState(project.StatePath(cwd))
	includeArchived := stateErr == nil && state.ShowArchived
	list, _ := specs.LoadSummariesWithArchived(project.SpecsDir(cwd), project.ArchivedSpecsDir(cwd), includeArchived)
	model := Model{
		summaries:    list,
		root:         cwd,
		mode:         "list",
		router:       Router{active: "list"},
		width:        120,
		height:       40,
		mdCache:      NewMarkdownCache(),
		focus:        "LIST",
		showArchived: includeArchived,
		keys:         pkgtui.NewCommonKeys(),
		helpOverlay:  pkgtui.NewHelpOverlay(),
	}
	model.searchOverlay = NewSearchOverlay()
	model.searchOverlay.SetItems(list)
	model.groupExpanded = defaultExpanded()

	if stateErr == nil {
		if state.Expanded != nil {
			model.groupExpanded = state.Expanded
		}
		model.showArchived = state.ShowArchived
		model.lastAction = state.LastAction
		model.rebuildGroups()
		if state.SelectedID != "" {
			model.selected = selectedIndexFromID(model.flatItems, state.SelectedID)
			model.viewOffset = clampViewOffset(model.selected, model.viewOffset, model.listContentHeight(), len(model.flatItems))
		}
	} else {
		if !os.IsNotExist(stateErr) {
			model.status = "state load failed"
		}
		model.rebuildGroups()
	}
	return model
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		keyStr := msg.String()
		if msg.Type == tea.KeyEnter {
			keyStr = "enter"
		}
		if msg.Type == tea.KeyBackspace {
			keyStr = "backspace"
		}
		if m.confirmAction != "" {
			switch {
			case key.Matches(msg, m.keys.Select):
				m.applyConfirmAction()
			case key.Matches(msg, m.keys.Back), key.Matches(msg, m.keys.Quit):
				m.clearConfirm()
			}
			return m, nil
		}
		if m.overlay == "tutorial" {
			switch {
			case key.Matches(msg, m.keys.Back), key.Matches(msg, m.keys.Quit):
				m.overlay = ""
			case key.Matches(msg, m.keys.Help):
				m.overlay = ""
				m.helpOverlay.Toggle()
			case keyStr == "`":
				m.overlay = ""
			}
			return m, nil
		}
		if m.helpOverlay.Visible {
			switch {
			case key.Matches(msg, m.keys.Help), key.Matches(msg, m.keys.Back):
				m.helpOverlay.Toggle()
			}
			return m, nil
		}
		if m.searchOverlay != nil && m.searchOverlay.Visible() {
			var cmd tea.Cmd
			m.searchOverlay, cmd = m.searchOverlay.Update(msg)
			if !m.searchOverlay.Visible() && keyStr == "enter" {
				if sel := m.searchOverlay.Selected(); sel != nil {
					m.search.Query = ""
					if idx := indexOfSummaryID(m.summaries, sel.ID); idx >= 0 {
						m.selected = idx
					}
					m.persistUIState()
				}
			}
			return m, cmd
		}
		if m.search.Active {
			done, canceled := updateSearch(&m.search, keyStr)
			if done {
				m.search.Active = false
				if canceled {
					m.search.Query = ""
				}
			}
			m.rebuildGroups()
			return m, nil
		}
		if m.mode == "sprint" {
			newSprint, cmd := m.sprint.Update(msg)
			m.sprint = newSprint.(*SprintView)
			return m, cmd
		}
		if m.mode == "suggestions" {
			switch {
			case key.Matches(msg, m.keys.Quit), key.Matches(msg, m.keys.Back):
				m.mode = "list"
			case keyStr == "a":
				m.applySuggestions()
				m.mode = "list"
			case keyStr == "1":
				m.suggestions.acceptSummary = !m.suggestions.acceptSummary
			case keyStr == "2":
				m.suggestions.acceptRequirements = !m.suggestions.acceptRequirements
			case keyStr == "3":
				m.suggestions.acceptCUJ = !m.suggestions.acceptCUJ
			case keyStr == "4":
				m.suggestions.acceptMarket = !m.suggestions.acceptMarket
			case keyStr == "5":
				m.suggestions.acceptCompetitive = !m.suggestions.acceptCompetitive
			}
			return m, nil
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Select):
			m.toggleSelectedGroup()
		case keyStr == "a":
			m.confirmArchive()
		case keyStr == "d":
			m.confirmDelete()
		case keyStr == "u":
			m.confirmUndo()
		case keyStr == "H":
			m.showArchived = !m.showArchived
			m.reloadSummaries()
			m.persistUIState()
		case key.Matches(msg, m.keys.Search):
			if m.searchOverlay != nil {
				m.searchOverlay.SetItems(m.summaries)
				m.searchOverlay.Show()
			} else {
				m.search.Active = true
				m.search.Query = ""
			}
		case key.Matches(msg, m.keys.TabCycle):
			if m.focus == "LIST" {
				m.focus = "DETAIL"
			} else {
				m.focus = "LIST"
			}
		case key.Matches(msg, m.keys.Help):
			m.helpOverlay.Toggle()
		case key.Matches(msg, m.keys.Refresh):
			m.reloadSummaries()
			m.persistUIState()
		case keyStr == "`":
			m.overlay = "tutorial"
		case keyStr == "g":
			if m.err == "" {
				m.startSprintForSelected()
			}
		case keyStr == "n":
			if m.err == "" {
				m.startSprint()
			}
		case keyStr == "R":
			if m.err == "" {
				m.runResearchForSelected()
			}
		case keyStr == "p":
			if m.err == "" {
				m.runSuggestionsForSelected()
			}
		case keyStr == "s":
			if m.err == "" {
				m.enterSuggestions()
			}
		case key.Matches(msg, m.keys.NavDown):
			if m.selected < len(m.flatItems)-1 {
				m.selected++
			}
		case key.Matches(msg, m.keys.NavUp):
			if m.selected > 0 {
				m.selected--
			}
		case key.Matches(msg, m.keys.Bottom):
			if len(m.flatItems) > 0 {
				m.selected = len(m.flatItems) - 1
			}
		case key.Matches(msg, m.keys.Top):
			if len(m.flatItems) > 0 {
				m.selected = 0
			}
		}
		m.viewOffset = clampViewOffset(m.selected, m.viewOffset, m.listContentHeight(), len(m.flatItems))
		m.persistUIState()
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
	}
	return m, nil
}

func (m Model) View() string {
	title := "LIST"
	focus := m.focus
	var body string
	if m.confirmAction != "" {
		title = "CONFIRM"
		focus = "CONFIRM"
		body = renderConfirmOverlay(m.confirmMessage)
		header := renderHeader(title, focus)
		footer := renderFooter("enter confirm  esc cancel", m.status)
		body = padBodyToHeight(body, m.height-2)
		return renderFrame(header, body, footer)
	}
	if m.helpOverlay.Visible {
		title = "HELP"
		body = m.helpOverlay.Render(m.keys, m.helpExtras(), m.width)
		header := renderHeader(title, focus)
		footer := renderFooter(defaultKeys(), m.status)
		body = padBodyToHeight(body, m.height-2)
		return renderFrame(header, body, footer)
	}
	if m.overlay == "tutorial" {
		title = "TUTORIAL"
		body = renderTutorialOverlay()
		header := renderHeader(title, focus)
		footer := renderFooter(defaultKeys(), m.status)
		return renderFrame(header, body, footer)
	}
	if m.mode == "sprint" {
		title = "SPRINT"
		focus = "SPRINT"
		body = m.sprint.View()
	} else if m.mode == "suggestions" {
		title = "SUGGESTIONS"
		left := []string{"SUGGESTIONS"}
		right := m.renderSuggestions()
		body = renderSplitView(m.width, left, right)
	} else {
		contentHeight := m.height - 2
		mode := layoutMode(m.width)
		listHeight := m.listContentHeight()
		listContent := m.renderGroupListContent(listHeight)
		detailContent := strings.Join(m.renderDetail(), "\n")
		switch mode {
		case LayoutModeSingle:
			body = renderSingleColumnLayout("PRDs", listContent, m.width, contentHeight)
		case LayoutModeStacked:
			body = renderStackedLayout("PRDs", listContent, "DETAILS", detailContent, m.width, contentHeight)
		default:
			body = renderDualColumnLayoutFocused("PRDs", listContent, "DETAILS", detailContent, m.width, contentHeight, m.focus)
		}
	}
	header := renderHeader(title, focus)
	footer := renderFooter(defaultKeys(), m.status)
	body = padBodyToHeight(body, m.height-2)
	return renderFrame(header, body, footer)
}

func (m Model) renderGroupListContent(height int) string {
	if m.err != "" {
		return "PRDs\n" + m.err
	}
	if len(m.flatItems) == 0 {
		return "No PRDs yet."
	}
	return renderGroupList(m.flatItems, m.selected, m.viewOffset, height)
}

func (m Model) renderDetail() []string {
	lines := []string{"DETAILS"}
	if m.err != "" {
		lines = append(lines, "Initialize with praude init.")
		return lines
	}
	sel := m.selectedSummary()
	if sel == nil {
		lines = append(lines, "No PRD selected.")
		return lines
	}
	if spec, err := specs.LoadSpec(sel.Path); err == nil {
		markdown := detailMarkdown(spec)
		hash := specs.SpecHash(spec)
		rendered := markdown
		if m.mdCache != nil {
			if cached, ok := m.mdCache.Get(spec.ID, hash); ok {
				rendered = cached
			} else {
				rendered = renderMarkdown(markdown, m.width)
				m.mdCache.Set(spec.ID, hash, rendered)
			}
		} else {
			rendered = renderMarkdown(markdown, m.width)
		}
		trimmed := strings.TrimSpace(rendered)
		if trimmed != "" {
			lines = append(lines, strings.Split(trimmed, "\n")...)
		}
	}
	if strings.TrimSpace(m.status) != "" {
		lines = append(lines, "Last action: "+m.status)
	}
	return lines
}

func (m *Model) reloadSummaries() {
	if m.root == "" {
		return
	}
	list, _ := specs.LoadSummariesWithArchived(project.SpecsDir(m.root), project.ArchivedSpecsDir(m.root), m.showArchived)
	selectedID := ""
	if sel := m.selectedSummary(); sel != nil {
		selectedID = sel.ID
	}
	m.summaries = list
	if m.searchOverlay != nil {
		m.searchOverlay.SetItems(list)
	}
	m.rebuildGroups()
	if selectedID != "" {
		m.selected = selectedIndexFromID(m.flatItems, selectedID)
		m.viewOffset = clampViewOffset(m.selected, m.viewOffset, m.listContentHeight(), len(m.flatItems))
	}
}

func (m *Model) rebuildGroups() {
	m.ensureExpandedDefaults()
	filtered := m.summaries
	if strings.TrimSpace(m.search.Query) != "" {
		filtered = filterSummaries(m.summaries, m.search.Query)
	}
	tree := NewGroupTree(filtered, m.groupExpanded)
	m.groupTree = tree
	m.flatItems = tree.Flatten()
	if m.selected >= len(m.flatItems) {
		if len(m.flatItems) == 0 {
			m.selected = 0
		} else {
			m.selected = len(m.flatItems) - 1
		}
	}
	if len(m.flatItems) > 0 && m.flatItems[m.selected].Type == ItemTypeGroup {
		if idx := firstPRDIndex(m.flatItems); idx >= 0 {
			m.selected = idx
		}
	}
	m.viewOffset = clampViewOffset(m.selected, m.viewOffset, m.listContentHeight(), len(m.flatItems))
}

func (m *Model) ensureExpandedDefaults() {
	if m.groupExpanded == nil {
		m.groupExpanded = defaultExpanded()
		return
	}
	for _, status := range StatusOrder {
		if _, ok := m.groupExpanded[status]; !ok {
			m.groupExpanded[status] = true
		}
	}
}

func (m Model) selectedSummary() *specs.Summary {
	if len(m.flatItems) == 0 {
		return nil
	}
	item := m.flatItems[m.selected]
	if item.Type != ItemTypePRD {
		return nil
	}
	return item.Summary
}

func (m *Model) toggleSelectedGroup() {
	if len(m.flatItems) == 0 {
		return
	}
	item := m.flatItems[m.selected]
	if item.Type != ItemTypeGroup || item.Group == nil {
		return
	}
	if m.groupExpanded == nil {
		m.groupExpanded = defaultExpanded()
	}
	m.groupExpanded[item.Group.Name] = !item.Group.Expanded
	m.rebuildGroups()
	m.persistUIState()
}

func (m Model) listContentHeight() int {
	contentHeight := m.height - 2
	if contentHeight < 1 {
		contentHeight = 1
	}
	switch layoutMode(m.width) {
	case LayoutModeStacked:
		listHeight := (contentHeight * 60) / 100
		if listHeight < 3 {
			listHeight = 3
		}
		return max(1, listHeight-2)
	default:
		return max(1, contentHeight-2)
	}
}

func (m *Model) runResearchForSelected() {
	sel := m.selectedSummary()
	if sel == nil {
		m.status = "No PRD selected"
		return
	}
	id := sel.ID
	now := time.Now()
	researchDir := project.ResearchDir(m.root)
	if err := os.MkdirAll(researchDir, 0o755); err != nil {
		m.status = "Research failed: " + err.Error()
		return
	}
	researchPath, err := research.Create(researchDir, id, now)
	if err != nil {
		m.status = "Research failed: " + err.Error()
		return
	}
	briefPath, err := writeResearchBrief(m.root, id, researchPath, now)
	if err != nil {
		m.status = "Research failed: " + err.Error()
		return
	}
	cfg, err := config.LoadFromRoot(m.root)
	if err != nil {
		m.status = "Research failed: " + err.Error()
		return
	}
	agentName := defaultAgentName(cfg)
	profile, err := agents.Resolve(agentProfiles(cfg), agentName)
	if err != nil {
		m.status = "Research failed: " + err.Error()
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
	m.status = "launched research agent " + agentName
}

func (m *Model) runSuggestionsForSelected() {
	sel := m.selectedSummary()
	if sel == nil {
		m.status = "No PRD selected"
		return
	}
	id := sel.ID
	now := time.Now()
	suggDir := project.SuggestionsDir(m.root)
	if err := os.MkdirAll(suggDir, 0o755); err != nil {
		m.status = "Suggestions failed: " + err.Error()
		return
	}
	suggPath, err := suggestions.Create(suggDir, id, now)
	if err != nil {
		m.status = "Suggestions failed: " + err.Error()
		return
	}
	briefPath, err := writeSuggestionBrief(m.root, id, suggPath, now)
	if err != nil {
		m.status = "Suggestions failed: " + err.Error()
		return
	}
	cfg, err := config.LoadFromRoot(m.root)
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
	m.status = "launched suggestions agent " + agentName
}

// newOrchestratorWithScanner creates an orchestrator with the real Pollard scanner.
func newOrchestratorWithScanner(projectPath string) *arbiter.Orchestrator {
	o := arbiter.NewOrchestrator(projectPath)
	o.SetScanner(pollardquick.NewScanner())
	return o
}

func (m *Model) startSprint() {
	orch := newOrchestratorWithScanner(m.root)
	ctx := context.Background()

	var state *arbiter.SprintState
	var err error

	if specs.NeedsVisionSpec(m.summaries) {
		state, err = orch.StartVision(ctx, "")
		m.status = "Vision spec required before creating another PRD"
	} else {
		state, err = orch.Start(ctx, "")
	}

	if err != nil {
		m.status = "Sprint failed: " + err.Error()
		return
	}
	m.sprint = NewSprintView(state)
	m.mode = "sprint"
}

func (m *Model) startSprintForSelected() {
	sel := m.selectedSummary()
	if sel == nil {
		m.status = "Select a PRD first"
		return
	}
	spec, err := specs.LoadSpec(sel.Path)
	if err != nil {
		m.status = "Load failed: " + err.Error()
		return
	}
	// Migrate existing spec into a sprint state
	orch := newOrchestratorWithScanner(m.root)
	state := arbiter.MigrateFromSpec(&spec, sel.Path)
	_ = orch // orchestrator available for future phase advancement
	m.sprint = NewSprintView(state)
	m.mode = "sprint"
}

func formatCompleteness(spec specs.Spec) string {
	summary := "no"
	if strings.TrimSpace(spec.Summary) != "" {
		summary = "yes"
	}
	return fmt.Sprintf(
		"Completeness: summary %s | req %d | cuj %d | market %d | competitive %d",
		summary,
		len(spec.Requirements),
		len(spec.CriticalUserJourneys),
		len(spec.MarketResearch),
		len(spec.CompetitiveLandscape),
	)
}

func formatCUJDetail(spec specs.Spec) string {
	if len(spec.CriticalUserJourneys) == 0 {
		return "CUJ: none"
	}
	cuj := spec.CriticalUserJourneys[0]
	label := cuj.ID
	if cuj.Title != "" {
		label += " " + cuj.Title
	}
	if cuj.Priority != "" {
		label += " (" + cuj.Priority + ")"
	}
	return "CUJ: " + label
}

func indexOfSummaryID(summaries []specs.Summary, id string) int {
	for i, summary := range summaries {
		if summary.ID == id {
			return i
		}
	}
	return -1
}

func defaultExpanded() map[string]bool {
	expanded := make(map[string]bool, len(StatusOrder))
	for _, status := range StatusOrder {
		expanded[status] = true
	}
	return expanded
}

func (m *Model) persistUIState() {
	if m.root == "" || m.groupExpanded == nil {
		return
	}
	selectedID := ""
	if sel := m.selectedSummary(); sel != nil {
		selectedID = sel.ID
	}
	state := UIState{
		Expanded:     m.groupExpanded,
		SelectedID:   selectedID,
		ShowArchived: m.showArchived,
		LastAction:   m.lastAction,
	}
	if err := SaveUIState(project.StatePath(m.root), state); err != nil {
		m.status = "state save failed: " + err.Error()
	}
}

func (m *Model) confirmArchive() {
	sel := m.selectedSummary()
	if sel == nil {
		m.status = "No PRD selected"
		return
	}
	m.confirmAction = "archive"
	m.confirmID = sel.ID
	m.confirmMessage = fmt.Sprintf("Archive %s?", sel.ID)
}

func (m *Model) confirmDelete() {
	sel := m.selectedSummary()
	if sel == nil {
		m.status = "No PRD selected"
		return
	}
	m.confirmAction = "delete"
	m.confirmID = sel.ID
	m.confirmMessage = fmt.Sprintf("Delete %s? (reversible)", sel.ID)
}

func (m *Model) confirmUndo() {
	if m.lastAction == nil {
		m.status = "Nothing to undo"
		return
	}
	m.confirmAction = "undo"
	m.confirmID = m.lastAction.ID
	m.confirmMessage = fmt.Sprintf("Undo %s for %s?", m.lastAction.Type, m.lastAction.ID)
}

func (m *Model) applyConfirmAction() {
	action := m.confirmAction
	id := m.confirmID
	last := m.lastAction
	m.clearConfirm()
	switch action {
	case "archive":
		sel := summaryByID(m.summaries, id)
		if sel == nil {
			m.status = "No PRD selected"
			return
		}
		prevStatus := ""
		if spec, err := specs.LoadSpec(sel.Path); err == nil {
			prevStatus = spec.Status
		}
		res, err := archive.Archive(m.root, id)
		if err != nil {
			m.status = "Archive failed: " + err.Error()
			return
		}
		m.lastAction = &LastAction{Type: "archive", ID: id, PrevStatus: prevStatus, From: res.From, To: res.To}
		m.status = "Archived " + id
		m.reloadSummaries()
		m.persistUIState()
	case "delete":
		sel := summaryByID(m.summaries, id)
		if sel == nil {
			m.status = "No PRD selected"
			return
		}
		prevStatus := ""
		if spec, err := specs.LoadSpec(sel.Path); err == nil {
			prevStatus = spec.Status
		}
		res, err := archive.Delete(m.root, id)
		if err != nil {
			m.status = "Delete failed: " + err.Error()
			return
		}
		m.lastAction = &LastAction{Type: "delete", ID: id, PrevStatus: prevStatus, From: res.From, To: res.To}
		m.status = "Deleted " + id
		m.reloadSummaries()
		m.persistUIState()
	case "undo":
		if last == nil {
			m.status = "Nothing to undo"
			return
		}
		if err := archive.Undo(m.root, last.From, last.To); err != nil {
			m.status = "Undo failed: " + err.Error()
			return
		}
		if last.PrevStatus != "" {
			specPath := ""
			for _, path := range last.From {
				if strings.HasSuffix(path, last.ID+".yaml") {
					specPath = path
					break
				}
			}
			if specPath != "" {
				_ = specs.UpdateStatus(specPath, last.PrevStatus)
			}
		}
		m.lastAction = nil
		m.status = "Undo " + last.Type + " " + last.ID
		m.reloadSummaries()
		m.persistUIState()
	}
}

func (m *Model) clearConfirm() {
	m.confirmAction = ""
	m.confirmMessage = ""
	m.confirmID = ""
}

func summaryByID(summaries []specs.Summary, id string) *specs.Summary {
	for i := range summaries {
		if summaries[i].ID == id {
			return &summaries[i]
		}
	}
	return nil
}

func selectedIndexFromID(items []Item, id string) int {
	for i, item := range items {
		if item.Type == ItemTypePRD && item.Summary != nil && item.Summary.ID == id {
			return i
		}
	}
	return 0
}

func clampViewOffset(cursor, viewOffset, height, total int) int {
	if total <= 0 {
		return 0
	}
	if height < 1 {
		height = 1
	}
	visible := height
	if viewOffset > 0 {
		visible--
		if visible < 1 {
			visible = 1
		}
	}
	if cursor < viewOffset {
		viewOffset = cursor
	}
	if cursor >= viewOffset+visible {
		viewOffset = cursor - visible + 1
	}
	if viewOffset < 0 {
		viewOffset = 0
	}
	maxOffset := total - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if viewOffset > maxOffset {
		viewOffset = maxOffset
	}
	return viewOffset
}

func firstPRDIndex(items []Item) int {
	for i, item := range items {
		if item.Type == ItemTypePRD {
			return i
		}
	}
	return -1
}

func formatResearchDetail(spec specs.Spec) string {
	market := "none"
	if len(spec.MarketResearch) > 0 {
		market = spec.MarketResearch[0].ID
		if spec.MarketResearch[0].Claim != "" {
			market += " " + spec.MarketResearch[0].Claim
		}
	}
	comp := "none"
	if len(spec.CompetitiveLandscape) > 0 {
		comp = spec.CompetitiveLandscape[0].ID
		if spec.CompetitiveLandscape[0].Name != "" {
			comp += " " + spec.CompetitiveLandscape[0].Name
		}
	}
	return "Market: " + market + " | Competitive: " + comp
}

func formatWarnings(spec specs.Spec) []string {
	if len(spec.Metadata.ValidationWarnings) == 0 {
		return nil
	}
	lines := []string{"Validation warnings:"}
	for _, warning := range spec.Metadata.ValidationWarnings {
		lines = append(lines, "- "+warning)
	}
	return lines
}

func joinColumns(left, right []string, leftWidth int) string {
	max := len(left)
	if len(right) > max {
		max = len(right)
	}
	var b strings.Builder
	for i := 0; i < max; i++ {
		l := ""
		r := ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		b.WriteString(padRight(l, leftWidth))
		b.WriteString(" | ")
		b.WriteString(r)
		if i < max-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func padRight(s string, width int) string {
	if visibleWidth(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visibleWidth(s))
}

func visibleWidth(s string) int {
	plain := ansiRegex.ReplaceAllString(s, "")
	return utf8.RuneCountInString(plain)
}

func defaultKeys() string {
	return "j/k move  enter toggle  / search  tab focus  n new  g sprint  [ ] prev/next  ctrl+o open  \\ swap  a archive  d delete  u undo  H archived  R research  p suggestions  s review  ? help  q quit"
}

func (m Model) helpExtras() []pkgtui.HelpBinding {
	return []pkgtui.HelpBinding{
		{Key: "a", Description: "archive"},
		{Key: "d", Description: "delete"},
		{Key: "u", Description: "undo"},
		{Key: "H", Description: "archived"},
		{Key: "g", Description: "sprint from PRD"},
		{Key: "n", Description: "new sprint"},
		{Key: "R", Description: "research"},
		{Key: "p", Description: "suggestions"},
		{Key: "s", Description: "review"},
		{Key: "`", Description: "tutorial"},
	}
}

func padBodyToHeight(body string, height int) string {
	if height <= 0 {
		return body
	}
	lines := []string{""}
	if strings.TrimSpace(body) != "" {
		lines = strings.Split(body, "\n")
	}
	if len(lines) >= height {
		return body
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
