package tui

import (
	"fmt"
	"path/filepath"

	"github.com/mistakeknot/autarch/internal/gurgeh/git"
	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/suggestions"
)

type suggestionsState struct {
	active             bool
	id                 string
	path               string
	sugg               suggestions.Suggestion
	err                string
	acceptSummary      bool
	acceptRequirements bool
	acceptCUJ          bool
	acceptMarket       bool
	acceptCompetitive  bool
}

func (m *Model) enterSuggestions() {
	sel := m.selectedSummary()
	if sel == nil {
		return
	}
	id := sel.ID
	dir := project.SuggestionsDir(m.root)
	sugg, path, err := suggestions.LoadLatest(dir, id)
	if err != nil {
		m.suggestions = suggestionsState{active: true, id: id, err: err.Error()}
		m.mode = "suggestions"
		return
	}
	m.suggestions = suggestionsState{
		active:             true,
		id:                 id,
		path:               path,
		sugg:               sugg,
		acceptSummary:      sugg.Summary != "",
		acceptRequirements: len(sugg.Requirements) > 0,
		acceptCUJ:          len(sugg.CriticalUserJourneys) > 0,
		acceptMarket:       len(sugg.MarketResearch) > 0,
		acceptCompetitive:  len(sugg.CompetitiveLandscape) > 0,
	}
	m.mode = "suggestions"
}

func (m *Model) applySuggestions() {
	if m.suggestions.path == "" {
		return
	}
	specPath := filepath.Join(project.SpecsDir(m.root), m.suggestions.id+".yaml")
	selected := suggestions.Suggestion{}
	if m.suggestions.acceptSummary {
		selected.Summary = m.suggestions.sugg.Summary
	}
	if m.suggestions.acceptRequirements {
		selected.Requirements = m.suggestions.sugg.Requirements
	}
	if m.suggestions.acceptCUJ {
		selected.CriticalUserJourneys = m.suggestions.sugg.CriticalUserJourneys
	}
	if m.suggestions.acceptMarket {
		selected.MarketResearch = m.suggestions.sugg.MarketResearch
	}
	if m.suggestions.acceptCompetitive {
		selected.CompetitiveLandscape = m.suggestions.sugg.CompetitiveLandscape
	}
	if err := suggestions.Apply(specPath, selected); err != nil {
		m.suggestions.err = err.Error()
		return
	}
	if err := git.EnsureRepo(m.root); err == nil {
		_ = git.CommitFiles(m.root, []string{specPath}, "chore(praude): apply suggestions "+m.suggestions.id)
	}
	m.reloadSummaries()
}

func (m *Model) renderSuggestions() []string {
	lines := []string{"Suggestions"}
	if m.suggestions.err != "" {
		lines = append(lines, "Error: "+m.suggestions.err)
		return lines
	}
	lines = append(lines, "Spec: "+m.suggestions.id)
	lines = append(lines, "1 Summary: "+toggleLabel(m.suggestions.acceptSummary, m.suggestions.sugg.Summary))
	lines = append(lines, "2 Requirements: "+countToggle(m.suggestions.acceptRequirements, len(m.suggestions.sugg.Requirements)))
	lines = append(lines, "3 CUJ: "+countToggle(m.suggestions.acceptCUJ, len(m.suggestions.sugg.CriticalUserJourneys)))
	lines = append(lines, "4 Market: "+countToggle(m.suggestions.acceptMarket, len(m.suggestions.sugg.MarketResearch)))
	lines = append(lines, "5 Competitive: "+countToggle(m.suggestions.acceptCompetitive, len(m.suggestions.sugg.CompetitiveLandscape)))
	lines = append(lines, "[1-5] toggle  [a] accept  [r] reject  [q] quit")
	return lines
}

func toggleLabel(accepted bool, summary string) string {
	if summary == "" {
		return "[none]"
	}
	if accepted {
		return "[accept] " + summary
	}
	return "[skip] " + summary
}

func countToggle(accepted bool, count int) string {
	if count == 0 {
		return "[none]"
	}
	label := "[skip]"
	if accepted {
		label = "[accept]"
	}
	return label + " " + fmt.Sprint(count)
}
